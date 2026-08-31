package txmanager

import (
	"context"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

// reconcileExistingLifecycleNonce distinguishes the benign race where one of this lifecycle's
// exact signed attempts was included just before a replacement from unexplained nonce consumption.
// Only a receipt proven canonical against a stable head can resume the lane.
func (m *Manager) reconcileExistingLifecycleNonce(ctx context.Context, pending *pendingTransaction) {
	if m.hasCanonicalTrackedReceipt(ctx, pending) {
		m.clearNonceConflict(pending.nonce)
		return
	}
	m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
}

func (m *Manager) hasCanonicalTrackedReceipt(ctx context.Context, pending *pendingTransaction) bool {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	for _, attempt := range pending.attempts {
		receipt, err := m.backend.TransactionReceipt(lookupCtx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			m.log.Error(err, "tracked receipt unavailable during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			m.log.Error(err, "invalid tracked receipt during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := m.confirmCanonicalReceipt(lookupCtx, receipt); err != nil {
			m.log.Error(err, "tracked receipt is not canonically visible during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		return true
	}
	return false
}

// isDefiniteBroadcastRejection is intentionally narrow. Once bytes have been signed and submitted,
// transport, decoding, nonce, fee, and generic RPC errors are ambiguous and the exact hash must stay
// tracked. These validation failures cannot have entered a node's transaction pool and cannot be
// repaired by replacing the same lifecycle.
func isDefiniteBroadcastRejection(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "insufficient funds") ||
		strings.Contains(message, "intrinsic gas too low") ||
		strings.Contains(message, "invalid sender") ||
		strings.Contains(message, "transaction type not supported")
}

func isNonceConsumedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "nonce too low") ||
		strings.Contains(message, "nonce is too low") ||
		strings.Contains(message, "nonce has already been used")
}

func isKnownTransactionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already known")
}

func isPendingNonceCollision(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "replacement transaction underpriced")
}

// nextNonce returns the nonce to use, failing closed if startup discovers an unknown pending
// transaction that the in-memory manager cannot safely replace or cancel.
func (m *Manager) nextNonce(ctx context.Context) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.nonceConflictErrorLocked(); err != nil {
		return 0, err
	}
	if err := m.initializeNonceLocked(ctx); err != nil {
		return 0, err
	}
	return m.nonce, nil
}

func (m *Manager) markNonceConflict(nonce uint64, hash common.Hash) {
	m.mu.Lock()
	if m.conflict != nil && m.conflict.nonce != nonce {
		panic("txmanager: multiple nonce conflicts")
	}
	first := m.conflict == nil
	m.conflict = &nonceConflict{nonce: nonce, hash: hash}
	m.mu.Unlock()
	if first {
		m.notifyLaneStateChange()
		m.log.Error(errors.New("nonce ownership is uncertain"),
			"transaction manager paused pending nonce reconciliation",
			"nonce", nonce,
			"hash", hash.Hex(),
		)
	}
}

func (m *Manager) clearNonceConflict(nonce uint64) {
	m.mu.Lock()
	existed := m.conflict != nil && m.conflict.nonce == nonce
	if existed {
		m.conflict = nil
	}
	m.mu.Unlock()
	if existed {
		m.notifyLaneStateChange()
	}
}

func (m *Manager) notifyLaneStateChange() {
	m.laneStateMu.Lock()
	defer m.laneStateMu.Unlock()
	for _, changes := range m.laneStateSubscribers {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) nonceConflictError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nonceConflictErrorLocked()
}

func (m *Manager) nonceConflictErrorLocked() error {
	if m.conflict == nil {
		return nil
	}
	return errors.Errorf(
		"%w: nonce %d has uncertain ownership; attempted signed hash %s has no receipt",
		errNonceLanePaused, m.conflict.nonce, m.conflict.hash.Hex(),
	)
}

func (m *Manager) hasNonceConflict(nonce uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conflict != nil && m.conflict.nonce == nonce
}

func (m *Manager) initializeNonceLocked(ctx context.Context) error {
	if m.nonceInit {
		return nil
	}
	latest, err := m.backend.NonceAt(ctx, m.signer.Address(), nil)
	if err != nil {
		return errors.Errorf("latest mined nonce: %w", err)
	}
	pending, err := m.backend.PendingNonceAt(ctx, m.signer.Address())
	if err != nil {
		return errors.Errorf("pending nonce: %w", err)
	}
	if pending != latest {
		return errors.Errorf(
			"unmanaged pending nonce gap: latest mined nonce %d, pending nonce %d",
			latest, pending,
		)
	}
	m.nonce = pending
	m.nonceInit = true
	return nil
}

func (m *Manager) commitNonce(used uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if used >= m.nonce {
		m.nonce = used + 1
	}
}
