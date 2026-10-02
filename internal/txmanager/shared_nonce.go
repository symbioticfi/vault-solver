package txmanager

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// freshPendingNonce reads before every initial signing. Other senders' pending work advances this
// view, and simultaneous readers may race. Only an owned abandoned nonce may override this value.
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

// reusableNonce remembers only process-owned abandoned work, never another replica's pending call.
// The fee floor grows with each submitted replacement so a relay can replace the previous bytes.
type reusableNonce struct {
	nonce uint64
	fees  feeQuote
}

func (m *Manager) selectNonce(ctx context.Context) (uint64, *feeQuote, error) {
	pending, err := m.freshPendingNonce(ctx)
	if err != nil {
		return 0, nil, err
	}
	m.mu.Lock()
	hasReusable := m.reusable != nil
	m.mu.Unlock()
	if !hasReusable {
		return pending, nil, nil
	}
	remembered, err := m.unusedReusableNonce(ctx)
	if err != nil {
		return 0, nil, err
	}
	if remembered == nil {
		// A mined nonce may have advanced after the first pending read. Refresh rather than
		// signing a stale pending value after dropping an owned gap.
		pending, err = m.freshPendingNonce(ctx)
		return pending, nil, err
	}
	return remembered.nonce, &remembered.fees, nil
}

// unusedReusableNonce also serves profitability quotes. An unavailable nonce read cannot establish
// whether the old fee floor still applies, so preserve the hint and let the caller retry later.
func (m *Manager) unusedReusableNonce(ctx context.Context) (*reusableNonce, error) {
	m.mu.Lock()
	var remembered *reusableNonce
	if m.reusable != nil {
		remembered = &reusableNonce{nonce: m.reusable.nonce, fees: cloneFeeQuote(m.reusable.fees)}
	}
	m.mu.Unlock()
	if remembered == nil {
		return nil, nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	latest, err := m.backend.NonceAt(lookupCtx, m.signer.Address(), nil)
	cancel()
	if err != nil {
		return nil, errors.Errorf("mined nonce before reusing abandoned nonce: %w", err)
	}
	if latest > remembered.nonce {
		m.forgetReusable(remembered.nonce)
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

// replacementFloor applies old fees only; all transaction fields and the current market quote
// originate in the new request. A request/global ceiling is never raised to repair a nonce gap.
func replacementFloor(current, previous feeQuote, limit *big.Int) (feeQuote, error) {
	current.tip = maxBigCopy(current.tip, bumpFee(previous.tip))
	current.maxFee = maxBigCopy(current.maxFee, bumpFee(previous.maxFee))
	if current.tip.Cmp(current.maxFee) > 0 || (limit != nil && current.maxFee.Cmp(limit) > 0) {
		return feeQuote{}, errors.Errorf("%w: abandoned nonce replacement requires fee %s tip %s under limit %s", errReplacementLimitReached, current.maxFee, current.tip, feeLimitString(limit))
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
