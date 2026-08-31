package liquidlane

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

var rateScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// MulDivUp returns ceil(left * right / denominator), or zero for invalid input.
func MulDivUp(left, right, denominator *big.Int) *big.Int {
	if left == nil || right == nil || denominator == nil ||
		left.Sign() <= 0 || right.Sign() <= 0 || denominator.Sign() <= 0 {
		return new(big.Int)
	}
	numerator := new(big.Int).Mul(left, right)
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func AmountOutForRate(amountIn, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if amountIn == nil || rate == nil || amountIn.Sign() <= 0 || rate.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountIn, rate)
	num.Mul(num, chain.Exp10(tokenOutDecimals))
	den := new(big.Int).Mul(rateScale, chain.Exp10(tokenInDecimals))
	return num.Div(num, den)
}

func MaxAmountInForRate(maxAssets, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if maxAssets == nil || rate == nil || maxAssets.Sign() <= 0 || rate.Sign() <= 0 {
		return new(big.Int)
	}
	den := new(big.Int).Mul(rate, chain.Exp10(tokenOutDecimals))
	num := new(big.Int).Mul(maxAssets, rateScale)
	num.Mul(num, chain.Exp10(tokenInDecimals))
	return num.Div(num, den)
}

func MinAmountInForAmountOut(amountOut, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if amountOut == nil || rate == nil || amountOut.Sign() <= 0 || rate.Sign() <= 0 {
		return new(big.Int)
	}
	den := new(big.Int).Mul(rate, chain.Exp10(tokenOutDecimals))
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, chain.Exp10(tokenInDecimals))
	num.Add(num, new(big.Int).Sub(den, big.NewInt(1)))
	return num.Div(num, den)
}

func RateForAmountOut(amountOut, amountIn *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if amountOut == nil || amountIn == nil || amountOut.Sign() <= 0 || amountIn.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, chain.Exp10(tokenInDecimals))
	den := new(big.Int).Mul(amountIn, chain.Exp10(tokenOutDecimals))
	return num.Div(num, den)
}

// ConservativeAdvertisedRate re-derives a fixed rate for amountIn from an advertised rate — one the
// backend produced by pre-applying a discount to the adapter's oracle price (a discount offer's
// maxRate) — so that pricing at the result never predicts more than the adapter pays.
//
// The adapter rounds down twice, in the opposite order: getAmountOut floors
// amountIn × price × 10^outDec / (1e18 × 10^inDec) first, then swap(DiscountSwap, ...) applies
// (DISCOUNT_PRECISION − discount) / DISCOUNT_PRECISION and floors again. An advertised rate has the
// discount applied and floored before we ever see it, so AmountOutForRate at that rate can land
// exactly one unit above the adapter's own result — enough to leave a filler short of an order's
// signed outputs. Shaving one unit and re-deriving the rate keeps every downstream
// AmountOutForRate(amountIn, ...) at or below the on-chain value, because that round trip through
// RateForAmountOut floors; no other call site has to know about the shave.
//
// Returns zero when nothing positive survives the shave, which marks the leg as not quotable.
func ConservativeAdvertisedRate(amountIn, advertisedRate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	amountOut := AmountOutForRate(amountIn, advertisedRate, tokenInDecimals, tokenOutDecimals)
	amountOut.Sub(amountOut, big.NewInt(1))
	if amountOut.Sign() <= 0 {
		return new(big.Int)
	}
	return RateForAmountOut(amountOut, amountIn, tokenInDecimals, tokenOutDecimals)
}

// AmountOutAfterDiscount applies a LiquidLane ppm discount, rounding down.
func AmountOutAfterDiscount(grossAmountOut, discount *big.Int) *big.Int {
	precision := big.NewInt(DiscountPrecision)
	if grossAmountOut == nil || grossAmountOut.Sign() <= 0 || discount == nil || discount.Sign() < 0 ||
		discount.Cmp(precision) > 0 {
		return new(big.Int)
	}
	multiplier := new(big.Int).Sub(precision, discount)
	return new(big.Int).Div(new(big.Int).Mul(grossAmountOut, multiplier), big.NewInt(DiscountPrecision))
}
