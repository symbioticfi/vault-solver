package types

import "math/big"

var rateDenominatorDeciBps = big.NewInt(100_000)

// ExpectedReturn derives the absolute expected return for principal at an exact tenth-basis-point
// rate. Integer division rounds down, keeping the offer at or below the requested rate.
func ExpectedReturn(principal, rateDeciBps *big.Int) *big.Int {
	if principal == nil || rateDeciBps == nil {
		return new(big.Int)
	}
	return new(big.Int).Quo(
		new(big.Int).Mul(principal, rateDeciBps),
		rateDenominatorDeciBps,
	)
}
