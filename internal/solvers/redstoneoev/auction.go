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
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

var weiPerEth = exp10(18)

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
	// err is the real failure behind skip, if any. skip still drives metrics and logging; err only
	// tells the bid span apart from an expected skip (spec §9.1).
	err error
}

func (s *Solver) handleMessage(ctx context.Context, raw []byte) {
	op, err := opName(raw)
	if err != nil {
		s.log.V(1).Error(err, "drop unparseable frame")
		return
	}
	switch op {
	case "auction":
		start := time.Now()
		if isFeedAuction(raw) {
			s.metrics.auctionDecision(auctionOutcomeFeedIgnored, time.Since(start))
			s.log.V(1).Info("ignoring feed auction")
			return
		}
		a, start, ok := s.parseAuctionFrame(raw)
		if !ok {
			return
		}
		if !s.auctionBusy.CompareAndSwap(false, true) {
			s.metrics.auctionDecision(types.SkipReasonInFlight, time.Since(start))
			return
		}
		s.auctionWorkers.Go(func() {
			defer s.auctionBusy.Store(false)
			s.handleAuction(ctx, a, start)
		})
	case "auction-result":
		s.handleAuctionResult(ctx, raw)
	case "liquidation-result":
		s.handleLiquidationResult(ctx, raw)
	case "blacklisted":
		s.handleBlacklisted(ctx, raw)
	default:
		s.log.V(1).Info("ignoring frame", "op", op)
	}
}

func (s *Solver) handleAuctionResult(ctx context.Context, raw []byte) {
	var r AuctionResult
	if err := json.Unmarshal(raw, &r); err != nil {
		s.log.V(1).Error(err, "drop malformed frame", "op", "auction-result")
		return
	}
	r.ID = normalizeAuctionID(r.ID)
	ctx, end, log := s.startResultSpan(ctx, "oev.auction.result", r.ID)
	defer func() { end(nil) }()

	liquidator := common.HexToAddress(r.Data.Liquidator)
	won := liquidator == s.cfg.Callback
	observability.SetAttributes(ctx, attrWon.Bool(won))
	if won {
		if bidWei, transitioned := s.markReservationWon(r.ID, time.Now()); transitioned {
			s.metrics.won(bidWei)
		}
	} else {
		s.releaseReservationByAuction(r.ID)
	}
	log.Info("auction-result", "winner", r.Data.Liquidator, "bid", r.Data.Bid, "won", won)
}

func (s *Solver) handleLiquidationResult(ctx context.Context, raw []byte) {
	var r LiquidationResult
	if err := json.Unmarshal(raw, &r); err != nil {
		s.log.V(1).Error(err, "drop malformed frame", "op", "liquidation-result")
		return
	}
	r.ID = normalizeAuctionID(r.ID)
	ctx, end, log := s.startResultSpan(ctx, "oev.liquidation.result", r.ID)
	defer func() { end(nil) }()
	if txHash := strings.TrimSpace(r.Data.TxHash); txHash != "" {
		observability.SetAttributes(ctx, observability.AttrTxHash.String(txHash))
	}

	liquidator := common.HexToAddress(r.Data.Liquidator)
	ours := liquidator == s.cfg.Callback
	log.Info("liquidation-result", "success", r.Data.Success,
		"txHash", r.Data.TxHash, "error", r.Data.Error, "ours", ours)
	if !ours {
		return
	}
	s.requestStateRefresh()
	lifecycleKey := r.ID
	if lifecycleKey == "" {
		lifecycleKey = liquidationResultIdentity(r)
	}
	transition := s.settleReservationByAuction(r.ID, lifecycleKey)
	if transition.won {
		s.metrics.won(transition.bidWei)
	}
	if transition.settled {
		s.metrics.settlement(r.Data.Success, transition.bidWei)
	}
	if !r.Data.Success {
		now := time.Now()
		identity := liquidationResultIdentity(r)
		recorded := true
		if identity == "" {
			s.breaker.recordFailure(now)
		} else {
			recorded = s.breaker.recordFailureOnce(identity, now)
		}
		if recorded {
			s.metrics.breakerFailure()
		}
	}
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

func (s *Solver) handleBlacklisted(ctx context.Context, raw []byte) {
	var b Blacklisted
	_ = json.Unmarshal(raw, &b)
	b.ID = normalizeAuctionID(b.ID)
	ctx, end, log := s.startResultSpan(ctx, "oev.blacklisted", b.ID)
	defer func() { end(nil) }()

	// Halting on a revoked key is an expected outcome of the feed, not a failure of this span.
	observability.Decline(ctx, "halted", "blacklisted")
	s.breaker.blacklist()
	log.Error(errors.New("api key blacklisted"), "halting bidding", "msg", b.Data.Msg)
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
	if a.TimeoutMs <= 0 {
		s.metrics.auctionDecision(auctionOutcomeTooLate, time.Since(start))
		s.log.V(1).Info("auction with invalid timeout received; dropping", "auctionId", a.ID, "timeoutMs", a.TimeoutMs)
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
		s.log.V(1).Info("duplicate auction; already processed", "auctionId", a.ID)
		return AuctionMessage{}, time.Time{}, false
	}
	return a, start, true
}

// handleAuction roots one auction's trace: the bid decision, its strategy stages and the outbound
// solve are all spans beneath it (spec §9.4).
func (s *Solver) handleAuction(ctx context.Context, a AuctionMessage, start time.Time) {
	ctx, end := tracer.Start(ctx, "oev.auction", observability.AttrAuctionID.String(a.ID))
	var err error
	defer func() { end(err) }()
	log := observability.TraceLogger(ctx, s.log).WithValues("auctionId", a.ID)

	outcome := auctionOutcomeContextCanceled
	defer func() {
		s.metrics.auctionDecision(outcome, time.Since(start))
	}()

	s.bidMu.Lock()
	defer s.bidMu.Unlock()
	if ctx.Err() != nil {
		err = ctx.Err()
		return
	}
	if s.bidExpired(log, a, start) {
		outcome = auctionOutcomeTooLate
		observability.Decline(ctx, "skipped", auctionOutcomeTooLate)
		return
	}
	bidCtx, cancel := auctionBidContext(ctx, a, start)
	defer cancel()
	d := s.buildBidWithContext(bidCtx, a, time.Now)

	if d.skip != "" {
		outcome = d.skip
		err = d.err
		s.logSkip(log, d)
		return
	}
	if s.dryRun {
		outcome = auctionOutcomeWouldBid
		observability.Decline(ctx, "suppressed", "dry_run")
		s.metrics.wouldBid(d.bidWei)
		log.Info("DRY-RUN would bid", "callback", d.callback.Hex(), "nonce", d.solve.Data.Nonce,
			"bidEth", d.solve.Data.Bid)
		return
	}
	if s.bidExpired(log, a, start) {
		outcome = auctionOutcomeTooLate
		observability.Decline(ctx, "skipped", auctionOutcomeTooLate)
		return
	}
	s.reserve(d.nonce, time.Now(), a.ID, d.bidWei)
	if !s.sendSolve(ctx, d.solve) {
		outcome = auctionOutcomeSendDropped
		s.releaseReservationByAuction(a.ID)
		log.Info("bid NOT enqueued (ws buffer full)", "nonce", d.solve.Data.Nonce)
		return
	}
	// The result frames arrive later in their own trace; remember this span so they can link back.
	s.rememberAuction(ctx, a.ID)
	outcome = auctionOutcomeEnqueued
	s.metrics.enqueuedBid(d.bidWei)
	log.Info("bid enqueued", "callback", d.callback.Hex(), "nonce", d.solve.Data.Nonce,
		"bidEth", d.solve.Data.Bid)
}

// sendSolve enqueues the solve under oev.auction.send and reports whether the bounded outbound queue
// accepted it. A dropped frame is an expected outcome of that bound, not a failure.
func (s *Solver) sendSolve(ctx context.Context, solve SolveMessage) bool {
	ctx, end := tracer.Start(ctx, "oev.auction.send")
	defer func() { end(nil) }()
	if s.ws.Send(marshal(solve)) {
		return true
	}
	observability.Decline(ctx, "dropped", "send_queue_full")
	return false
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

func (s *Solver) logSkip(log logr.Logger, d bidDecision) {
	if d.skipDetail != "" {
		log.V(1).Info("no bid", "reason", d.skip, "strategyReason", d.skipDetail)
		return
	}
	log.V(1).Info("no bid", "reason", d.skip)
}

func (s *Solver) bidExpired(log logr.Logger, a AuctionMessage, start time.Time) bool {
	now := time.Now()
	if a.TimeoutMs <= 0 || !tooLate(a.Timestamp, a.TimeoutMs, start, now) {
		return false
	}
	log.Info("bid not enqueued: auction deadline (since emit) exceeded",
		"timeoutMs", a.TimeoutMs, "sinceEmitMs", sinceEmitMs(a.Timestamp, now),
		"localElapsedMs", time.Since(start).Milliseconds())
	return true
}

// staleStateGate fails closed when the solver-owned Executor accounting is older than cfg.ExecutorStateMaxAge.
func (s *Solver) staleStateGate(log logr.Logger, now time.Time) string {
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
	log.Error(errors.New("executor state stale"), "bid skipped: cache exceeds intervals.executorStateMaxAgeMs",
		append(kv, "maxAge", s.cfg.ExecutorStateMaxAge)...)
	return skipExecutorStateStale
}

func cacheAge(at, now time.Time) string {
	if at.IsZero() {
		return "never"
	}
	return now.Sub(at).String()
}

func (s *Solver) buildBid(ctx context.Context, a AuctionMessage, nowFn func() time.Time) bidDecision {
	return s.buildBidWithContext(ctx, a, nowFn)
}

// buildBidWithContext evaluates one auction under oev.auction.bid. An expected skip is declined on the
// span; a real failure (misconfigured or failing strategy, bad envelope, signing) is recorded as an error.
func (s *Solver) buildBidWithContext(ctx context.Context, a AuctionMessage, nowFn func() time.Time) (d bidDecision) {
	ctx, end := tracer.Start(ctx, "oev.auction.bid",
		observability.AttrAuctionID.String(a.ID), observability.AttrStrategy.String(s.strategyLabel()))
	defer func() {
		if d.err == nil && d.skip != "" {
			observability.Decline(ctx, "skipped", d.skip)
		}
		end(d.err)
	}()
	log := observability.TraceLogger(ctx, s.log).WithValues("auctionId", a.ID)

	now := nowFn()
	if tripped, _ := s.breaker.tripped(now); tripped {
		return bidDecision{skip: "breaker"}
	}
	if skip := s.staleStateGate(log, now); skip != "" {
		return bidDecision{skip: skip}
	}
	st, ok := s.state.load()
	if !ok {
		return bidDecision{skip: "state_unknown"}
	}
	if st.Exec.Locked {
		return bidDecision{skip: "signer_locked"}
	}
	if depositSkip := s.depositSkip(log, st); depositSkip != "" {
		return bidDecision{skip: depositSkip}
	}
	inFlight := s.inFlightSnapshot()
	gasPrice := new(big.Int).Set(s.cfg.MaxTxGasPrice)
	if s.strategy == nil {
		unconfigured := errors.New("strategy is not configured")
		log.Error(unconfigured, "bid skipped")
		return bidDecision{skip: "strategy_error", err: unconfigured}
	}
	out, err := s.strategy.DecideBid(ctx, s.bidInput(a, now, st, inFlight, gasPrice))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			outcome := auctionOutcomeContextCanceled
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				outcome = auctionOutcomeTooLate
			}
			log.V(1).Info("strategy stopped with auction context", "reason", outcome)
			return bidDecision{skip: outcome}
		}
		log.Error(err, "strategy failed")
		return bidDecision{skip: "strategy_error", err: err}
	}
	if err := checkExecutionEnvelope(out); err != nil {
		log.Error(err, "execution envelope rejected")
		return bidDecision{skip: "strategy_invalid", err: err}
	}
	if out.Decision == types.DecisionSkip {
		return bidDecision{skip: types.BoundedSkipReason(out.Reason), skipDetail: out.Reason}
	}
	bidNative := cloneBig(out.BidAmount)
	if s.bidCapExceeded(log, bidNative) {
		return bidDecision{skip: skipBidCap}
	}
	nonce := s.nonces.next(st.Exec.Nonce.Uint64())
	callback := s.cfg.Callback
	sig, err := SignBid(s.deps.Signer, s.chainID, callback, out.OperationData, bidNative, big.NewInt(int64(nonce)), gasPrice)
	if err != nil {
		log.Error(err, "sign bid failed")
		return bidDecision{skip: "sign_error", err: err}
	}

	return bidDecision{
		nonce:    nonce,
		callback: callback,
		bidWei:   bidNative,
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

func (s *Solver) bidCapExceeded(log logr.Logger, bidNative *big.Int) bool {
	if s.cfg.MaxBidWei == nil || bidNative.Cmp(s.cfg.MaxBidWei) <= 0 {
		return false
	}
	log.Info("bid skipped: strategy bid exceeds configured cap",
		"bidWei", bidNative, "maxBidWei", s.cfg.MaxBidWei)
	return true
}

func (s *Solver) depositSkip(log logr.Logger, st cachedState) string {
	if orZero(st.Exec.Deposit).Cmp(minDeposit) < 0 {
		log.Info("bid skipped: executor deposit below minimum",
			"depositWei", st.Exec.Deposit, "minDepositWei", minDeposit)
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
