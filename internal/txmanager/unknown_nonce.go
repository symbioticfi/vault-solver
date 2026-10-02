package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

type unknownNonceRecovery struct {
	nonce        uint64
	observedAt   time.Time
	cancellation *types.Transaction
}

func (m *Manager) observeUnknownNonce(nonce uint64) {
	if m.unknownNonce == nil || m.unknownNonce.nonce != nonce {
		m.unknownNonce = &unknownNonceRecovery{nonce: nonce, observedAt: time.Now()}
	}
}

// recoverUnknownNonce runs only under reconcileMu. A vanished pending entry does not erase the
// observed nonce: delayed private bytes remain executable until canonical account state consumes it.
func (m *Manager) recoverUnknownNonce(ctx context.Context, latest uint64) error {
	recovery := m.unknownNonce
	if recovery == nil || latest != recovery.nonce || time.Since(recovery.observedAt) < m.cfg.PendingTimeout {
		return nil
	}
	if recovery.cancellation != nil {
		if limit := m.globalFeeLimit(); limit == nil || recovery.cancellation.GasFeeCap().Cmp(limit) > 0 {
			return errors.Errorf("%w: saved cancellation exceeds the current cap", errReconciliationPending)
		}
		err := m.sendSigned(ctx, recovery.cancellation, true, true)
		return recoveryBroadcastError(err)
	}
	signed, err := m.signCappedCancellation(ctx, recovery.nonce)
	if signed != nil {
		recovery.cancellation = signed
		observability.Log(ctx).Info("capped nonce recovery cancellation submitted",
			"nonce", recovery.nonce, "hash", signed.Hash().Hex(), "maxFeePerGas", signed.GasFeeCap().String())
	}
	return recoveryBroadcastError(err)
}

func recoveryBroadcastError(err error) error {
	if err == nil || isKnownTransactionError(err) || isNonceConsumedError(err) {
		return nil
	}
	if isPendingNonceCollision(err) {
		return errors.Errorf("%w: capped recovery cancellation was underpriced", errReconciliationPending)
	}
	return err
}

// signCappedCancellation fixes every cancellation payload field, including both fee caps. Local-key
// replicas produce identical bytes; remote signers may use distinct signatures, which still cannot
// bid up the fee. An unavailable or insufficient cap leaves recovery pending rather than escalating.
func (m *Manager) signCappedCancellation(ctx context.Context, nonce uint64) (*types.Transaction, error) {
	limit := m.globalFeeLimit()
	if limit == nil || limit.Sign() <= 0 {
		return nil, errors.New("nonce recovery requires a positive global fee cap")
	}
	fees := feeQuote{baseFee: new(big.Int), tip: new(big.Int).Set(limit), maxFee: new(big.Int).Set(limit)}
	return m.signAndSend(ctx, nonce, m.signer.Address(), nil, new(big.Int), cancellationGasLimit, fees, true, true)
}

// cancelContestedNonce is the timeout path for a signed candidate rejected by another replica's
// same-nonce submission. It keeps all candidate hashes and signs only the fixed recovery cancellation.
func (m *Manager) cancelContestedNonce(ctx context.Context, pending *pendingTransaction) error {
	latest, err := m.readLatestNonce(ctx)
	if err != nil {
		return errors.Errorf("latest nonce before contested cancellation: %w", err)
	}
	if latest > pending.nonce {
		return nil // Receipt/account proof will finish this nonce; never cancel a later one.
	}
	poolNonce, err := m.readPendingNonce(ctx)
	if err != nil {
		return errors.Errorf("pending nonce before contested cancellation: %w", err)
	}
	if poolNonce < latest || poolNonce-latest > 1 || latest < pending.nonce {
		return nil // Multi-nonce or inconsistent views cannot authorize unknown-work cancellation.
	}
	// A stale submission endpoint cannot authorize cancelling work already mined on the ordinary
	// chain, even before that inclusion has the request's confirmation depth. Pin current state to
	// one stable canonical head; a lower nonce after a reorg also forbids signing ahead of a gap.
	current, err := m.canonicalAccountNonce(ctx, 0)
	if err != nil {
		return errors.Errorf("canonical current nonce before contested cancellation: %w", err)
	}
	if current != pending.nonce {
		return errors.Errorf("%w: contested nonce %d differs from canonical current nonce %d", errReconciliationPending, pending.nonce, current)
	}
	limit := m.globalFeeLimit()
	for _, attempt := range pending.attempts {
		if !attempt.cancellation || attempt.tx == nil {
			continue
		}
		if limit == nil || attempt.tx.GasFeeCap().Cmp(limit) > 0 {
			return errors.New("saved recovery cancellation exceeds the current global fee cap")
		}
		return recoveryBroadcastError(m.sendSigned(ctx, attempt.tx, true, true))
	}
	signed, sendErr := m.signCappedCancellation(ctx, pending.nonce)
	if signed != nil {
		pending.attempts = append(pending.attempts, txAttempt{hash: signed.Hash(), tx: signed, cancellation: true})
		pending.fees = feeQuote{baseFee: new(big.Int), tip: signed.GasTipCap(), maxFee: signed.GasFeeCap()}
	}
	if isKnownTransactionError(sendErr) || isPendingNonceCollision(sendErr) || isNonceConsumedError(sendErr) {
		return nil // The canonical winner, rather than this RPC acknowledgement, determines completion.
	}
	return sendErr
}
