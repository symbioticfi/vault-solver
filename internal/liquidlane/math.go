package liquidlane

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

var rateScale = chain.Exp10(18)

// MulDivUp computes ceil(left*right/denominator). Invalid or non-positive input is not quotable.
func MulDivUp(left, right, denominator *big.Int) *big.Int {
	if !positive(left) || !positive(right) || !positive(denominator) {
		return new(big.Int)
	}

	product := new(big.Int).Mul(left, right)
	quotient, remainder := new(big.Int).QuoRem(product, denominator, new(big.Int))
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

// AmountOutForRate converts input units at an 18-decimal fixed-point rate, rounding down.
func AmountOutForRate(amountIn, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if !positive(amountIn) || !positive(rate) {
		return new(big.Int)
	}

	numerator := new(big.Int).Mul(amountIn, rate)
	numerator.Mul(numerator, chain.Exp10(tokenOutDecimals))
	denominator := new(big.Int).Mul(rateScale, chain.Exp10(tokenInDecimals))
	return numerator.Quo(numerator, denominator)
}

// MaxAmountInForRate returns the largest input whose priced output fits maxAssets.
func MaxAmountInForRate(maxAssets, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if !positive(maxAssets) || !positive(rate) {
		return new(big.Int)
	}

	numerator := new(big.Int).Mul(maxAssets, rateScale)
	numerator.Mul(numerator, chain.Exp10(tokenInDecimals))
	denominator := new(big.Int).Mul(rate, chain.Exp10(tokenOutDecimals))
	return numerator.Quo(numerator, denominator)
}

// MinAmountInForAmountOut returns the least input that prices to amountOut, rounding up.
func MinAmountInForAmountOut(amountOut, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if !positive(amountOut) || !positive(rate) {
		return new(big.Int)
	}

	numerator := new(big.Int).Mul(amountOut, rateScale)
	numerator.Mul(numerator, chain.Exp10(tokenInDecimals))
	denominator := new(big.Int).Mul(rate, chain.Exp10(tokenOutDecimals))
	return MulDivUp(numerator, big.NewInt(1), denominator)
}

// RateForAmountOut derives an 18-decimal fixed-point rate, rounding down.
func RateForAmountOut(amountOut, amountIn *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if !positive(amountOut) || !positive(amountIn) {
		return new(big.Int)
	}

	numerator := new(big.Int).Mul(amountOut, rateScale)
	numerator.Mul(numerator, chain.Exp10(tokenInDecimals))
	denominator := new(big.Int).Mul(amountIn, chain.Exp10(tokenOutDecimals))
	return numerator.Quo(numerator, denominator)
}

// ConservativeAdvertisedRate compensates for the adapter's two-stage discount rounding.
func ConservativeAdvertisedRate(amountIn, advertisedRate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	amountOut := AmountOutForRate(amountIn, advertisedRate, tokenInDecimals, tokenOutDecimals)
	if amountOut.Cmp(big.NewInt(1)) <= 0 {
		return new(big.Int)
	}
	amountOut.Sub(amountOut, big.NewInt(1))
	return RateForAmountOut(amountOut, amountIn, tokenInDecimals, tokenOutDecimals)
}

// AmountOutAfterDiscount applies a ppm discount and rounds down exactly like the adapter.
func AmountOutAfterDiscount(grossAmountOut, discount *big.Int) *big.Int {
	precision := big.NewInt(DiscountPrecision)
	if !positive(grossAmountOut) || discount == nil || discount.Sign() < 0 || discount.Cmp(precision) > 0 {
		return new(big.Int)
	}

	multiplier := new(big.Int).Sub(precision, discount)
	return new(big.Int).Quo(new(big.Int).Mul(grossAmountOut, multiplier), precision)
}

func positive(value *big.Int) bool {
	return value != nil && value.Sign() > 0
}
