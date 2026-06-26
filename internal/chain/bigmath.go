package chain

import "math/big"

// Exp10 returns 10^n as a *big.Int — the canonical power-of-ten / token-decimals scale used across the
// solvers for fixed-point conversions (e.g. scaling between tokens of different decimals). n must be
// >= 0; negative exponents are not meaningful for decimal scales and yield 1 (big.Int.Exp on a negative
// exponent with a nil modulus returns 1).
func Exp10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
