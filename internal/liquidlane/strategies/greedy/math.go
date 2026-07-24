package greedy

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const bpsDenominator = 10_000

func applyBpsDown(amount *big.Int, bps int) *big.Int {
	if amount == nil || amount.Sign() <= 0 || bps <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Div(
		new(big.Int).Mul(amount, big.NewInt(int64(bps))),
		big.NewInt(bpsDenominator),
	)
}

func applyBpsUp(amount *big.Int, bps int) *big.Int {
	if amount == nil || amount.Sign() <= 0 || bps <= 0 {
		return new(big.Int)
	}
	return liquidlane.MulDivUp(amount, big.NewInt(int64(bps)), big.NewInt(bpsDenominator))
}

func minBig(left, right *big.Int) *big.Int {
	if left.Cmp(right) < 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}
