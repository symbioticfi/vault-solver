package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/go-errors/errors"
)

// The horizon fee policy (strategy §2.1–§2.5, §2.9; docs/TXMANAGER-PLAN.md §4.3) prices a new attempt from
// a fresh fee snapshot: the three-level tip of tipRule and an exact EIP-1559 validity horizon,
// fee(maxHorizonBlocks, tip), clamped to the request cap and the signer balance and refused below
// fee(minHorizonBlocks + lag, tipFloor). Its gas limit comes from a next-block estimate. Quotes are priced at
// fees.pricingHorizonBlocks from the cached snapshot. The legacy policy keeps its own path unchanged.

// pricedAttempt is a new attempt priced by the fee policy, with what the send log, span and metrics report.
type pricedAttempt struct {
	fees         feeQuote
	gas          uint64
	estimateMode string
	// snapshot carries the head, next base fee and lag the attempt was priced at; its nextBase is nil
	// under the legacy policy with the balance guard off.
	snapshot     sendSnapshot
	balance      *big.Int // signer balance at the snapshot's block; nil with the balance guard off
	horizon      uint64   // blocks the fee cap stays valid at the floor tip; set with snapshot.nextBase
	balanceBound bool     // the signer balance, not the policy or a cap, set the fee cap
}

// priceHorizonAttempt prices a new horizon-policy attempt (strategy §2.5): a fresh fee snapshot, with the
// stale-head wait; then, concurrently, the next-block gas estimate on top of its header and the signer
// balance pinned to its block; then initialFees. A refusal returns before anything is signed. The returned
// attempt carries the snapshot even on error, for the refusal log.
func (m *Manager) priceHorizonAttempt(ctx context.Context, req Request, value *big.Int) (priced pricedAttempt, err error) {
	priced.snapshot, err = m.horizonSendSnapshot(ctx)
	if err != nil {
		return priced, errors.Errorf("send %q: %w", req.Label, err)
	}
	snapshot := priced.snapshot
	// A failed estimate fails the send whatever the balance read returns, so it also ends that read.
	readCtx, stopReads := context.WithCancelCause(ctx)
	defer stopReads(nil)
	estimate := m.estimateAsync(ctx, req, func(ctx context.Context) (uint64, string, error) {
		return m.estimateHorizonGas(ctx, req, snapshot.fees.header)
	}, func() { stopReads(errEstimateFailed) })
	defer estimate.stop()

	if m.guardEnabled() {
		if priced.balance, err = m.pinnedBalance(readCtx, snapshot.pin); err != nil {
			if estimateErr := estimate.failure(); estimateErr != nil {
				return priced, estimateErr
			}
			return priced, errors.Errorf("send %q: %w", req.Label, err)
		}
		m.evaluateFunding(ctx, priced.balance, snapshot.nextBase, snapshot.historyHead)
	}
	if priced.gas, priced.estimateMode, err = estimate.wait(); err != nil {
		return priced, err
	}
	quote, err := initialFees(
		snapshot.fees, priced.gas, priced.balance, value, reserveFeeBump(m.normalFeeLimit(req)), snapshot.lag, m.horizon,
	)
	m.observeFeeSnapshot(snapshot.nextBase, priced.gas)
	if err != nil {
		return priced, errors.Errorf("send %q: %w", req.Label, err)
	}
	priced.fees, priced.horizon, priced.balanceBound = quote.fees, quote.horizon, quote.binding == feeBindingBalance
	return priced, nil
}

// horizonMaxFeePerGas prices a quote from the cached fee snapshot (strategy §2.9), reading a new one only
// when the cached one is older than the poll interval, so quoting costs no RPCs per quote. A snapshot
// trailing the real next block by more than fees.maxHeadLagBlocks is read again once, and still stale fails
// the quote rather than pricing it from an old base fee.
func (m *Manager) horizonMaxFeePerGas(ctx context.Context) (*big.Int, error) {
	snapshot, err := m.snapshots.get(ctx, m.cfg.PollInterval)
	if err != nil {
		return nil, err
	}
	lag := snapshot.lag(time.Now(), m.cfg.Fees.BlockTime)
	if lag > m.maxHeadLagBlocks() {
		if snapshot, err = m.snapshots.get(ctx, 0); err != nil {
			return nil, err
		}
		if lag = snapshot.lag(time.Now(), m.cfg.Fees.BlockTime); lag > m.maxHeadLagBlocks() {
			return nil, errors.Errorf(
				"%w: fee snapshot head %d trails the next block by %d blocks, more than fees.maxHeadLagBlocks %d",
				errFreshFeesUnavailable, snapshot.header.Number, lag, m.maxHeadLagBlocks(),
			)
		}
	}
	return pricingFee(snapshot, lag, m.cfg.Balance.ReferenceGasUnits, m.normalFeeLimit(Request{}), m.horizon)
}
