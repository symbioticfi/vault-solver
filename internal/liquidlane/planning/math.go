package planning

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const bpsDenominator = 10_000

func scaleDown(amount *big.Int, numerator int) *big.Int {
	if amount == nil || amount.Sign() <= 0 || numerator <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Div(
		new(big.Int).Mul(amount, big.NewInt(int64(numerator))),
		big.NewInt(bpsDenominator),
	)
}

func scaleUp(amount *big.Int, numerator int) *big.Int {
	if amount == nil || amount.Sign() <= 0 || numerator <= 0 {
		return new(big.Int)
	}
	return liquidlane.MulDivUp(amount, big.NewInt(int64(numerator)), big.NewInt(bpsDenominator))
}

func cloneMin(left, right *big.Int) *big.Int {
	if left.Cmp(right) < 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}
