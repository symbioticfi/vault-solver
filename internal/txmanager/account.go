package txmanager

import (
	"context"
	"time"

	"github.com/go-errors/errors"
)

func (m *Manager) monitorAccount(ctx context.Context) {
	if m.metrics == nil {
		return
	}
	m.refreshAccount(ctx)
	ticker := time.NewTicker(m.cfg.AccountPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.refreshAccount(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) refreshAccount(ctx context.Context) {
	readCtx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
	defer cancel()
	balance, err := m.backend.TransactionSenderBalanceAt(readCtx, m.signer.Address(), nil)
	if err == nil && (balance == nil || balance.Sign() < 0) {
		err = errors.New("txmanager: invalid account balance")
	}
	latest, pending := uint64(0), uint64(0)
	if err == nil {
		latest, err = m.backend.NonceAt(readCtx, m.signer.Address(), nil)
	}
	if err == nil {
		pending, err = m.backend.PendingNonceAt(readCtx, m.signer.Address())
	}
	if err == nil {
		m.metrics.observeAccount(balance, latest, pending)
		return
	}
	if ctx.Err() == nil {
		m.metrics.observeAccountRefreshError()
		m.log.V(1).Info("account metrics refresh failed", "error", err)
	}
}
