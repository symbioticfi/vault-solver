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

var errReconciliationPending = errors.New("account nonce reconciliation is pending")

// canonicalAccountNonce pins a nonce to an ancestor of one stable canonical head. Every RPC has a
// bounded context, and disagreement or unavailable historical state withholds proof rather than
// falling back to a nonce at an unrelated block height.
func (m *Manager) canonicalAccountNonce(ctx context.Context, confirmations uint64) (uint64, error) {
	state, err := m.canonicalNonceState(ctx, confirmations, false)
	return state.confirmed, err
}

type canonicalNonceState struct {
	confirmed uint64
	current   uint64
}

// Admission additionally compares current-head account state with the confirmation ancestor. Both
// reads belong to the same stable head, preventing a stale write RPC from hiding recent inclusion.
func (m *Manager) canonicalNonceState(ctx context.Context, confirmations uint64, readCurrent bool) (canonicalNonceState, error) {
	head, err := m.confirmationHead(ctx)
	if err != nil {
		return canonicalNonceState{}, errors.Errorf("nonce reconciliation head: %w", err)
	}
	if !head.Number.IsUint64() || head.Number.Uint64() < confirmations {
		return canonicalNonceState{}, errors.Errorf("%w: head has insufficient confirmation history", errReconciliationPending)
	}
	anchorNumber := new(big.Int).SetUint64(head.Number.Uint64() - confirmations)
	anchor, err := m.readHeaderAtNumber(ctx, anchorNumber)
	if err != nil {
		return canonicalNonceState{}, errors.Errorf("nonce reconciliation ancestor: %w", err)
	}
	if anchor == nil || anchor.Number == nil || anchor.Number.Cmp(anchorNumber) != 0 {
		return canonicalNonceState{}, errors.New("nonce reconciliation ancestor has an invalid block number")
	}
	if err := m.confirmReceiptAncestry(ctx, head, &types.Receipt{BlockNumber: anchor.Number, BlockHash: anchor.Hash()}); err != nil {
		if errors.Is(err, errReceiptReorged) {
			return canonicalNonceState{}, errors.Errorf("%w: %w", errReconciliationPending, err)
		}
		return canonicalNonceState{}, errors.Errorf("nonce reconciliation ancestor is not canonical: %w", err)
	}
	nonce, err := m.readNonceAtHash(ctx, anchor.Hash())
	if err != nil {
		return canonicalNonceState{}, errors.Errorf("hash-pinned account nonce: %w", err)
	}
	state := canonicalNonceState{confirmed: nonce, current: nonce}
	if readCurrent && anchor.Hash() != head.Hash() {
		state.current, err = m.readNonceAtHash(ctx, head.Hash())
		if err != nil {
			return canonicalNonceState{}, errors.Errorf("hash-pinned current account nonce: %w", err)
		}
	}
	headAfter, err := m.confirmationHead(ctx)
	if err != nil {
		return canonicalNonceState{}, errors.Errorf("nonce reconciliation head after state read: %w", err)
	}
	if head.Hash() != headAfter.Hash() {
		return canonicalNonceState{}, errors.Errorf("%w: head changed during proof", errReconciliationPending)
	}
	return state, nil
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
	nonce, err := m.readFreshReconciledNonce(ctx)
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
	changed := !m.initialized
	m.initialized = true
	m.mu.Unlock()
	if changed {
		m.notifyLaneStateChange()
	}
	return nonce, nil
}

func (m *Manager) readFreshReconciledNonce(ctx context.Context) (uint64, error) {
	latest, err := m.readLatestNonce(ctx)
	if err != nil {
		return 0, errors.Errorf("latest mined nonce before signing: %w", err)
	}
	pending, err := m.readPendingNonce(ctx)
	if err != nil {
		return 0, errors.Errorf("pending nonce before signing: %w", err)
	}
	state, err := m.canonicalNonceState(ctx, m.cfg.Confirmations, true)
	if err != nil {
		return 0, err
	}
	confirmed := state.confirmed
	if state.current != confirmed || confirmed != latest {
		return 0, errors.Errorf("%w: latest %d, current %d, confirmed %d", errReconciliationPending, latest, state.current, confirmed)
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
	latest, err := m.readLatestNonce(ctx)
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
	nonce, err := m.canonicalAccountNonce(ctx, m.confirmations(pending.req))
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

// Each reconciliation RPC gets a fresh budget. The complete proof retains its caller's context,
// allowing healthy deeper ancestry walks while bounding a single stalled dependency.
func (m *Manager) readLatestNonce(ctx context.Context) (uint64, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	return m.backend.NonceAt(lookupCtx, m.signer.Address(), nil)
}

func (m *Manager) readPendingNonce(ctx context.Context) (uint64, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	return m.backend.PendingNonceAt(lookupCtx, m.signer.Address())
}

func (m *Manager) readNonceAtHash(ctx context.Context, hash common.Hash) (uint64, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	return m.backend.ReadNonceAtHash(lookupCtx, m.signer.Address(), hash)
}

func (m *Manager) readHeaderAtNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	return m.backend.HeaderByNumber(lookupCtx, number)
}
