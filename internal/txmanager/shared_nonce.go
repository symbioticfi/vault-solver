package txmanager

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// freshMinedNonce reads the first unconsumed nonce before every initial signing. Pending pools
// cannot advance admission past an unused nonce, including after a restart without local state.
func (m *Manager) freshMinedNonce(ctx context.Context) (uint64, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	nonce, err := m.backend.NonceAt(lookupCtx, m.signer.Address(), nil)
	if err != nil {
		return 0, errors.Errorf("mined nonce before signing: %w", err)
	}
	m.mu.Lock()
	nonce = max(nonce, m.confirmedNonceFloor)
	if nonce == ^uint64(0) {
		m.mu.Unlock()
		return 0, errors.New("account nonce is exhausted")
	}
	changed := !m.initialized
	m.initialized = true
	m.mu.Unlock()
	if changed {
		m.notifyLaneStateChange()
	}
	return nonce, nil
}

// rememberConfirmedNonce is called only by the lifecycle owner after an owned receipt has passed
// canonical ancestry and the requested confirmation depth. A lagging write RPC cannot move this
// process back to a nonce it already proved consumed. No unconfirmed or peer outcome advances it.
func (m *Manager) rememberConfirmedNonce(nonce uint64) {
	if nonce != ^uint64(0) {
		nonce++
	}
	m.mu.Lock()
	m.confirmedNonceFloor = max(m.confirmedNonceFloor, nonce)
	m.mu.Unlock()
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

// reusableNonce remembers process-local fee hints for abandoned or initially underpriced work.
// The nonce always comes from mined state; the floor applies only to that exact nonce.
type reusableNonce struct {
	nonce uint64
	fees  feeQuote
}

func (m *Manager) selectNonce(ctx context.Context) (uint64, *feeQuote, error) {
	remembered := m.reusableSnapshot()
	nonce, err := m.freshMinedNonce(ctx)
	if err != nil {
		return 0, nil, err
	}
	if remembered == nil {
		return nonce, nil, nil
	}
	if remembered.nonce != nonce {
		// A consumed nonce or a lower nonce after a reorg invalidates the old fee hint.
		m.forgetReusableIfUnchanged(remembered)
		return nonce, nil, nil
	}
	return nonce, &remembered.fees, nil
}

func (m *Manager) reusableSnapshot() *reusableNonce {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reusable == nil {
		return nil
	}
	return &reusableNonce{nonce: m.reusable.nonce, fees: cloneFeeQuote(m.reusable.fees)}
}

// unusedReusableNonce also serves profitability quotes. An unavailable nonce read cannot establish
// whether the old fee floor still applies, so preserve the hint and let the caller retry later.
func (m *Manager) unusedReusableNonce(ctx context.Context) (*reusableNonce, error) {
	remembered := m.reusableSnapshot()
	if remembered == nil {
		return nil, nil
	}
	latest, err := m.freshMinedNonce(ctx)
	if err != nil {
		return nil, errors.Errorf("mined nonce before applying replacement fee hint: %w", err)
	}
	if latest != remembered.nonce {
		m.forgetReusableIfUnchanged(remembered)
		return nil, nil
	}
	return remembered, nil
}

func (m *Manager) rememberReusable(nonce uint64, fees feeQuote) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reusable != nil && m.reusable.nonce == nonce {
		fees.tip = maxBigCopy(fees.tip, m.reusable.fees.tip)
		fees.maxFee = maxBigCopy(fees.maxFee, m.reusable.fees.maxFee)
	}
	m.reusable = &reusableNonce{nonce: nonce, fees: cloneFeeQuote(fees)}
}

func (m *Manager) forgetReusable(nonce uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reusable != nil && m.reusable.nonce == nonce {
		m.reusable = nil
	}
}

// Quote and admission reads may complete after a newer underpriced candidate updates the hint.
// A stale observation may clear only the exact fee snapshot it checked, never newer fee progress.
func (m *Manager) forgetReusableIfUnchanged(remembered *reusableNonce) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reusable != nil && m.reusable.nonce == remembered.nonce &&
		m.reusable.fees.maxFee.Cmp(remembered.fees.maxFee) == 0 &&
		m.reusable.fees.tip.Cmp(remembered.fees.tip) == 0 {
		m.reusable = nil
	}
}

// replacementFloor applies old fees only; all transaction fields and the current market quote
// originate in the new request. A request/global ceiling is never raised to repair a nonce gap.
func replacementFloor(current, previous feeQuote, limit *big.Int) (feeQuote, error) {
	current.tip = maxBigCopy(current.tip, bumpFee(previous.tip))
	current.maxFee = maxBigCopy(current.maxFee, bumpFee(previous.maxFee))
	if current.tip.Cmp(current.maxFee) > 0 || (limit != nil && current.maxFee.Cmp(limit) > 0) {
		return feeQuote{}, errors.Errorf("%w: same-nonce replacement requires fee %s tip %s under limit %s", errReplacementLimitReached, current.maxFee, current.tip, feeLimitString(limit))
	}
	return current, nil
}

func (m *Manager) abandonPending(ctx context.Context, pending *pendingTransaction, reason string, cause error) Result {
	m.rememberReusable(pending.nonce, pending.fees)
	var err error = errors.Errorf("send %q at nonce %d: %w", pending.req.Label, pending.nonce, ErrAbandoned)
	if cause != nil {
		err = errors.Join(err, cause)
	}
	observability.Log(ctx).Info("pending transaction tracking abandoned; nonce retained for fresh work",
		"label", pending.req.Label, "hash", pending.originalHash.Hex(), "nonce", pending.nonce, "reason", reason)
	return Result{Hash: pending.originalHash, Outcome: OutcomeAbandoned, Err: err}
}
