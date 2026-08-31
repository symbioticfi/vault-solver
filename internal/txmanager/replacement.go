package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

type replacementSendMode uint8

const (
	appendNewReplacementAttempt replacementSendMode = iota
	retryReplacementAttemptInPlace
)

type replacementSendOutcome struct {
	hash      common.Hash
	uncertain bool
}

func (m *Manager) tryReplace(ctx context.Context, pending *pendingTransaction, cancellation bool) {
	if m.hasNonceConflict(pending.nonce) {
		return
	}
	cancellation = cancellation || pending.cancellationDue(time.Now())
	if !cancellation && m.rebroadcastUncertainAttempt(ctx, pending) {
		return
	}
	limit := m.normalFeeLimit(pending.req)
	if cancellation {
		limit = m.globalFeeLimit()
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, limit)
	if !cancellation && pending.cancellationDue(time.Now()) {
		m.tryReplace(ctx, pending, true)
		return
	}
	if err != nil {
		if errors.Is(err, errReplacementLimitReached) &&
			m.rebroadcastLatestAttempt(ctx, pending, cancellation) {
			return
		}
		m.log.Error(err, "cannot replace pending transaction",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	to := pending.req.To
	data := pending.req.Data
	value := pending.value
	gas := pending.gas
	if cancellation {
		to = m.signer.Address()
		data = nil
		value = new(big.Int)
		gas = cancellationGasLimit
	}
	if !cancellation && pending.cancellationDue(time.Now()) {
		m.tryReplace(ctx, pending, true)
		return
	}
	sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
	signed, sendErr := m.signAndSend(sendCtx, pending.nonce, to, data, value, gas, fees, true)
	cancelSend()
	if signed == nil {
		m.log.Error(sendErr, "pending transaction replacement rejected",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	outcome := m.applyReplacementSend(
		ctx, pending, appendNewReplacementAttempt,
		txAttempt{tx: signed, cancellation: cancellation}, fees, sendErr,
	)
	if outcome.uncertain {
		m.log.Error(sendErr, "replacement broadcast uncertain; tracking signed hash",
			"label", pending.req.Label,
			"hash", outcome.hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	if sendErr != nil {
		m.log.Info("replacement already known by write RPC",
			"label", pending.req.Label,
			"hash", outcome.hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
			"rpcResult", sendErr.Error(),
		)
		return
	}
	m.log.Info("pending transaction replaced",
		"label", pending.req.Label,
		"hash", outcome.hash.Hex(),
		"nonce", pending.nonce,
		"cancellation", cancellation,
		"maxFeePerGas", fees.maxFee.String(),
		"maxPriorityFeePerGas", fees.tip.String(),
	)
}

// rebroadcastUncertainAttempt gives a transport-ambiguous normal submission one exact-byte retry
// before escalating its fees. It never appends a duplicate attempt or changes the cached fee state.
func (m *Manager) rebroadcastUncertainAttempt(ctx context.Context, pending *pendingTransaction) bool {
	now := time.Now()
	if pending.cancellationDue(now) || !m.hasExactRebroadcastSlack(pending, now) {
		return false
	}
	if len(pending.attempts) == 0 {
		return false
	}
	attempt := &pending.attempts[len(pending.attempts)-1]
	if attempt.cancellation || attempt.tx == nil || !attempt.exactRebroadcastPending {
		return false
	}
	attempt.exactRebroadcastPending = false
	err := m.sendSigned(ctx, attempt.tx, true)
	outcome := m.applyReplacementSend(
		ctx, pending, retryReplacementAttemptInPlace, *attempt, pending.fees, err,
	)
	switch {
	case err == nil:
		m.log.Info("uncertain transaction rebroadcast",
			"label", pending.req.Label,
			"hash", outcome.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
		)
	case !outcome.uncertain:
		m.log.Info("uncertain transaction already known by write RPC",
			"label", pending.req.Label,
			"hash", outcome.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
			"rpcResult", err.Error(),
		)
	default:
		m.log.Error(err, "uncertain transaction exact rebroadcast failed; replacement deferred",
			"label", pending.req.Label,
			"hash", outcome.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
		)
	}
	return true
}

func (m *Manager) hasExactRebroadcastSlack(pending *pendingTransaction, now time.Time) bool {
	if pending.cancelDeadline.IsZero() {
		return true
	}
	return pending.cancelDeadline.Sub(now) > m.cfg.BroadcastTimeout+m.cfg.ReplacementInterval
}

func (m *Manager) rebroadcastLatestAttempt(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) bool {
	for i := len(pending.attempts) - 1; i >= 0; i-- {
		attempt := pending.attempts[i]
		if attempt.cancellation != cancellation || attempt.tx == nil {
			continue
		}
		sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
		err := m.sendSigned(sendCtx, attempt.tx, true)
		cancelSend()
		outcome := m.applyReplacementSend(
			ctx, pending, retryReplacementAttemptInPlace, attempt, pending.fees, err,
		)
		if err != nil {
			m.log.Error(err, "capped transaction rebroadcast failed",
				"label", pending.req.Label,
				"hash", outcome.hash.Hex(),
				"nonce", pending.nonce,
				"cancellation", cancellation,
			)
		} else {
			m.log.Info("capped transaction rebroadcast",
				"label", pending.req.Label,
				"hash", outcome.hash.Hex(),
				"nonce", pending.nonce,
				"cancellation", cancellation,
			)
		}
		return true
	}
	return false
}

func (m *Manager) applyReplacementSend(
	ctx context.Context,
	pending *pendingTransaction,
	mode replacementSendMode,
	attempt txAttempt,
	fees feeQuote,
	sendErr error,
) replacementSendOutcome {
	outcome := replacementSendOutcome{
		hash:      attempt.hash,
		uncertain: sendErr != nil && !isKnownTransactionError(sendErr),
	}
	if mode == appendNewReplacementAttempt {
		outcome.hash = attempt.tx.Hash()
		attempt.hash = outcome.hash
		attempt.exactRebroadcastPending = outcome.uncertain
		pending.fees = cloneFeeQuote(fees)
		pending.attempts = append(pending.attempts, attempt)
	}
	if isNonceConsumedError(sendErr) {
		pending.nonceConflictHash = outcome.hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	return outcome
}

// Normal replacement broadcasts must not outlive the request's cancellation deadline. Cancellation
// transactions use the lifecycle context because the request deadline has already elapsed for them.
func replacementBroadcastContext(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) (context.Context, context.CancelFunc) {
	if cancellation || pending.cancelDeadline.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, pending.cancelDeadline)
}

func (m *Manager) nextReplacementFees(
	ctx context.Context,
	previous feeQuote,
	limit *big.Int,
) (feeQuote, error) {
	current, err := m.currentFees(ctx, nil)
	if err != nil && !errors.Is(err, errFreshFeesUnavailable) {
		return feeQuote{}, err
	}
	requiredTip := bumpFee(previous.tip)
	requiredMaxFee := bumpFee(previous.maxFee)
	next := feeQuote{
		baseFee: new(big.Int).Set(previous.baseFee),
		tip:     new(big.Int).Set(requiredTip),
		maxFee:  new(big.Int).Set(requiredMaxFee),
	}
	if err == nil {
		next.baseFee.Set(current.baseFee)
		next.maxFee = maxBigCopy(current.maxFee, next.maxFee)
	} else {
		m.log.V(1).Info("fresh replacement fees unavailable; using cached bump", "error", err)
	}
	if limit != nil && next.maxFee.Cmp(limit) > 0 {
		next.maxFee.Set(limit)
	}
	effectiveTipLimit := new(big.Int).Sub(next.maxFee, next.baseFee)
	if effectiveTipLimit.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"replacement base fee %s exceeds fee limit %s", next.baseFee, next.maxFee,
		)
	}
	if err == nil {
		freshTip := new(big.Int).Set(current.tip)
		if freshTip.Cmp(effectiveTipLimit) > 0 {
			freshTip.Set(effectiveTipLimit)
		}
		next.tip = maxBigCopy(freshTip, requiredTip)
	}
	if next.maxFee.Cmp(requiredMaxFee) < 0 || next.tip.Cmp(next.maxFee) > 0 {
		return feeQuote{}, errors.Errorf(
			"%w: previous max fee %s tip %s, limit %s",
			errReplacementLimitReached,
			previous.maxFee, previous.tip, feeLimitString(limit),
		)
	}
	return next, nil
}

func (m *Manager) normalFeeLimit(req Request) *big.Int {
	limit := reserveFeeBump(m.globalFeeLimit())
	if req.MaxFeePerGas != nil && (limit == nil || req.MaxFeePerGas.Cmp(limit) < 0) {
		limit = new(big.Int).Set(req.MaxFeePerGas)
	}
	return limit
}

func (m *Manager) globalFeeLimit() *big.Int {
	if m.cfg.MaxFeeGwei <= 0 {
		return nil
	}
	return gweiToWei(m.cfg.MaxFeeGwei)
}

func (m *Manager) trackUnminedTransaction(pending *pendingTransaction) {
	if pending.cancelDeadline.IsZero() {
		pending.cancelDeadline = m.cancellationDeadline(pending.req)
	}
	if pending.cancelRequested == nil {
		pending.cancelRequested = make(chan struct{})
	}
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	if m.unmined != nil {
		panic("txmanager: multiple signed lifecycles")
	}
	m.unmined = pending
}

func (m *Manager) removeUnminedTransaction(pending *pendingTransaction) {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	if m.unmined == pending {
		m.unmined = nil
	}
}

func (m *Manager) requestActiveCancellation() {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	if m.unmined != nil {
		requestCancellation(m.unmined)
	}
}

func (m *Manager) deliverActiveShutdownTimeout() {
	m.unminedMu.Lock()
	pending := m.unmined
	m.unminedMu.Unlock()
	if pending != nil {
		pending.deliver(Result{Hash: pending.originalHash, Err: errShutdownTimeout})
	}
}

func requestCancellation(pending *pendingTransaction) {
	pending.cancelOnce.Do(func() { close(pending.cancelRequested) })
}

func (pending *pendingTransaction) cancellationDue(now time.Time) bool {
	if !pending.cancelDeadline.IsZero() && !now.Before(pending.cancelDeadline) {
		return true
	}
	select {
	case <-pending.cancelRequested:
		return true
	default:
		return false
	}
}
