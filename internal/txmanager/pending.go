package txmanager

import (
	"context"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// Under the horizon fee policy a pending lifecycle is driven by block evidence instead of timed fee bumps
// (strategy §2.7, §2.8; docs/TXMANAGER-PLAN.md §4.4). The replacement ticker fires every blockTime/2. Each
// tick reads one fee snapshot covering every block since the latest attempt was sent and, only at a new
// head, applies evaluatePending: hold, reprice (validity, congestion, or stall once its exact rebroadcasts
// did not land it), or the stall response. A reprice or stall of the call re-estimates its gas first, off
// the tick: a revert cancels the lifecycle once the mined nonce is known to be free, a stall estimate above
// the gas limit replaces the gas limit, and otherwise a stall rebroadcasts the exact bytes. A pending
// cancellation follows the same rules without re-estimation. A reorg that removes an inclusion restarts the
// miss count and rebroadcasts the included bytes at once. When the evaluation reads fail for
// replacementIntervalMs, the legacy cached bump takes over, once per replacement interval, until they recover.
//
// Goroutine model: a pendingEvaluator belongs to its lifecycle goroutine, which calls its methods from the
// waitForPendingTransaction select loop only. At most one re-estimate runs at a time, on its own goroutine;
// that goroutine writes the estimate's result fields and then closes done, and the lifecycle goroutine reads
// them only after done is closed.

// pendingHistoryMargin is how many blocks beyond those since the latest send or stall rebroadcast an
// evaluation's fee history covers (strategy §2.7: blocksSinceSend + 3), so a skipped head still counts.
const pendingHistoryMargin = 3

// maxFeeHistoryBlocks is the most blocks one eth_feeHistory call serves.
const maxFeeHistoryBlocks = 1024

// cancelReasonSimulatedRevert is the cancellation reason when a pending call's re-estimate reverts.
const cancelReasonSimulatedRevert = "simulated_revert"

// Values of the txmanager.evaluate span's pending.decision attribute beyond the evaluatePending actions.
const (
	evaluationUncertain = "uncertain_rebroadcast" // an ambiguous broadcast got its one exact retry
	evaluationNoNewHead = "no_new_head"           // the head did not advance, or a lagging upstream went back
	evaluationStaleHead = "stale_head"            // the new head trails the real next block too far to judge
)

// pendingEvaluator is the horizon policy's state of one pending lifecycle.
type pendingEvaluator struct {
	m       *Manager
	pending *pendingTransaction
	actions pendingActions

	// maxSeenHead is the highest head (header or fee history) an evaluation read reported; a read below it
	// comes from a lagging upstream and is ignored. evaluatedHead is the newest fee-history block decided
	// on, so each head is decided once and at most one fee-changing send follows it.
	maxSeenHead   uint64
	evaluatedHead uint64
	// sentAt is the head the latest attempt was sent at, stallBase the head since which its roomy misses
	// count (sentAt, or its latest stall rebroadcast), and rebroadcasts its stall rebroadcasts. attempts is
	// how many attempts the lifecycle had when they were set: a new attempt resets them.
	sentAt       uint64
	stallBase    uint64
	rebroadcasts int
	attempts     int

	// reads is the failure streak of the evaluation reads, failingSince when the current run of failures
	// began (zero after a successful read), and lastFallback when the fallback last replaced.
	reads        readStreak
	failingSince time.Time
	lastFallback time.Time

	// estimate is the re-estimate in flight, or nil; no evaluation runs while it is set.
	estimate *pendingEstimate
}

// pendingActions are the lifecycle loop's own steps an evaluator drives. replace signs the next attempt
// under a txmanager.replace span and starts cancellation if a deadline promoted it; cancel starts
// cancellation for a reason and signs its first attempt.
type pendingActions struct {
	replace func(cancellation bool, plan *replacementPlan)
	cancel  func(reason string)
}

// pendingEstimate is a pending call's re-estimate, run for decision against snapshot's header.
type pendingEstimate struct {
	done     chan struct{}
	cancel   context.CancelCauseFunc
	decision pendingDecision
	snapshot *feeSnapshot

	result horizonGasEstimate
	err    error
}

// reorgedInclusion is an attempt whose inclusion in block a reorg removed.
type reorgedInclusion struct {
	attempt txAttempt
	block   uint64
}

func (m *Manager) newPendingEvaluator(pending *pendingTransaction, actions pendingActions) *pendingEvaluator {
	return &pendingEvaluator{
		m:             m,
		pending:       pending,
		actions:       actions,
		maxSeenHead:   pending.sendSeen,
		evaluatedHead: pending.sendSeen,
		sentAt:        pending.sendSeen,
		stallBase:     pending.sendSeen,
		attempts:      len(pending.attempts),
		reads:         readStreak{quietStart: true},
	}
}

// estimated is closed when the re-estimate in flight has finished; nil (never ready) without one.
func (ev *pendingEvaluator) estimated() <-chan struct{} {
	if ev == nil || ev.estimate == nil {
		return nil
	}
	return ev.estimate.done
}

// tick runs on every replacement tick of a lifecycle that is not about to start its deadline cancellation.
// A cancellation whose first attempt is not signed yet is retried instead (strategy §2.7 rule 1); otherwise
// the latest attempt is evaluated, unless a re-estimate is in flight or the nonce is in conflict.
func (ev *pendingEvaluator) tick(ctx context.Context, cancelling bool) {
	ev.syncAttempts()
	if ev.estimate != nil || ev.m.hasNonceConflict(ev.pending.nonce) {
		return
	}
	if cancelling && !hasCancellationAttempt(ev.pending.attempts) {
		ev.actions.replace(true, nil)
		return
	}
	// The span carries a failed read's error; the read streak logs it.
	_ = ev.evaluate(ctx, cancelling)
}

// evaluate is one evaluation tick, the span txmanager.evaluate. An ambiguous broadcast first gets its one
// exact retry, as on the legacy path. Then one fee snapshot is read; a failed read counts toward the
// fallback. At a new, fresh head the latest attempt is decided on and acted upon.
func (ev *pendingEvaluator) evaluate(ctx context.Context, cancelling bool) (err error) {
	ctx, end := tracer.Start(ctx, "txmanager.evaluate", attribute.Bool("tx.cancellation", cancelling))
	defer func() { end(err) }()
	m, pending := ev.m, ev.pending
	now := time.Now()
	if !cancelling && m.uncertainRebroadcastDue(pending, now) {
		observability.SetAttributes(ctx, attribute.String("pending.decision", evaluationUncertain))
		available, err := m.replacementNonceAvailable(ctx, pending)
		if err != nil || !available {
			return err
		}
		m.rebroadcastUncertainAttempt(ctx, pending)
		return nil
	}
	snapshot, err := ev.read(ctx)
	if err != nil {
		ev.readFailed(ctx, cancelling, now, err)
		return err
	}
	ev.readRecovered(ctx)
	head := snapshot.historyHead
	seen := max(head, headerNumber(snapshot.header))
	observability.SetAttributes(ctx, attribute.Int64("fee.head", int64(head)))
	if seen < ev.maxSeenHead || head <= ev.evaluatedHead {
		observability.SetAttributes(ctx, attribute.String("pending.decision", evaluationNoNewHead))
		return nil
	}
	ev.maxSeenHead, ev.evaluatedHead = seen, head
	lag := snapshot.lag(time.Now(), m.cfg.Fees.BlockTime)
	if lag > m.maxHeadLagBlocks() {
		// Misses counted against a head this far behind are real, but pricing from it is not: wait for a
		// fresh one, as a new send does.
		observability.SetAttributes(ctx, attribute.String("pending.decision", evaluationStaleHead))
		observability.Log(ctx).V(1).Info("pending transaction evaluation head is stale; holding",
			"label", pending.req.Label, "nonce", pending.nonce, "head", head, "headLagBlocks", lag)
		return nil
	}
	gas := pending.gas
	if cancelling {
		gas = cancellationGasLimit
	}
	decision := evaluatePending(pendingInput{
		fees: pending.fees, gas: gas, sentAt: ev.sentAt, stallBase: ev.stallBase, rebroadcasts: ev.rebroadcasts, lag: lag,
	}, snapshot, m.horizon)
	observability.SetAttributes(ctx,
		attribute.String("pending.decision", decision.action.String()),
		attribute.String("pending.reason", decision.reason),
		attribute.Int64("pending.full_misses", int64(decision.fullMisses)),
		attribute.Int64("pending.roomy_misses", int64(decision.roomyMisses)),
	)
	fields := []any{
		"label", pending.req.Label, "nonce", pending.nonce, "cancellation", cancelling, "head", head,
		"blocksSinceSend", head - min(head, ev.sentAt), "fullMisses", decision.fullMisses, "roomyMisses", decision.roomyMisses,
		"stallRebroadcasts", ev.rebroadcasts, "nextBaseFee", snapshot.nextBase.String(), "maxFeePerGas", pending.fees.maxFee.String(),
	}
	switch decision.action {
	case pendingHold:
		observability.Log(ctx).V(1).Info("pending transaction held", fields...)
	case pendingReprice:
		observability.Log(ctx).V(1).Info("pending transaction repricing", append(fields, "reason", decision.reason)...)
		ev.respond(ctx, cancelling, decision, snapshot)
	case pendingStall:
		observability.Log(ctx).Info("pending transaction stalled", fields...)
		ev.respond(ctx, cancelling, decision, snapshot)
	}
	return nil
}

// respond acts on a reprice or stall decision. A cancellation, and a call whose gas limit the request
// supplied, are not re-estimated: they reprice at once, or a stall rebroadcasts them. A call reprices or
// answers its stall only once its re-estimate returns (finishEstimate).
func (ev *pendingEvaluator) respond(ctx context.Context, cancelling bool, decision pendingDecision, snapshot *feeSnapshot) {
	if !cancelling && ev.pending.req.GasLimit == 0 {
		ev.startEstimate(ctx, decision, snapshot)
		return
	}
	if decision.action == pendingStall {
		ev.stallRebroadcast(ctx, cancelling)
		return
	}
	ev.actions.replace(cancelling, &replacementPlan{snapshot: snapshot, reason: decision.reason, cancellation: cancelling})
}

// startEstimate re-estimates the pending call on top of the snapshot's header, in the next block's context
// as a new send is estimated, bounded by gas.estimateTimeoutMs. It runs on its own goroutine so the
// lifecycle keeps reading receipts and meeting its deadline meanwhile.
func (ev *pendingEvaluator) startEstimate(ctx context.Context, decision pendingDecision, snapshot *feeSnapshot) {
	estimateCtx, cancel := context.WithCancelCause(ctx)
	estimate := &pendingEstimate{done: make(chan struct{}), cancel: cancel, decision: decision, snapshot: snapshot}
	req := ev.pending.req
	go func() {
		defer close(estimate.done)
		estimate.result, estimate.err = ev.m.horizonEstimate(estimateCtx, req, snapshot.header)
	}()
	ev.estimate = estimate
}

// finishEstimate acts on the re-estimate that just finished (strategy §2.2.6, §2.7): a revert cancels the
// lifecycle once the mined nonce is known to be free; a stall whose estimate exceeds the gas limit signs a
// gas-limit replacement at bumped fees; any other stall rebroadcasts the exact bytes; and a reprice signs at
// max(gasLimit, estimate + headroom). An estimate that failed otherwise tells nothing: a stall rebroadcasts
// and a reprice keeps the gas limit. A cancellation that began meanwhile drops the result.
func (ev *pendingEvaluator) finishEstimate(ctx context.Context, cancelling bool) {
	estimate := ev.estimate
	if estimate == nil {
		return
	}
	ev.estimate = nil
	<-estimate.done
	estimate.cancel(nil)
	if cancelling {
		return
	}
	pending := ev.pending
	stall := estimate.decision.action == pendingStall
	var limit uint64
	err := estimate.err
	if err == nil {
		limit, err = gasLimitWithHeadroom(estimate.result.gas, estimate.result.headroomBps)
	}
	fields := []any{"label", pending.req.Label, "nonce", pending.nonce, "gasLimit", pending.gas, "estimateMode", estimate.result.mode}
	switch {
	case err != nil && chain.IsExecutionReverted(err):
		ev.simulatedRevert(ctx, estimate)
	case err != nil:
		observability.Log(ctx).Info("pending transaction re-estimate unavailable; keeping its gas limit",
			append(fields, "error", err.Error())...)
		if stall {
			ev.stallRebroadcast(ctx, false)
			return
		}
		ev.actions.replace(false, &replacementPlan{snapshot: estimate.snapshot, reason: estimate.decision.reason})
	case stall && estimate.result.gas > pending.gas:
		// Its whole headroom is used up, which estimate noise (at most about 1%) does not explain: a fill on
		// the same vault landed first. Without more gas the call can only run out of it.
		observability.Log(ctx).Info("pending transaction re-estimate exceeds its gas limit; replacing the gas limit",
			append(fields, "estimate", estimate.result.gas, "newGasLimit", limit)...)
		ev.actions.replace(false, &replacementPlan{snapshot: estimate.snapshot, gas: limit, reason: repriceGas})
	case stall:
		ev.stallRebroadcast(ctx, false)
	default:
		ev.actions.replace(false, &replacementPlan{
			snapshot: estimate.snapshot, gas: max(pending.gas, limit), reason: estimate.decision.reason,
		})
	}
}

// simulatedRevert cancels a pending call whose re-estimate reverts: it cannot succeed, so repricing or
// waiting only holds the lane and wastes fees (strategy §2.2.6). The mined nonce is checked first: a call
// that already landed reverts when simulated again, and its receipt, not a cancellation, ends the lifecycle.
// An unreadable nonce defers the decision to the next head.
func (ev *pendingEvaluator) simulatedRevert(ctx context.Context, estimate *pendingEstimate) {
	pending := ev.pending
	fields := []any{
		"label", pending.req.Label, "hash", pending.originalHash.Hex(), "nonce", pending.nonce,
		"trigger", estimate.decision.action.String(), "estimateMode", estimate.result.mode, "error", estimate.err.Error(),
	}
	available, err := ev.m.replacementNonceAvailable(ctx, pending)
	if err != nil || !available {
		observability.Log(ctx).Info("pending transaction re-estimate reverts but its nonce is mined or unreadable; not cancelling",
			fields...)
		return
	}
	observability.Log(ctx).Info("pending transaction re-estimate reverts; cancelling", fields...)
	ev.actions.cancel(cancelReasonSimulatedRevert)
}

// stallRebroadcast sends the latest attempt's exact bytes again ("already known" counts as sent) and
// restarts the roomy-miss count from the current head. A call is not rebroadcast once its deadline
// cancellation is due or too near for the rebroadcast to matter (hasExactRebroadcastSlack).
func (ev *pendingEvaluator) stallRebroadcast(ctx context.Context, cancelling bool) {
	m, pending := ev.m, ev.pending
	if now := time.Now(); !cancelling && (pending.cancellationDue(now) || !m.hasExactRebroadcastSlack(pending, now)) {
		observability.Log(ctx).V(1).Info("pending transaction stalled near its cancellation deadline; not rebroadcasting",
			"label", pending.req.Label, "nonce", pending.nonce)
		return
	}
	attempt, ok := latestAttempt(pending.attempts, cancelling)
	if !ok {
		return
	}
	if available, err := m.replacementNonceAvailable(ctx, pending); err != nil || !available {
		return
	}
	if m.rebroadcastExact(ctx, pending, attempt, rebroadcastStall) {
		ev.rebroadcasts++
		ev.stallBase = ev.maxSeenHead
	}
}

// reorged handles a reorg that removed an attempt's inclusion (strategy §2.7): the blocks it was included
// in are not misses, so the counts restart from the current head, and its exact bytes go straight back to
// the endpoint it used, since a private relay may have dropped a transaction it saw included. The mined
// nonce is checked first, as before every rebroadcast.
func (ev *pendingEvaluator) reorged(ctx context.Context, inclusion reorgedInclusion) {
	ev.abandonEstimate()
	ev.maxSeenHead = max(ev.maxSeenHead, inclusion.block)
	ev.attempts = len(ev.pending.attempts)
	ev.sentAt, ev.stallBase, ev.rebroadcasts = ev.maxSeenHead, ev.maxSeenHead, 0
	if inclusion.attempt.tx == nil {
		return
	}
	if available, err := ev.m.replacementNonceAvailable(ctx, ev.pending); err != nil || !available {
		return
	}
	ev.m.rebroadcastExact(ctx, ev.pending, inclusion.attempt, rebroadcastReorg)
}

// read reads the evaluation's fee snapshot: the shared snapshot when its history covers every block since
// the latest send or stall rebroadcast plus the margin, and otherwise a longer history of its own.
func (ev *pendingEvaluator) read(ctx context.Context) (*feeSnapshot, error) {
	m := ev.m
	blocks := max(ev.maxSeenHead-ev.stallBase, m.cfg.Fees.EscalateAfterFullMisses) + pendingHistoryMargin
	if blocks <= m.snapshotBlocks() {
		return m.snapshots.get(ctx, 0)
	}
	return m.readFeeSnapshot(ctx, min(blocks, maxFeeHistoryBlocks))
}

// readFailed records a failed evaluation read. Once reads have failed for replacementIntervalMs, the legacy
// cached bump, which is also balance-capped, replaces the latest attempt once per replacement interval
// until they recover (strategy §2.7 fallback).
func (ev *pendingEvaluator) readFailed(ctx context.Context, cancelling bool, now time.Time, err error) {
	pending := ev.pending
	ev.reads.failed(observability.Log(ctx), err, "pending transaction evaluation reads unavailable",
		"label", pending.req.Label, "nonce", pending.nonce, "cancellation", cancelling)
	if ev.failingSince.IsZero() {
		ev.failingSince = now
	}
	interval := ev.m.cfg.ReplacementInterval
	if now.Sub(ev.failingSince) < interval || now.Sub(ev.lastFallback) < interval {
		return
	}
	ev.lastFallback = now
	ev.actions.replace(cancelling, &replacementPlan{reason: repriceFallback, cancellation: cancelling})
}

func (ev *pendingEvaluator) readRecovered(ctx context.Context) {
	ev.reads.recovered(observability.Log(ctx), "pending transaction evaluation reads recovered",
		"label", ev.pending.req.Label, "nonce", ev.pending.nonce)
	ev.failingSince = time.Time{}
}

// syncAttempts restarts the counts when an attempt was signed since the last tick, by any path: it was sent
// at the newest head seen.
func (ev *pendingEvaluator) syncAttempts() {
	if count := len(ev.pending.attempts); count != ev.attempts {
		ev.attempts = count
		ev.sentAt, ev.stallBase, ev.rebroadcasts = ev.maxSeenHead, ev.maxSeenHead, 0
	}
}

// abandonEstimate cancels the re-estimate in flight, if any, and waits for its goroutine. An abandoned
// estimate is not an estimate outcome, so gas_estimates_total does not count it.
func (ev *pendingEvaluator) abandonEstimate() {
	if ev == nil || ev.estimate == nil {
		return
	}
	ev.estimate.cancel(errEstimateAbandoned)
	<-ev.estimate.done
	ev.estimate = nil
}

// String names the action for the txmanager.evaluate span and logs.
func (a pendingAction) String() string {
	switch a {
	case pendingHold:
		return "hold"
	case pendingReprice:
		return "reprice"
	case pendingStall:
		return "stall"
	default:
		return "unknown"
	}
}

// rebroadcastExact sends an attempt's exact signed bytes again to the endpoint it used (the cancellation
// route for a cancellation), within the request deadline for a call. It signs nothing, so the attempts and
// fees are unchanged. It reports whether the endpoint accepted the bytes or already knew them; a failure is
// logged at Info, since the next stall trigger retries it.
func (m *Manager) rebroadcastExact(ctx context.Context, pending *pendingTransaction, attempt txAttempt, reason string) bool {
	sendCtx, cancel := replacementBroadcastContext(ctx, pending, attempt.cancellation)
	err := m.sendSigned(sendCtx, attempt.tx, true, attempt.cancellation)
	cancel()
	if isNonceConsumedError(err) {
		pending.nonceConflictHash = attempt.hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	fields := []any{
		"label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce,
		"cancellation", attempt.cancellation, "reason", reason,
	}
	if err != nil && !isKnownTransactionError(err) {
		observability.Log(ctx).Info("pending transaction rebroadcast failed", append(fields, "error", err.Error())...)
		return false
	}
	m.metrics.rebroadcast(pending.req.Label, reason)
	if err != nil {
		fields = append(fields, "rpcResult", err.Error())
	}
	observability.Log(ctx).Info("pending transaction rebroadcast", fields...)
	return true
}

// latestAttempt is the latest signed attempt of the kind: the call's or the cancellation's.
func latestAttempt(attempts []txAttempt, cancellation bool) (txAttempt, bool) {
	for _, attempt := range slices.Backward(attempts) {
		if attempt.cancellation == cancellation && attempt.tx != nil {
			return attempt, true
		}
	}
	return txAttempt{}, false
}

// hasCancellationAttempt reports whether a cancellation was signed.
func hasCancellationAttempt(attempts []txAttempt) bool {
	_, ok := latestAttempt(attempts, true)
	return ok
}
