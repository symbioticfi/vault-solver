package txmanager

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

func (m *Manager) applyReplacementSend(
	pending *pendingTransaction,
	attempt *txAttempt,
	fees *feeQuote,
	sendErr error,
) (common.Hash, bool) {
	hash := attempt.hash
	uncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	if fees != nil {
		hash = attempt.tx.Hash()
		attempt.hash = hash
		attempt.exactRebroadcastPending = uncertain
		pending.fees = cloneFeeQuote(*fees)
		pending.attempts = append(pending.attempts, *attempt)
	}
	if isNonceConsumedError(sendErr) {
		pending.nonceConflictHash = hash
		m.markNonceConflict(pending.nonce, hash)
	}
	return hash, uncertain
}

func replacementBroadcastContext(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) (context.Context, context.CancelFunc) {
	if cancellation || pending.cancelDeadline.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, pending.cancelDeadline)
}

func (m *Manager) nextReplacementFees(
	ctx context.Context,
	previous feeQuote,
	limit *big.Int,
) (feeQuote, error) {
	current, freshErr := m.currentFees(ctx, nil)
	if freshErr != nil && !errors.Is(freshErr, errFreshFeesUnavailable) {
		return feeQuote{}, freshErr
	}
	requiredTip := bumpFee(previous.tip)
	requiredMaximum := bumpFee(previous.maxFee)
	next := feeQuote{
		baseFee: new(big.Int).Set(previous.baseFee),
		tip:     new(big.Int).Set(requiredTip),
		maxFee:  new(big.Int).Set(requiredMaximum),
	}
	if freshErr == nil {
		next.baseFee.Set(current.baseFee)
		next.maxFee = maxBigCopy(current.maxFee, next.maxFee)
	} else {
		m.log.V(1).Info("fresh replacement fees unavailable; using cached bump", "error", freshErr)
	}
	if limit != nil && next.maxFee.Cmp(limit) > 0 {
		next.maxFee.Set(limit)
	}
	effectiveTipLimit := new(big.Int).Sub(next.maxFee, next.baseFee)
	if effectiveTipLimit.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"replacement base fee %s exceeds fee limit %s",
			next.baseFee,
			next.maxFee,
		)
	}
	if freshErr == nil {
		freshTip := new(big.Int).Set(current.tip)
		if freshTip.Cmp(effectiveTipLimit) > 0 {
			freshTip.Set(effectiveTipLimit)
		}
		next.tip = maxBigCopy(freshTip, requiredTip)
	}
	if next.maxFee.Cmp(requiredMaximum) < 0 || next.tip.Cmp(next.maxFee) > 0 {
		return feeQuote{}, errors.Errorf(
			"%w: previous max fee %s tip %s, limit %s",
			errReplacementLimitReached,
			previous.maxFee,
			previous.tip,
			feeLimitString(limit),
		)
	}
	return next, nil
}

func (m *Manager) normalFeeLimit(request Request) *big.Int {
	limit := reserveFeeBump(m.globalFeeLimit())
	if request.MaxFeePerGas != nil && (limit == nil || request.MaxFeePerGas.Cmp(limit) < 0) {
		return new(big.Int).Set(request.MaxFeePerGas)
	}
	return limit
}

func (m *Manager) globalFeeLimit() *big.Int {
	if m.cfg.MaxFeeGwei <= 0 {
		return nil
	}
	return gweiToWei(m.cfg.MaxFeeGwei)
}
