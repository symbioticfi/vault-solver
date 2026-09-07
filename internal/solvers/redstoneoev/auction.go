package redstoneoev

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const (
	auctionOutcomeContextCanceled = "context_canceled"
	auctionOutcomeDuplicate       = "duplicate"
	auctionOutcomeEnqueued        = "enqueued"
	auctionOutcomeFeedIgnored     = "feed_ignored"
	auctionOutcomeSendDropped     = "send_dropped"
	auctionOutcomeTooLate         = "too_late"
	auctionOutcomeWouldBid        = "would_bid"

	skipBidCap             = "bid_cap"
	skipDepositLow         = "deposit_low"
	skipEmptyAuctionID     = "empty_auction_id"
	skipExecutorStateStale = "executor_state_stale"

	bidDecisionDeadlineMargin = 50 * time.Millisecond
)

// bidDecision is the outcome of evaluating one auction: either a ready-to-send solve or a bounded skip.
type bidDecision struct {
	solve      SolveMessage
	bidWei     *big.Int
	nonce      uint64
	callback   common.Address
	skip       string
	skipDetail string
}

func (s *Solver) handleMessage(ctx context.Context, raw []byte) {
	frame, err := decodeFrame(raw)
	if err != nil {
		s.log.V(1).Error(err, "drop unparseable frame")
		return
	}
	switch frame.Op {
	case "auction":
		start := time.Now()
		if frame.feedAuction() {
			s.metrics.auctionDecision(auctionOutcomeFeedIgnored, time.Since(start))
			s.log.V(1).Info("ignoring feed auction")
			return
		}
		a, start, ok := s.parseAuctionFrame(raw)
		if !ok {
			return
		}
		// One decision owns the pending-auction snapshot. Do not queue unbounded
		// goroutines behind a slow strategy while result frames need to keep flowing.
		if !s.bidMu.TryLock() {
			s.metrics.auctionDecision("bid_busy", time.Since(start))
			return
		}
		s.bidWG.Go(func() {
			defer s.bidMu.Unlock()
			s.decideAuction(ctx, a, start)
		})
	case "auction-result":
		s.handleAuctionResult(raw)
	case "liquidation-result":
		s.handleLiquidationResult(raw)
	case "blacklisted":
		s.handleBlacklisted(raw)
	default:
		s.log.V(1).Info("ignoring frame", "op", frame.Op)
	}
}

func (s *Solver) handleAuctionResult(raw []byte) {
	result, ok := decodeResultFrame[AuctionResult](s.log, raw, "auction-result")
	if !ok {
		return
	}
	result.ID = normalizeAuctionID(result.ID)
	won := common.HexToAddress(result.Data.Liquidator) == s.cfg.Callback
	if !won {
		s.releaseReservationByAuction(result.ID)
	} else {
		if amount, transitioned := s.markReservationWon(result.ID, time.Now()); transitioned {
			s.metrics.won(amount)
		}
	}
	s.log.Info("auction-result", "id", result.ID, "winner", result.Data.Liquidator, "bid", result.Data.Bid, "won", won)
}

func (s *Solver) handleLiquidationResult(raw []byte) {
	result, ok := decodeResultFrame[LiquidationResult](s.log, raw, "liquidation-result")
	if !ok {
		return
	}
	result.ID = normalizeAuctionID(result.ID)
	ours := common.HexToAddress(result.Data.Liquidator) == s.cfg.Callback
	s.log.Info("liquidation-result", "id", result.ID, "success", result.Data.Success,
		"txHash", result.Data.TxHash, "error", result.Data.Error, "ours", ours)
	if !ours {
		return
	}
	s.requestStateRefresh()
	identity := liquidationResultIdentity(result)
	key := result.ID
	if key == "" {
		key = identity
	}
	transition := s.settleReservationByAuction(result.ID, key)
	if transition.won {
		s.metrics.won(transition.bidWei)
	}
	if transition.settled {
		s.metrics.settlement(result.Data.Success, transition.bidWei)
	}
	if result.Data.Success {
		return
	}
	if identity != "" {
		if !s.breaker.recordFailureOnce(identity, time.Now()) {
			return
		}
	} else {
		s.breaker.recordFailure(time.Now())
	}
	s.metrics.breakerFailure()
}

func decodeResultFrame[T any](log logr.Logger, raw []byte, operation string) (T, bool) {
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		log.V(1).Error(err, "drop malformed frame", "op", operation)
		return result, false
	}
	return result, true
}

func liquidationResultIdentity(result LiquidationResult) string {
	if id := strings.TrimSpace(result.ID); id != "" {
		return "id:" + id
	}
	if txHash := strings.ToLower(strings.TrimSpace(result.Data.TxHash)); txHash != "" {
		return "tx:" + txHash
	}
	return ""
}

func (s *Solver) handleBlacklisted(raw []byte) {
	var b Blacklisted
	_ = json.Unmarshal(raw, &b)
	s.breaker.blacklist()
	s.log.Error(errors.New("api key blacklisted"), "halting bidding", "msg", b.Data.Msg)
}

func (s *Solver) handleAuctionWithContext(ctx context.Context, raw []byte) {
	a, start, ok := s.parseAuctionFrame(raw)
	if !ok {
		return
	}
	s.handleAuction(ctx, a, start)
}

func (s *Solver) parseAuctionFrame(raw []byte) (AuctionMessage, time.Time, bool) {
	start := time.Now()
	var a AuctionMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		s.log.V(1).Error(err, "drop malformed auction")
		return AuctionMessage{}, time.Time{}, false
	}
	a.ID = normalizeAuctionID(a.ID)
	key := a.dedupKey()
	if key == "" {
		s.metrics.auctionDecision(skipEmptyAuctionID, time.Since(start))
		s.log.Info("auction with empty id received; dropping", "timestamp", a.Timestamp, "timeoutMs", a.TimeoutMs)
		return AuctionMessage{}, time.Time{}, false
	}
	if s.seen.seen(key) {
		s.metrics.auctionDecision(auctionOutcomeDuplicate, time.Since(start))
		s.log.V(1).Info("duplicate auction; already processed", "auction", a.ID)
		return AuctionMessage{}, time.Time{}, false
	}
	return a, start, true
}

func (s *Solver) handleAuction(ctx context.Context, a AuctionMessage, start time.Time) {
	s.bidMu.Lock()
	defer s.bidMu.Unlock()
	s.decideAuction(ctx, a, start)
}

func (s *Solver) decideAuction(ctx context.Context, a AuctionMessage, start time.Time) {
	outcome := auctionOutcomeContextCanceled
	defer func() {
		s.metrics.auctionDecision(outcome, time.Since(start))
	}()

	if ctx.Err() != nil {
		return
	}
	if s.bidExpired(a, start) {
		outcome = auctionOutcomeTooLate
		return
	}
	bidCtx, cancel := auctionBidContext(ctx, a, start)
	defer cancel()
	d := s.buildBid(bidCtx, a, time.Now)

	if d.skip != "" {
		outcome = d.skip
		s.logSkip(a.ID, d)
		return
	}
	if s.dryRun {
		outcome = auctionOutcomeWouldBid
		s.metrics.wouldBid(d.bidWei)
		s.log.Info("DRY-RUN would bid", "auction", a.ID, "callback", d.callback.Hex(), "nonce", d.solve.Data.Nonce,
			"bidEth", d.solve.Data.Bid)
		return
	}
	if s.bidExpired(a, start) {
		outcome = auctionOutcomeTooLate
		return
	}
	if ctx.Err() != nil {
		outcome = auctionOutcomeContextCanceled
		return
	}
	var sendDeadline time.Time
	if a.TimeoutMs > 0 {
		sendDeadline = auctionDeadline(a, start)
	}
	s.reserve(d.nonce, time.Now(), a.ID, d.bidWei)
	if !s.ws.Send(ctx, marshal(d.solve), sendDeadline) {
		outcome = auctionOutcomeSendDropped
		s.releaseReservationByAuction(a.ID)
		s.log.Info("bid NOT enqueued (ws buffer full)", "auction", a.ID, "nonce", d.solve.Data.Nonce)
		return
	}
	outcome = auctionOutcomeEnqueued
	s.metrics.enqueuedBid(d.bidWei)
	s.log.Info("bid enqueued", "auction", a.ID, "callback", d.callback.Hex(), "nonce", d.solve.Data.Nonce,
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
	s.log.Info("bid not enqueued: auction deadline (since emit) exceeded",
		"auction", a.ID, "timeoutMs", a.TimeoutMs, "sinceEmitMs", sinceEmitMs(a.Timestamp, now),
		"localElapsedMs", time.Since(start).Milliseconds())
	return true
}

func cacheAge(at, now time.Time) string {
	if at.IsZero() {
		return "never"
	}
	return now.Sub(at).String()
}

func (s *Solver) buildBid(ctx context.Context, auction AuctionMessage, nowFn func() time.Time) bidDecision {
	now := nowFn()
	log := s.log.WithValues("auction", auction.ID)
	if stopped, _ := s.breaker.tripped(now); stopped {
		return bidDecision{skip: "breaker"}
	}
	snapshot, known := s.state.load()
	if !known || now.Sub(snapshot.UpdatedAt) > s.cfg.ExecutorStateMaxAge {
		log.Error(errors.New("executor state stale"), "bid skipped: cache exceeds intervals.executorStateMaxAgeMs",
			"opsAge", cacheAge(snapshot.UpdatedAt, now), "maxAge", s.cfg.ExecutorStateMaxAge)
		return bidDecision{skip: skipExecutorStateStale}
	}
	if snapshot.Exec.Locked {
		return bidDecision{skip: "signer_locked"}
	}
	if bigmath.OrZero(snapshot.Exec.Deposit).Cmp(minDeposit) < 0 {
		log.Info("bid skipped: executor deposit below minimum", "depositWei", snapshot.Exec.Deposit, "minDepositWei", minDeposit)
		return bidDecision{skip: skipDepositLow}
	}
	if s.strategy == nil {
		log.Error(errors.New("strategy is not configured"), "bid skipped")
		return bidDecision{skip: "strategy_error"}
	}
	gasPrice := bigmath.Clone(s.cfg.MaxTxGasPrice)
	decision, err := s.strategy.DecideBid(ctx, s.bidInput(auction, now, snapshot, s.inFlightSnapshot(), gasPrice))
	if err != nil {
		if ctx.Err() != nil {
			outcome := auctionOutcomeContextCanceled
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				outcome = auctionOutcomeTooLate
			}
			log.V(1).Info("strategy stopped with auction context", "reason", outcome)
			return bidDecision{skip: outcome}
		}
		log.Error(err, "strategy failed")
		return bidDecision{skip: "strategy_error"}
	}
	if err := checkExecutionEnvelope(decision); err != nil {
		log.Error(err, "execution envelope rejected")
		return bidDecision{skip: "strategy_invalid"}
	}
	if decision.Decision == types.DecisionSkip {
		return bidDecision{skip: types.BoundedSkipReason(decision.Reason), skipDetail: decision.Reason}
	}
	if limit := s.cfg.MaxBidWei; limit != nil && decision.BidAmount.Cmp(limit) > 0 {
		log.Info("bid skipped: strategy bid exceeds configured cap", "bidWei", decision.BidAmount, "maxBidWei", limit)
		return bidDecision{skip: skipBidCap}
	}
	prepared, err := s.signDecision(auction.ID, snapshot.Exec, decision, gasPrice)
	if err != nil {
		log.Error(err, "sign bid failed")
		return bidDecision{skip: "sign_error"}
	}
	return prepared
}

func (s *Solver) signDecision(auctionID string, executor ExecutorState, output types.BidOutput, gasPrice *big.Int) (bidDecision, error) {
	nonce, err := s.nonces.next(executor.Nonce.Uint64())
	if err != nil {
		return bidDecision{}, errors.Errorf("allocate bid nonce: %w", err)
	}
	prepared := bidDecision{nonce: nonce, callback: s.cfg.Callback, bidWei: bigmath.Clone(output.BidAmount)}
	nonceValue := new(big.Int).SetUint64(nonce)
	signature, err := SignBid(s.deps.Signer, s.chainID, prepared.callback, output.OperationData, prepared.bidWei, nonceValue, gasPrice)
	if err != nil {
		return bidDecision{}, err
	}
	prepared.solve = SolveMessage{Op: "solve", ID: auctionID, Data: SolveData{
		Bid: bigmath.Decimal(prepared.bidWei, 18), Nonce: nonceValue.String(), OperationCallback: prepared.callback.Hex(),
		OperationData: hexutil.Encode(output.OperationData), LiquidationSig: hexutil.Encode(signature), MaxTxGasPrice: gasPrice.String(),
	}}
	return prepared, nil
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
