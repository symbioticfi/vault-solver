// Package bigmath provides owned arbitrary-precision values without protocol dependencies.
package bigmath

import "math/big"

// Clone preserves nil; OrZero supplies a fresh numeric zero when absence means zero.
func Clone(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}
func OrZero(value *big.Int) *big.Int {
	if value == nil {
		return new(big.Int)
	}
	return Clone(value)
}

// Exp10 returns the decimal scale. As with big.Int.Exp, a negative exponent yields one.
func Exp10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}

// Min returns an independent copy of the smaller non-nil argument.
func Min(a, b *big.Int) *big.Int {
	if a.Cmp(b) < 0 {
		return Clone(a)
	}
	return Clone(b)
}
