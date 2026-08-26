package redstoneoev

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

var weiPerEth = chain.Exp10(18)

const (
	skipBidCap             = "bid_cap"
	skipDepositLow         = "deposit_low"
	skipEmptyAuctionID     = "empty_auction_id"
	skipExecutorStateStale = "executor_state_stale"

	bidDecisionDeadlineMargin = 50 * time.Millisecond
)

// bidDecision is the outcome of evaluating one auction: either a ready-to-send solve or a bounded skip.
type bidDecision struct {
	solve      SolveMessage
	nonce      uint64
	callback   common.Address
	skip       string
	skipDetail string
}

func (s *Solver) handleMessage(ctx context.Context, raw []byte) {
	op, err := opName(raw)
	if err != nil {
		s.log.V(1).Error(err, "drop unparseable frame")
		return
	}
	switch op {
	case "auction":
		if isFeedAuction(raw) {
			s.log.V(1).Info("ignoring feed auction")
			return
		}
		a, start, ok := s.parseAuctionFrame(raw)
		if !ok {
			return
		}
		go s.handleAuction(ctx, a, start)
	case "auction-result":
		s.handleAuctionResult(raw)
	case "liquidation-result":
		s.handleLiquidationResult(raw)
	case "blacklisted":
		s.handleBlacklisted(raw)
	default:
		s.log.V(1).Info("ignoring frame", "op", op)
	}
}

func (s *Solver) handleAuctionResult(raw []byte) {
	var r AuctionResult
	if err := json.Unmarshal(raw, &r); err != nil {
		s.log.V(1).Error(err, "drop malformed frame", "op", "auction-result")
		return
	}
	liquidator := common.HexToAddress(r.Data.Liquidator)
	won := liquidator == s.cfg.Callback
	if won {
		s.metrics.won()
		s.markReservationWon(r.ID)
	} else {
		s.releaseReservationByAuction(r.ID)
	}
	s.log.Info("auction-result", "id", r.ID, "winner", r.Data.Liquidator, "bid", r.Data.Bid, "won", won)
}

func (s *Solver) handleLiquidationResult(raw []byte) {
	var r LiquidationResult
	if err := json.Unmarshal(raw, &r); err != nil {
		s.log.V(1).Error(err, "drop malformed frame", "op", "liquidation-result")
		return
	}
	liquidator := common.HexToAddress(r.Data.Liquidator)
	ours := liquidator == s.cfg.Callback
	s.log.Info("liquidation-result", "id", r.ID, "success", r.Data.Success,
		"txHash", r.Data.TxHash, "error", r.Data.Error, "ours", ours)
	if !ours {
		return
	}
	s.requestStateRefresh()
	s.releaseReservationByAuction(r.ID)
	if !r.Data.Success {
		s.breaker.recordFailure(time.Now())
		s.metrics.failed()
	}
}

func (s *Solver) handleBlacklisted(raw []byte) {
	var b Blacklisted
	if err := json.Unmarshal(raw, &b); err != nil {
		s.log.Error(err, "malformed blacklisted frame; halting bidding")
	}
	s.breaker.blacklist()
	s.log.Error(errors.New("api key blacklisted"), "halting bidding", "msg", b.Data.Msg)
}

func (s *Solver) parseAuctionFrame(raw []byte) (AuctionMessage, time.Time, bool) {
	start := time.Now()
	var a AuctionMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		s.log.V(1).Error(err, "drop malformed auction")
		return AuctionMessage{}, time.Time{}, false
	}
	s.metrics.auction()
	key := a.dedupKey()
	if key == "" {
		s.metrics.skip(skipEmptyAuctionID)
		s.log.Info("auction with empty id received; dropping", "timestamp", a.Timestamp, "timeoutMs", a.TimeoutMs)
		return AuctionMessage{}, time.Time{}, false
	}
	if s.seen.seen(key) {
		s.metrics.skip("duplicate")
		s.log.V(1).Info("duplicate auction; already processed", "auction", a.ID)
		return AuctionMessage{}, time.Time{}, false
	}
	return a, start, true
}

func (s *Solver) handleAuction(ctx context.Context, a AuctionMessage, start time.Time) {
	s.bidMu.Lock()
	defer s.bidMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if s.bidExpired(a, start) {
		return
	}
	bidCtx, cancel := auctionBidContext(ctx, a, start)
	defer cancel()
	d := s.buildBid(bidCtx, a, time.Now)
	s.metrics.latency(time.Since(start))

	if d.skip != "" {
		s.logSkip(a.ID, d)
		return
	}
	if s.dryRun {
		s.metrics.bid()
		s.log.Info("DRY-RUN would bid", "auction", a.ID, "callback", d.callback.Hex(), "nonce", d.solve.Data.Nonce,
			"bidEth", d.solve.Data.Bid)
		return
	}
	if s.bidExpired(a, start) {
		return
	}
	if !s.ws.Send(marshal(d.solve)) {
		s.metrics.skip("send_dropped")
		s.log.Info("bid NOT sent (ws buffer full)", "auction", a.ID, "nonce", d.solve.Data.Nonce)
		return
	}
	s.reserve(d.nonce, time.Now(), a.ID)
	s.metrics.bid()
	s.log.Info("bid sent", "auction", a.ID, "callback", d.callback.Hex(), "nonce", d.solve.Data.Nonce,
		"bidEth", d.solve.Data.Bid)
}

func auctionBidContext(ctx context.Context, a AuctionMessage, start time.Time) (context.Context, context.CancelFunc) {
	if a.TimeoutMs <= 0 {
		return context.WithCancel(ctx)
	}
	deadline := auctionDeadline(a, start).Add(-bidDecisionDeadlineMargin)
	if deadline.Before(start) {
		deadline = start
	}
	return context.WithDeadline(ctx, deadline)
}

func (s *Solver) logSkip(auctionID string, d bidDecision) {
	s.metrics.skip(d.skip)
	if d.skipDetail != "" {
		s.log.V(1).Info("no bid", "auction", auctionID, "reason", d.skip, "strategyReason", d.skipDetail)
		return
	}
	s.log.V(1).Info("no bid", "auction", auctionID, "reason", d.skip)
}

func (s *Solver) bidExpired(a AuctionMessage, start time.Time) bool {
	now := time.Now()
	if a.TimeoutMs <= 0 || !tooLate(a.Timestamp, a.TimeoutMs, start, now) {
		return false
	}
	s.metrics.skip("too_late")
	s.log.Info("bid not sent: auction deadline (since emit) exceeded",
		"auction", a.ID, "timeoutMs", a.TimeoutMs, "sinceEmitMs", sinceEmitMs(a.Timestamp, now),
		"localElapsedMs", time.Since(start).Milliseconds())
	return true
}

// staleStateGate fails closed when the solver-owned Executor accounting is older than cfg.ExecutorStateMaxAge.
func (s *Solver) staleStateGate(auctionID string, now time.Time) string {
	kv := make([]any, 0, 4)
	if st, ok := s.state.load(); !ok || now.Sub(st.UpdatedAt) > s.cfg.ExecutorStateMaxAge {
		var at time.Time
		if ok {
			at = st.UpdatedAt
		}
		kv = append(kv, "opsAge", cacheAge(at, now))
	}
	if len(kv) == 0 {
		return ""
	}
	s.log.Error(errors.New("executor state stale"), "bid skipped: cache exceeds intervals.executorStateMaxAgeMs",
		append(kv, "maxAge", s.cfg.ExecutorStateMaxAge, "auction", auctionID)...)
	return skipExecutorStateStale
}

func cacheAge(at, now time.Time) string {
	if at.IsZero() {
		return "never"
	}
	return now.Sub(at).String()
}

func (s *Solver) buildBid(ctx context.Context, a AuctionMessage, nowFn func() time.Time) bidDecision {
	now := nowFn()
	if tripped, _ := s.breaker.tripped(now); tripped {
		return bidDecision{skip: "breaker"}
	}
	if skip := s.staleStateGate(a.ID, now); skip != "" {
		return bidDecision{skip: skip}
	}
	st, ok := s.state.load()
	if !ok {
		return bidDecision{skip: "state_unknown"}
	}
	if st.Exec.Locked {
		return bidDecision{skip: "signer_locked"}
	}
	if depositSkip := s.depositSkip(a, st); depositSkip != "" {
		return bidDecision{skip: depositSkip}
	}
	inFlight := s.inFlightSnapshot()
	gasPrice := new(big.Int).Set(s.cfg.MaxTxGasPrice)
	if s.strategy == nil {
		s.log.Error(errors.New("strategy is not configured"), "bid skipped", "auction", a.ID)
		return bidDecision{skip: "strategy_error"}
	}
	out, err := s.strategy.DecideBid(ctx, s.bidInput(a, now, st, inFlight, gasPrice))
	if err != nil {
		s.log.Error(err, "strategy failed", "auction", a.ID)
		return bidDecision{skip: "strategy_error"}
	}
	if err := checkExecutionEnvelope(out); err != nil {
		s.log.Error(err, "execution envelope rejected", "auction", a.ID)
		return bidDecision{skip: "strategy_invalid"}
	}
	if out.Decision == types.DecisionSkip {
		return bidDecision{skip: types.BoundedSkipReason(out.Reason), skipDetail: out.Reason}
	}
	bidNative := cloneBig(out.BidAmount)
	if s.bidCapExceeded(a, bidNative) {
		return bidDecision{skip: skipBidCap}
	}
	nonce := s.nonces.next(st.Exec.Nonce.Uint64())
	callback := s.cfg.Callback
	sig, err := SignBid(s.deps.Signer, s.chainID, callback, out.OperationData, bidNative, big.NewInt(int64(nonce)), gasPrice)
	if err != nil {
		s.log.Error(err, "sign bid failed", "auction", a.ID)
		return bidDecision{skip: "sign_error"}
	}

	return bidDecision{
		nonce:    nonce,
		callback: callback,
		solve: SolveMessage{
			Op: "solve", ID: a.ID,
			Data: SolveData{
				Bid:               weiToEthString(bidNative),
				Nonce:             new(big.Int).SetUint64(nonce).String(),
				OperationCallback: callback.Hex(),
				OperationData:     hexutil.Encode(out.OperationData),
				LiquidationSig:    hexutil.Encode(sig),
				MaxTxGasPrice:     gasPrice.String(),
			},
		},
	}
}

func (s *Solver) bidCapExceeded(a AuctionMessage, bidNative *big.Int) bool {
	if s.cfg.MaxBidWei == nil || bidNative.Cmp(s.cfg.MaxBidWei) <= 0 {
		return false
	}
	s.log.Info("bid skipped: strategy bid exceeds configured cap",
		"auction", a.ID, "bidWei", bidNative, "maxBidWei", s.cfg.MaxBidWei)
	return true
}

func (s *Solver) depositSkip(a AuctionMessage, st cachedState) string {
	if orZero(st.Exec.Deposit).Cmp(minDeposit) < 0 {
		s.log.Info("bid skipped: executor deposit below minimum",
			"auction", a.ID, "depositWei", st.Exec.Deposit, "minDepositWei", minDeposit)
		return skipDepositLow
	}
	return ""
}

func tooLate(emitMs int64, timeoutMs int, start, now time.Time) bool {
	window := time.Duration(timeoutMs) * time.Millisecond
	if emitMs <= 0 || emitMs > now.UnixMilli() {
		return now.Sub(start) > window
	}
	return now.UnixMilli()-emitMs > int64(timeoutMs)
}

func auctionDeadline(a AuctionMessage, start time.Time) time.Time {
	window := time.Duration(a.TimeoutMs) * time.Millisecond
	if a.Timestamp <= 0 || a.Timestamp > start.UnixMilli() {
		return start.Add(window)
	}
	return time.UnixMilli(a.Timestamp).Add(window)
}

func sinceEmitMs(emitMs int64, now time.Time) int64 {
	if emitMs <= 0 {
		return 0
	}
	return now.UnixMilli() - emitMs
}

func weiToEthString(wei *big.Int) string {
	q, r := new(big.Int).DivMod(wei, weiPerEth, new(big.Int))
	if r.Sign() == 0 {
		return q.String()
	}
	frac := r.String()
	for len(frac) < 18 {
		frac = "0" + frac
	}
	frac = strings.TrimRight(frac, "0")
	return q.String() + "." + frac
}
