package txmanager

import (
	"context"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

// Rebroadcast resubmits already-journaled signed bytes for the lane's current
// owner. It never signs or picks a nonce; the journal owner reconciles receipts.
func (m *Manager) Rebroadcast(ctx context.Context, owner string, tx *types.Transaction) error {
	if tx == nil || tx.ChainId().Cmp(m.chainID) != 0 {
		return errors.New("rebroadcast transaction chain mismatch")
	}
	from, err := types.Sender(types.LatestSignerForChainID(m.chainID), tx)
	if err != nil || from != m.signer.Address() {
		return errors.New("rebroadcast transaction signer mismatch")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case m.lifecycleSlot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stopping:
		return errManagerStopped
	}
	defer func() { <-m.lifecycleSlot; m.notifyLaneStateChange() }()
	m.reservationMu.Lock()
	allowed := owner != "" && m.reservation == owner
	m.reservationMu.Unlock()
	if !allowed {
		return ErrLaneReserved
	}
	select {
	case <-m.stopping:
		return errManagerStopped
	default:
	}
	if err := m.sendSigned(ctx, tx); err != nil {
		return errors.Errorf("rebroadcast journaled transaction: %w", err)
	}
	return nil
}
