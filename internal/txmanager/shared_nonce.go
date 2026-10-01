package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// HashNonceBackend reads ordinary on-chain account state at exactly the supplied block hash.
// ReconcileNonces requires this capability; a private relay's pending view is not canonical proof.
type HashNonceBackend interface {
	ReadNonceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error)
}

var errReconciliationPending = errors.New("account nonce reconciliation is pending")

// canonicalAccountNonce pins a nonce to an ancestor of one stable canonical head. Every RPC has a
// bounded context, and disagreement or unavailable historical state withholds proof rather than
// falling back to a nonce at an unrelated block height.
func (m *Manager) canonicalAccountNonce(ctx context.Context, confirmations uint64) (uint64, error) {
	backend, ok := m.backend.(HashNonceBackend)
	if !ok {
		return 0, errors.New("nonce reconciliation requires hash-pinned account state reads")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	head, err := m.confirmationHead(lookupCtx)
	if err != nil {
		return 0, errors.Errorf("nonce reconciliation head: %w", err)
	}
	if !head.Number.IsUint64() || head.Number.Uint64() < confirmations {
		return 0, errors.Errorf("%w: head has insufficient confirmation history", errReconciliationPending)
	}
	anchorNumber := new(big.Int).SetUint64(head.Number.Uint64() - confirmations)
	anchor, err := m.backend.HeaderByNumber(lookupCtx, anchorNumber)
	if err != nil {
		return 0, errors.Errorf("nonce reconciliation ancestor: %w", err)
	}
	if anchor == nil || anchor.Number == nil || anchor.Number.Cmp(anchorNumber) != 0 {
		return 0, errors.New("nonce reconciliation ancestor has an invalid block number")
	}
	if err := m.confirmReceiptAncestry(lookupCtx, head, &types.Receipt{BlockNumber: anchor.Number, BlockHash: anchor.Hash()}); err != nil {
		if errors.Is(err, errReceiptReorged) {
			return 0, errors.Errorf("%w: %w", errReconciliationPending, err)
		}
		return 0, errors.Errorf("nonce reconciliation ancestor is not canonical: %w", err)
	}
	nonce, err := backend.ReadNonceAtHash(lookupCtx, m.signer.Address(), anchor.Hash())
	if err != nil {
		return 0, errors.Errorf("hash-pinned account nonce: %w", err)
	}
	headAfter, err := m.confirmationHead(lookupCtx)
	if err != nil {
		return 0, errors.Errorf("nonce reconciliation head after state read: %w", err)
	}
	if head.Hash() != headAfter.Hash() {
		return 0, errors.Errorf("%w: head changed during proof", errReconciliationPending)
	}
	return nonce, nil
}

func (m *Manager) setReconciliationBusy(busy bool) {
	m.mu.Lock()
	changed := m.reconcileBusy != busy
	m.reconcileBusy = busy
	m.mu.Unlock()
	if changed {
		m.notifyLaneStateChange()
	}
}

// freshReconciledNonce runs before each initial signing, including after another replica advanced
// the account. Unknown singleton work first waits, then only a fixed recovery cancellation may be sent.
func (m *Manager) freshReconciledNonce(ctx context.Context) (uint64, error) {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	nonce, err := m.readFreshReconciledNonce(lookupCtx)
	m.setReconciliationBusy(err != nil)
	if err != nil && !errors.Is(err, errReconciliationPending) && ctx.Err() == nil {
		m.reconciliationReads.failed(observability.Log(ctx), err, "account nonce reconciliation RPC unavailable; retaining readiness gate")
	} else if err == nil || errors.Is(err, errReconciliationPending) {
		m.reconciliationReads.recovered(observability.Log(ctx), "account nonce reconciliation RPC recovered")
	}
	if err != nil {
		return 0, errors.Errorf("%w: %w", errNonceLanePaused, err)
	}
	m.mu.Lock()
	changed := !m.nonceInit
	m.nonce, m.nonceInit = nonce, true
	m.mu.Unlock()
	if changed {
		m.notifyLaneStateChange()
	}
	return nonce, nil
}

func (m *Manager) readFreshReconciledNonce(ctx context.Context) (uint64, error) {
	latest, err := m.backend.NonceAt(ctx, m.signer.Address(), nil)
	if err != nil {
		return 0, errors.Errorf("latest mined nonce before signing: %w", err)
	}
	pending, err := m.backend.PendingNonceAt(ctx, m.signer.Address())
	if err != nil {
		return 0, errors.Errorf("pending nonce before signing: %w", err)
	}
	confirmed, err := m.canonicalAccountNonce(ctx, m.cfg.Confirmations)
	if err != nil {
		return 0, err
	}
	if confirmed != latest {
		return 0, errors.Errorf("%w: latest %d, confirmed %d", errReconciliationPending, latest, confirmed)
	}
	if m.unknownNonce != nil && confirmed > m.unknownNonce.nonce {
		m.unknownNonce = nil
	}
	if pending != latest {
		if pending > latest && pending-latest == 1 {
			m.observeUnknownNonce(latest)
			if err := m.recoverUnknownNonce(ctx, latest); err != nil {
				return 0, err
			}
		}
		return 0, errors.Errorf("%w: unknown pending lane latest %d, pending %d", errReconciliationPending, latest, pending)
	}
	if m.unknownNonce != nil {
		if confirmed <= m.unknownNonce.nonce {
			if err := m.recoverUnknownNonce(ctx, latest); err != nil {
				return 0, err
			}
			return 0, errors.Errorf("%w: previously observed unknown nonce has not been consumed", errReconciliationPending)
		}
		m.unknownNonce = nil
	}
	if latest == ^uint64(0) {
		return 0, errors.New("account nonce is exhausted")
	}
	m.mu.Lock()
	conflict := m.conflict
	m.mu.Unlock()
	if conflict != nil {
		if confirmed <= conflict.nonce {
			return 0, errors.Errorf("%w: conflicted nonce has not been consumed", errReconciliationPending)
		}
		m.clearNonceConflict(conflict.nonce)
	}
	return latest, nil
}

func (m *Manager) initializeReconciledNonce(ctx context.Context) error {
	if _, ok := m.backend.(HashNonceBackend); !ok {
		return errors.New("nonce reconciliation requires hash-pinned account state reads")
	}
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	waiting := false
	for {
		if _, err := m.freshReconciledNonce(ctx); err == nil {
			return nil
		} else if ctx.Err() == nil && !waiting {
			waiting = true
			observability.Log(ctx).Info("waiting for a confirmed empty account nonce lane", "reason", err)
		}
		select {
		case <-ctx.Done():
			return errors.Errorf("initialize reconciled nonce: %w", context.Cause(ctx))
		case <-ticker.C:
		}
	}
}

// monitorReconciledNonce heals unsigned admission pauses. It never competes with an admitted
// lifecycle: receipt polling owns that lifecycle's stronger, request-specific finality decision.
func (m *Manager) monitorReconciledNonce(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case m.lifecycleSlot <- struct{}{}:
				_, _ = m.freshReconciledNonce(ctx) // The probe logs RPC failures and retains the gate for the next tick.
				<-m.lifecycleSlot                  // This probe never took admission-demand ownership.
			default:
			}
		}
	}
}

// confirmConsumedNonce is only considered after every known hash returned NotFound. The outcome
// deliberately cannot distinguish another replica's transaction from an owned but lagging receipt.
func (m *Manager) confirmConsumedNonce(ctx context.Context, pending *pendingTransaction) (Result, bool) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	latest, err := m.backend.NonceAt(lookupCtx, m.signer.Address(), nil)
	if err != nil {
		if ctx.Err() == nil {
			pending.nonceReads.failed(observability.Log(ctx), err, "mined nonce reconciliation RPC unavailable", "nonce", pending.nonce)
		}
		return Result{}, false
	}
	if latest <= pending.nonce {
		pending.nonceReads.recovered(observability.Log(ctx), "mined nonce reconciliation RPC recovered", "nonce", pending.nonce)
		return Result{}, false
	}
	pending.nonceConflictHash = pending.originalHash
	m.markNonceConflict(pending.nonce, pending.originalHash)
	nonce, err := m.canonicalAccountNonce(lookupCtx, m.confirmations(pending.req))
	if err != nil {
		if !errors.Is(err, errReconciliationPending) && ctx.Err() == nil {
			pending.nonceReads.failed(observability.Log(ctx), err, "canonical nonce proof RPC unavailable", "nonce", pending.nonce)
		}
		return Result{}, false
	}
	pending.nonceReads.recovered(observability.Log(ctx), "canonical nonce proof RPC recovered", "nonce", pending.nonce)
	if nonce <= pending.nonce {
		return Result{}, false
	}
	m.setReconciliationBusy(true)
	m.clearNonceConflict(pending.nonce)
	_, _ = m.freshReconciledNonce(ctx) // Logs failures; a newer unknown lifecycle keeps admission paused.
	return Result{
		Hash: pending.originalHash, Outcome: OutcomeNonceConsumed,
		Err: errors.Errorf("send %q at nonce %d: %w", pending.req.Label, pending.nonce, ErrNonceConsumed),
	}, true
}
