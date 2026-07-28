package liquidlane

import "math/big"

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

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func AmountOutForRate(amountIn, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if amountIn == nil || rate == nil || amountIn.Sign() <= 0 || rate.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountIn, rate)
	num.Mul(num, pow10(tokenOutDecimals))
	den := new(big.Int).Mul(rateScale, pow10(tokenInDecimals))
	return num.Div(num, den)
}

func MaxAmountInForRate(maxAssets, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if maxAssets == nil || rate == nil || maxAssets.Sign() <= 0 || rate.Sign() <= 0 {
		return new(big.Int)
	}
	den := new(big.Int).Mul(rate, pow10(tokenOutDecimals))
	num := new(big.Int).Mul(maxAssets, rateScale)
	num.Mul(num, pow10(tokenInDecimals))
	return num.Div(num, den)
}

func MinAmountInForAmountOut(amountOut, rate *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if amountOut == nil || rate == nil || amountOut.Sign() <= 0 || rate.Sign() <= 0 {
		return new(big.Int)
	}
	den := new(big.Int).Mul(rate, pow10(tokenOutDecimals))
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, pow10(tokenInDecimals))
	num.Add(num, new(big.Int).Sub(den, big.NewInt(1)))
	return num.Div(num, den)
}

func RateForAmountOut(amountOut, amountIn *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if amountOut == nil || amountIn == nil || amountOut.Sign() <= 0 || amountIn.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, pow10(tokenInDecimals))
	den := new(big.Int).Mul(amountIn, pow10(tokenOutDecimals))
	return num.Div(num, den)
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
