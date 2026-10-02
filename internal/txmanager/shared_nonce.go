package txmanager

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// freshPendingNonce reads before every initial signing. There is no local nonce cache or replica
// coordination: other senders' pending work advances this view, and simultaneous readers may race.
func (m *Manager) freshPendingNonce(ctx context.Context) (uint64, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	nonce, err := m.backend.PendingNonceAt(lookupCtx, m.signer.Address())
	if err != nil {
		return 0, errors.Errorf("pending nonce before signing: %w", err)
	}
	if nonce == ^uint64(0) {
		return 0, errors.New("account nonce is exhausted")
	}
	m.mu.Lock()
	changed := !m.initialized
	m.initialized = true
	m.mu.Unlock()
	if changed {
		m.notifyLaneStateChange()
	}
	return nonce, nil
}

// confirmConsumedNonce runs only after every owned attempt returned NotFound. A higher mined nonce
// is enough to stop replacing obsolete bytes, but cannot prove which order executed. Receipt lag or
// reorgs are possible; the solver must reconcile protocol state before rebuilding any business call.
func (m *Manager) confirmConsumedNonce(ctx context.Context, pending *pendingTransaction) (Result, bool) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	latest, err := m.backend.NonceAt(lookupCtx, m.signer.Address(), nil)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			pending.nonceReads.failed(observability.Log(ctx), err, "mined nonce RPC unavailable", "nonce", pending.nonce)
		}
		return Result{}, false
	}
	pending.nonceReads.recovered(observability.Log(ctx), "mined nonce RPC recovered", "nonce", pending.nonce)
	if latest <= pending.nonce {
		return Result{}, false
	}
	return Result{
		Hash: pending.originalHash, Outcome: OutcomeNonceConsumed,
		Err: errors.Errorf("send %q at nonce %d: %w", pending.req.Label, pending.nonce, ErrNonceConsumed),
	}, true
}
