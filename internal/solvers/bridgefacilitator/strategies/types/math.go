package types

import "math/big"

// RateDenominatorBps converts a basis-point rate to a fraction (10_000 = 100%).
const RateDenominatorBps = 10_000.0

// ExpectedReturn derives the absolute expected return for principal at rateBps basis points. 3F
// maxRate is expressed in bps with tenths-of-a-basis-point precision, so the denominator is 10_000.
// The result truncates down, keeping the offer at or below the requested rate.
func ExpectedReturn(principal *big.Int, rateBps float64) *big.Int {
	num := new(big.Float).Mul(new(big.Float).SetInt(principal), big.NewFloat(rateBps))
	num.Quo(num, big.NewFloat(RateDenominatorBps))
	out, _ := num.Int(nil)
	return out
}

// BpsToFloat converts an integer bps value to float64 for comparison against auction maxRate.
func BpsToFloat(n *big.Int) float64 {
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}
