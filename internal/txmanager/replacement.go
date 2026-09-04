package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/go-errors/errors"
)

// tryReplace reports whether this attempt entered cancellation mode, including deadline crossings
// during fee reads.
func (m *Manager) tryReplace(ctx context.Context, pending *pendingTransaction, cancellation bool) bool {
	if m.hasNonceConflict(pending.nonce) {
		return cancellation
	}
	cancellation = cancellation || pending.cancellationDue(time.Now())
	if !cancellation && m.rebroadcastUncertainAttempt(ctx, pending) {
		return false
	}
	limit := m.normalFeeLimit(pending.req)
	if cancellation {
		limit = m.globalFeeLimit()
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, limit)
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.tryReplace(ctx, pending, true)
	}
	if err != nil {
		if errors.Is(err, errReplacementLimitReached) &&
			m.rebroadcastLatestAttempt(ctx, pending, cancellation) {
			return cancellation
		}
		m.log.Error(
			err,
			"cannot replace pending transaction",
			"label",
			pending.req.Label,
			"nonce",
			pending.nonce,
			"cancellation",
			cancellation,
		)
		return cancellation
	}
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.tryReplace(ctx, pending, true)
	}
	return m.sendReplacement(ctx, pending, cancellation, fees)
}

func (m *Manager) sendReplacement(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
	fees feeQuote,
) bool {
	to := pending.req.To
	data := pending.req.Data
	value := new(big.Int)
	gas := pending.gas
	if cancellation {
		to = m.signer.Address()
		data = nil
		gas = cancellationGasLimit
	}
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.tryReplace(ctx, pending, true)
	}
	broadcastCtx, cancel := replacementBroadcastContext(ctx, pending, cancellation)
	signed, sendErr := m.signAndSend(
		broadcastCtx,
		pending.nonce,
		to,
		data,
		value,
		gas,
		fees,
		true,
	)
	cancel()
	if signed == nil {
		m.log.Error(
			sendErr,
			"pending transaction replacement rejected",
			"label",
			pending.req.Label,
			"nonce",
			pending.nonce,
			"cancellation",
			cancellation,
		)
		return cancellation
	}
	attempt := txAttempt{tx: signed, cancellation: cancellation}
	hash, uncertain := m.applyReplacementSend(pending, &attempt, &fees, sendErr)
	m.logAttempt("replacement", pending.req.Label, pending.nonce, hash, cancellation, sendErr, uncertain)
	kind := replacementKindReplacement
	if cancellation {
		kind = replacementKindCancellation
	}
	m.metrics.replacement(pending.req.Label, kind)
	return cancellation
}

func (m *Manager) rebroadcastUncertainAttempt(
	ctx context.Context,
	pending *pendingTransaction,
) bool {
	now := time.Now()
	if pending.cancellationDue(now) || !m.hasExactRebroadcastSlack(pending, now) ||
		len(pending.attempts) == 0 {
		return false
	}
	attempt := &pending.attempts[len(pending.attempts)-1]
	if attempt.cancellation || attempt.tx == nil || !attempt.exactRebroadcastPending {
		return false
	}
	attempt.exactRebroadcastPending = false
	err := m.sendSigned(ctx, attempt.tx, true)
	hash, uncertain := m.applyReplacementSend(pending, attempt, nil, err)
	m.logAttempt("exact rebroadcast", pending.req.Label, pending.nonce, hash, false, err, uncertain)
	return true
}

func (m *Manager) hasExactRebroadcastSlack(
	pending *pendingTransaction,
	now time.Time,
) bool {
	return pending.cancelDeadline.IsZero() ||
		pending.cancelDeadline.Sub(now) > m.cfg.BroadcastTimeout+m.cfg.ReplacementInterval
}

func (m *Manager) rebroadcastLatestAttempt(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) bool {
	for index := len(pending.attempts) - 1; index >= 0; index-- {
		attempt := pending.attempts[index]
		if attempt.cancellation != cancellation || attempt.tx == nil {
			continue
		}
		broadcastCtx, cancel := replacementBroadcastContext(ctx, pending, cancellation)
		err := m.sendSigned(broadcastCtx, attempt.tx, true)
		cancel()
		hash, uncertain := m.applyReplacementSend(pending, &attempt, nil, err)
		m.logAttempt("capped rebroadcast", pending.req.Label, pending.nonce, hash, cancellation, err, uncertain)
		return true
	}
	return false
}
