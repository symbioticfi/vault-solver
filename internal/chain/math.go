package chain

import "math/big"

// Exp10 returns the integer decimal scale 10^exponent.
func Exp10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
