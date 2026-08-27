package txmanager

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/tenderly"
)

// broadcast runs on the worker goroutine only, after lifecycle admission, so fee selection, gas
// estimation, signing, and nonce assignment stay serialized.
func (m *Manager) broadcast(ctx context.Context, req Request) (*pendingTransaction, error) {
	broadcastCtx := ctx
	cancel := func() {}
	if !req.CancelAt.IsZero() {
		broadcastCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	if err := broadcastCtx.Err(); err != nil {
		return nil, errors.Errorf("send %q before broadcast: %w", req.Label, err)
	}

	if req.MaxFeePerGas != nil && req.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.Errorf("send %q: request max fee per gas must be positive", req.Label)
	}
	normalLimit := m.normalFeeLimit(req)
	fees, err := m.currentFees(broadcastCtx, reserveFeeBump(normalLimit))
	if err != nil {
		return nil, errors.Errorf("send %q: %w", req.Label, err)
	}

	gas := req.GasLimit
	if gas == 0 {
		gas, err = m.estimateGas(broadcastCtx, req)
		if err != nil {
			return nil, err
		}
	}
	obsolete, obsoleteErr := m.requestObsolete(broadcastCtx, req)
	if obsoleteErr != nil {
		// Obsolescence is only a liveness optimization. The solver already validated the call,
		// and execution-time contracts remain authoritative, so an unknown check keeps it alive.
		m.log.Error(obsoleteErr, "transaction obsolescence check unavailable; continuing",
			"label", req.Label)
	} else if obsolete {
		return nil, errors.Errorf("send %q: %w", req.Label, errRequestObsolete)
	}

	value := req.Value
	if value == nil {
		value = new(big.Int)
	}
	m.log.V(1).Info(
		"transaction prepared",
		"label", req.Label,
		"to", req.To.Hex(),
		"value", value.String(),
		"calldataBytes", len(req.Data),
		"gasLimit", gas,
		"baseFeePerGas", fees.baseFee.String(),
		"maxPriorityFeePerGas", fees.tip.String(),
		"maxFeePerGas", fees.maxFee.String(),
		"requestMaxFeePerGas", optionalBigString(req.MaxFeePerGas),
	)

	nonce, err := m.nextNonce(broadcastCtx)
	if err != nil {
		return nil, err
	}
	signed, sendErr := m.signAndSend(
		broadcastCtx, nonce, req.To, req.Data, value, gas, fees, false,
	)
	if signed == nil {
		return nil, errors.Errorf("send %q: %w", req.Label, sendErr)
	}
	hash := signed.Hash()
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	if broadcastUncertain {
		m.log.Error(sendErr, "transaction broadcast uncertain; tracking signed hash",
			"label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	} else if sendErr != nil {
		m.log.Info("transaction already known by write RPC",
			"label", req.Label, "hash", hash.Hex(), "nonce", nonce, "rpcResult", sendErr.Error())
	} else {
		m.log.Info("sent", "label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	}
	m.commitNonce(nonce)
	return &pendingTransaction{
		req:   req,
		nonce: nonce,
		gas:   gas,
		value: new(big.Int).Set(value),
		fees:  cloneFeeQuote(fees),
		attempts: []txAttempt{{
			hash: hash, tx: signed, exactRebroadcastPending: broadcastUncertain,
		}},
		originalHash: hash,
	}, nil
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction) {
	defer m.removeUnminedTransaction(pending)
	outcome := m.waitForPendingTransaction(ctx, pending)
	if errors.Is(outcome.Err, errShutdownTimeout) {
		m.log.Error(outcome.Err, "accepted transaction lifecycle did not drain before shutdown",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"hashes", attemptHashStrings(pending.attempts),
		)
	}
	if outcome.Receipt != nil {
		m.clearNonceConflict(pending.nonce)
	}
	pending.deliver(outcome)
}

func (pending *pendingTransaction) deliver(result Result) {
	if pending.result == nil {
		return
	}
	pending.resultOnce.Do(func() { pending.result <- result })
}

func (m *Manager) confirmations(req Request) uint64 {
	if req.Confirmations != nil {
		return *req.Confirmations
	}
	return m.cfg.Confirmations
}

func (m *Manager) waitForPendingTransaction(ctx context.Context, pending *pendingTransaction) Result {
	poll := time.NewTicker(m.cfg.PollInterval)
	defer poll.Stop()
	replace := time.NewTicker(m.cfg.ReplacementInterval)
	defer replace.Stop()
	timeout := time.NewTimer(max(time.Until(pending.cancelDeadline), 0))
	defer timeout.Stop()

	cancelling := false
	cancelRequested, timeoutC := pending.cancelRequested, timeout.C
	startCancellation := func() {
		cancelling, cancelRequested, timeoutC = true, nil, nil
	}
	for {
		if receiptResult, done := m.receiptResult(ctx, pending); done {
			return receiptResult
		}
		select {
		case <-ctx.Done():
			return Result{Hash: pending.attempts[0].hash, Err: context.Cause(ctx)}
		case <-cancelRequested:
			startCancellation()
			m.tryReplace(ctx, pending, true)
		case <-poll.C:
			if cancelling || pending.req.Obsolete == nil {
				continue
			}
			obsolete, err := m.requestObsolete(ctx, pending.req)
			if err != nil {
				m.log.Error(err, "pending transaction obsolescence check unavailable; retaining lifecycle",
					"label", pending.req.Label,
					"hash", pending.originalHash.Hex(),
					"nonce", pending.nonce,
				)
				continue
			}
			if !obsolete {
				continue
			}
			startCancellation()
			m.log.Info("pending transaction became obsolete; cancelling nonce",
				"label", pending.req.Label,
				"hash", pending.originalHash.Hex(),
				"nonce", pending.nonce,
			)
			m.tryReplace(ctx, pending, true)
		case <-replace.C:
			if !cancelling && pending.cancellationDue(time.Now()) {
				startCancellation()
			}
			m.tryReplace(ctx, pending, cancelling)
		case <-timeoutC:
			startCancellation()
			m.log.Info("pending transaction timed out; cancelling nonce",
				"label", pending.req.Label,
				"nonce", pending.nonce,
				"timeout", m.cfg.PendingTimeout.String(),
			)
			m.tryReplace(ctx, pending, true)
		}
	}
}

func (m *Manager) requestObsolete(ctx context.Context, req Request) (bool, error) {
	if req.Obsolete == nil {
		return false, nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	obsolete, err := req.Obsolete(checkCtx)
	if err != nil {
		return false, errors.Errorf("check transaction obsolescence: %w", err)
	}
	return obsolete, nil
}

func (m *Manager) receiptResult(ctx context.Context, pending *pendingTransaction) (Result, bool) {
	lookupCtx, cancelLookup := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancelLookup()
	attempts := len(pending.attempts)
	if attempts == 0 {
		return Result{}, false
	}
	start := pending.receiptCursor % attempts
	for checked := range attempts {
		if lookupCtx.Err() != nil {
			break
		}
		i := (start + checked) % attempts
		pending.receiptCursor = (i + 1) % attempts
		attempt := pending.attempts[i]
		receipt, err := m.backend.TransactionReceipt(lookupCtx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			m.log.Error(err, "pending transaction receipt unavailable",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			m.log.Error(err, "invalid pending transaction receipt",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		cancelLookup()
		if pending.nonceConflictHash != (common.Hash{}) && m.hasNonceConflict(pending.nonce) {
			if err := m.confirmCanonicalReceipt(ctx, receipt); err != nil {
				m.log.Error(err, "owned receipt cannot reconcile nonce conflict",
					"label", pending.req.Label,
					"hash", attempt.hash.Hex(),
					"nonce", pending.nonce,
				)
				continue
			}
			m.clearNonceConflict(pending.nonce)
		}
		confirmations := m.confirmations(pending.req)
		receipt, err = m.waitForConfirmations(ctx, attempt.hash, receipt, confirmations)
		if errors.Is(err, errReceiptReorged) {
			if pending.nonceConflictHash != (common.Hash{}) {
				m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
			}
			m.log.Info("transaction inclusion reorged; resuming pending lifecycle",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			return Result{}, false
		}
		if err != nil {
			return Result{Hash: attempt.hash, Receipt: receipt, Err: err}, true
		}
		if receipt.Status == types.ReceiptStatusFailed {
			revertErr := errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex())
			m.log.Error(revertErr, "transaction reverted",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"tenderly", tenderly.SimulatorURL(m.chainID, m.signer.Address(), pending.req.To, pending.req.Data, pending.req.Value),
			)
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Err:     revertErr,
			}, true
		}
		if attempt.cancellation {
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Err: errors.Errorf(
					"send %q: pending transaction cancelled at nonce %d",
					pending.req.Label, pending.nonce,
				),
			}, true
		}
		m.log.V(1).Info(
			"transaction confirmed",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"blockNumber", optionalBigString(receipt.BlockNumber),
			"gasUsed", receipt.GasUsed,
			"effectiveGasPrice", optionalBigString(receipt.EffectiveGasPrice),
			"confirmations", confirmations,
		)
		return Result{Hash: attempt.hash, Receipt: receipt}, true
	}
	return Result{}, false
}
