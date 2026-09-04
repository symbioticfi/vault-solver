package txmanager

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/params"
)

func cloneRequest(request Request) Request {
	request.Data = append([]byte(nil), request.Data...)
	request.MaxFeePerGas = cloneOptionalBig(request.MaxFeePerGas)
	return request
}

func cloneOptionalBig(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func cloneFeeQuote(quote feeQuote) feeQuote {
	return feeQuote{
		baseFee: new(big.Int).Set(quote.baseFee),
		tip:     new(big.Int).Set(quote.tip),
		maxFee:  new(big.Int).Set(quote.maxFee),
	}
}

func bumpFee(value *big.Int) *big.Int {
	numerator := new(big.Int).Mul(value, big.NewInt(replacementBumpNumerator))
	numerator.Add(numerator, big.NewInt(replacementBumpDenominator-1))
	bumped := numerator.Div(numerator, big.NewInt(replacementBumpDenominator))
	if bumped.Cmp(value) <= 0 {
		bumped.Add(value, big.NewInt(1))
	}
	return bumped
}

func reserveFeeBump(limit *big.Int) *big.Int {
	if limit == nil {
		return nil
	}
	reserved := new(big.Int).Mul(limit, big.NewInt(replacementBumpDenominator))
	return reserved.Div(reserved, big.NewInt(replacementBumpNumerator))
}

func maxBigCopy(left, right *big.Int) *big.Int {
	if left.Cmp(right) >= 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}

func feeLimitString(limit *big.Int) string {
	if limit == nil {
		return "unbounded"
	}
	return limit.String()
}

func attemptHashStrings(attempts []txAttempt) []string {
	hashes := make([]string, len(attempts))
	for index, attempt := range attempts {
		hashes[index] = attempt.hash.Hex()
	}
	return hashes
}

func gweiToWei(gwei float64) *big.Int {
	wei, _ := new(big.Float).Mul(big.NewFloat(gwei), big.NewFloat(params.GWei)).Int(nil)
	return wei
}

func (m *Manager) cancellationDeadline(request Request) time.Time {
	deadline := time.Now().Add(m.cfg.PendingTimeout)
	if !request.CancelAt.IsZero() && request.CancelAt.Before(deadline) {
		return request.CancelAt
	}
	return deadline
}

func (m *Manager) feeReadTimeout() time.Duration {
	return minPositiveDuration(maxFeeReadTimeout, m.cfg.ReplacementInterval/2)
}

func (m *Manager) receiptReadTimeout() time.Duration {
	return minPositiveDuration(maxReceiptReadTimeout, m.cfg.ReplacementInterval/2)
}

func minPositiveDuration(fallback, candidate time.Duration) time.Duration {
	if candidate > 0 && candidate < fallback {
		return candidate
	}
	return fallback
}
