package txmanager

import (
	"context"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/tenderly"
)

func (m *Manager) completeLifecycle(managerCtx context.Context, pending *pendingTransaction) Result {
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(managerCtx))
	done := make(chan struct{})
	go func() {
		select {
		case <-managerCtx.Done():
			m.markStopped()
			timer := time.NewTimer(m.cfg.ShutdownTimeout)
			defer timer.Stop()
			select {
			case <-done:
			case <-timer.C:
				cancel(errShutdownTimeout)
			}
		case <-done:
		}
	}()
	result := m.waitForPendingTransaction(ctx, managerCtx.Done(), pending)
	close(done)
	cancel(errManagerStopped)
	if errors.Is(result.Err, errShutdownTimeout) {
		m.log.Error(
			result.Err,
			"accepted transaction lifecycle did not drain before shutdown",
			"label",
			pending.req.Label,
			"nonce",
			pending.nonce,
			"hashes",
			attemptHashStrings(pending.attempts),
		)
	}
	if result.Receipt != nil {
		m.clearNonceConflict(pending.nonce)
	}
	return result
}

func (pending *pendingTransaction) cancellationDue(now time.Time) bool {
	return !pending.cancelDeadline.IsZero() && !now.Before(pending.cancelDeadline)
}

func (m *Manager) waitForPendingTransaction(
	ctx context.Context,
	shutdown <-chan struct{},
	pending *pendingTransaction,
) Result {
	poll := time.NewTicker(m.cfg.PollInterval)
	defer poll.Stop()
	replace := time.NewTicker(m.cfg.ReplacementInterval)
	defer replace.Stop()
	timeout := time.NewTimer(max(time.Until(pending.cancelDeadline), 0))
	defer timeout.Stop()

	cancelling := false
	shutdownSignal := shutdown
	timeoutSignal := timeout.C
	startCancellation := func(reason string) {
		if cancelling {
			return
		}
		if reason == "" {
			reason = "pending_timeout"
			if pending.cancelDeadline.Equal(pending.req.CancelAt) {
				reason = "request_deadline"
			}
		}
		cancelling = true
		shutdownSignal = nil
		timeoutSignal = nil
		m.log.Info(
			"pending transaction cancellation requested",
			"label", pending.req.Label,
			"hash", pending.originalHash.Hex(),
			"nonce", pending.nonce,
			"reason", reason,
			"deadline", pending.cancelDeadline.UTC().Format(time.RFC3339Nano),
		)
	}
	for {
		if result, terminal := m.receiptResult(ctx, pending); terminal {
			return result
		}
		select {
		case <-ctx.Done():
			return Result{
				Hash: pending.originalHash, Outcome: OutcomeTrackingStopped, Err: context.Cause(ctx),
			}
		case <-shutdownSignal:
			startCancellation("shutdown")
			m.tryReplace(ctx, pending, true)
		case <-poll.C:
			if cancelling || pending.req.Obsolete == nil {
				continue
			}
			obsolete, err := m.requestObsolete(ctx, pending.req)
			if err != nil {
				m.log.Error(
					err,
					"pending transaction obsolescence check unavailable; retaining lifecycle",
					"label",
					pending.req.Label,
					"hash",
					pending.originalHash.Hex(),
					"nonce",
					pending.nonce,
				)
				continue
			}
			if !obsolete {
				continue
			}
			startCancellation("obsolete")
			m.tryReplace(ctx, pending, true)
		case <-replace.C:
			if !cancelling && pending.cancellationDue(time.Now()) {
				startCancellation("")
			}
			if m.tryReplace(ctx, pending, cancelling) {
				// A fee read may cross the deadline and promote this attempt to cancellation.
				startCancellation("")
			}
		case <-timeoutSignal:
			startCancellation("")
			m.tryReplace(ctx, pending, true)
		}
	}
}

func (m *Manager) requestObsolete(ctx context.Context, request Request) (bool, error) {
	if request.Obsolete == nil {
		return false, nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	obsolete, err := request.Obsolete(checkCtx)
	if err != nil {
		return false, errors.Errorf("check transaction obsolescence: %w", err)
	}
	return obsolete, nil
}

func (m *Manager) receiptResult(
	ctx context.Context,
	pending *pendingTransaction,
) (Result, bool) {
	attempt, receipt, found := m.findPendingReceipt(ctx, pending)
	if !found {
		return Result{}, false
	}
	if pending.nonceConflictHash != (common.Hash{}) && m.hasNonceConflict(pending.nonce) {
		if err := m.confirmCanonicalReceipt(ctx, receipt); err != nil {
			m.log.Error(
				err,
				"owned receipt cannot reconcile nonce conflict",
				"label",
				pending.req.Label,
				"hash",
				attempt.hash.Hex(),
				"nonce",
				pending.nonce,
			)
			return Result{}, false
		}
		m.clearNonceConflict(pending.nonce)
	}

	confirmations := m.cfg.Confirmations
	pending.lifecycle.transitionPhase(lifecyclePhaseConfirming)
	receipt, err := m.waitForConfirmations(ctx, attempt.hash, receipt, confirmations)
	if errors.Is(err, errReceiptReorged) {
		pending.lifecycle.transitionPhase(lifecyclePhasePending)
		if pending.nonceConflictHash != (common.Hash{}) {
			m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
		}
		m.log.Info(
			"transaction inclusion reorged; resuming pending lifecycle",
			"label",
			pending.req.Label,
			"hash",
			attempt.hash.Hex(),
			"nonce",
			pending.nonce,
		)
		return Result{}, false
	}
	if err != nil {
		outcome := OutcomeIncludedUnconfirmed
		if attempt.cancellation {
			outcome = OutcomeCancelled
		}
		return Result{Hash: attempt.hash, Receipt: receipt, Outcome: outcome, Err: err}, true
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return m.revertedResult(pending, attempt, receipt), true
	}
	if attempt.cancellation {
		return Result{
			Hash:    attempt.hash,
			Receipt: receipt,
			Outcome: OutcomeCancelled,
			Err: errors.Errorf(
				"send %q: pending transaction cancelled at nonce %d",
				pending.req.Label,
				pending.nonce,
			),
		}, true
	}
	m.log.V(1).Info(
		"transaction confirmed",
		"label",
		pending.req.Label,
		"hash",
		attempt.hash.Hex(),
		"nonce",
		pending.nonce,
		"blockNumber",
		optionalBigString(receipt.BlockNumber),
		"gasUsed",
		receipt.GasUsed,
		"effectiveGasPrice",
		optionalBigString(receipt.EffectiveGasPrice),
		"confirmations",
		confirmations,
	)
	return Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeConfirmed}, true
}

func (m *Manager) revertedResult(
	pending *pendingTransaction,
	attempt txAttempt,
	receipt *types.Receipt,
) Result {
	revertErr := errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex())
	m.log.Error(
		revertErr,
		"transaction reverted",
		"label",
		pending.req.Label,
		"hash",
		attempt.hash.Hex(),
		"nonce",
		pending.nonce,
		"tenderly",
		tenderly.SimulatorURL(
			m.chainID,
			m.signer.Address(),
			pending.req.To,
			pending.req.Data,
			nil,
		),
	)
	return Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeReverted, Err: revertErr}
}

func (m *Manager) findPendingReceipt(
	ctx context.Context,
	pending *pendingTransaction,
) (txAttempt, *types.Receipt, bool) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	count := len(pending.attempts)
	if count == 0 {
		return txAttempt{}, nil, false
	}
	start := pending.receiptCursor % count
	for checked := range count {
		if lookupCtx.Err() != nil {
			break
		}
		index := (start + checked) % count
		pending.receiptCursor = (index + 1) % count
		attempt := pending.attempts[index]
		receipt, err := m.backend.TransactionReceipt(lookupCtx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			m.log.Error(
				err,
				"pending transaction receipt unavailable",
				"label",
				pending.req.Label,
				"hash",
				attempt.hash.Hex(),
				"nonce",
				pending.nonce,
			)
			continue
		}
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			m.log.Error(
				err,
				"invalid pending transaction receipt",
				"label",
				pending.req.Label,
				"hash",
				attempt.hash.Hex(),
				"nonce",
				pending.nonce,
			)
			continue
		}
		return attempt, receipt, true
	}
	return txAttempt{}, nil, false
}
