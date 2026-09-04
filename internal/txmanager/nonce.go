package txmanager

import (
	"context"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

func isDefiniteBroadcastRejection(err error) bool {
	return errorContains(err,
		"insufficient funds",
		"intrinsic gas too low",
		"invalid sender",
		"transaction type not supported",
	)
}

func isNonceConsumedError(err error) bool {
	return errorContains(err, "nonce too low", "nonce is too low", "nonce has already been used")
}

func isKnownTransactionError(err error) bool {
	return errorContains(err, "already known")
}

func isPendingNonceCollision(err error) bool {
	return errorContains(err, "replacement transaction underpriced")
}

func errorContains(err error, fragments ...string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range fragments {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

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
		m.mu.Unlock()
		panic("txmanager: multiple nonce conflicts")
	}
	first := m.conflict == nil
	m.conflict = &nonceConflict{nonce: nonce, hash: hash}
	m.mu.Unlock()
	if !first {
		return
	}
	m.notifyLaneStateChange()
	m.log.Error(
		errors.New("nonce ownership is uncertain"),
		"transaction manager paused pending nonce reconciliation",
		"nonce",
		nonce,
		"hash",
		hash.Hex(),
	)
}

func (m *Manager) clearNonceConflict(nonce uint64) {
	m.mu.Lock()
	cleared := m.conflict != nil && m.conflict.nonce == nonce
	if cleared {
		m.conflict = nil
	}
	m.mu.Unlock()
	if cleared {
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
		errNonceLanePaused,
		m.conflict.nonce,
		m.conflict.hash.Hex(),
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
	if latest != pending {
		return errors.Errorf(
			"unmanaged pending nonce gap: latest mined nonce %d, pending nonce %d",
			latest,
			pending,
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
