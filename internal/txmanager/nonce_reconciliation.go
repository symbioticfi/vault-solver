package txmanager

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// replacementNonceAvailable checks mined state before every replacement path,
// including exact rebroadcasts. A private relay may acknowledge a stale nonce
// without returning "nonce too low". Pending nonce advancement is insufficient:
// our own unmined submission can advance it.
func (m *Manager) replacementNonceAvailable(ctx context.Context, pending *pendingTransaction) (bool, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	latest, err := m.backend.NonceAt(lookupCtx, m.signer.Address(), nil)
	cancel()
	log := observability.Log(ctx)
	if err != nil {
		err = errors.Errorf("latest mined nonce before replacement: %w", err)
		if ctx.Err() == nil {
			pending.nonceReads.failed(log, err, "pending transaction nonce unavailable; deferring replacement",
				"label", pending.req.Label, "hash", pending.originalHash.Hex(), "nonce", pending.nonce)
		}
		return false, err
	}
	pending.nonceReads.recovered(log, "pending transaction nonce reads recovered",
		"label", pending.req.Label, "nonce", pending.nonce)
	if latest <= pending.nonce {
		return true, nil
	}
	if pending.nonceConflictHash == (common.Hash{}) {
		log.Info("pending transaction nonce consumed; reconciling tracked receipts",
			"label", pending.req.Label, "hash", pending.originalHash.Hex(),
			"sender", m.signer.Address().Hex(), "nonce", pending.nonce, "latestNonce", latest)
	}
	pending.nonceConflictHash = pending.originalHash
	m.reconcileExistingLifecycleNonce(ctx, pending)
	// Even a canonical owned receipt means replacement must stop. The receipt
	// reader retains ownership and applies the request's confirmation policy.
	return false, nil
}
