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
	return AmountOutAfterDiscountPrecision(grossAmountOut, discount, DiscountPrecision)
}

// AmountOutAfterDiscountPrecision preserves the signed integer in its contract scale.
func AmountOutAfterDiscountPrecision(grossAmountOut, discount *big.Int, scale int64) *big.Int {
	precision := big.NewInt(scale)
	if grossAmountOut == nil || grossAmountOut.Sign() <= 0 || discount == nil || discount.Sign() < 0 ||
		scale <= 0 || discount.Cmp(precision) > 0 {
		return new(big.Int)
	}
	multiplier := new(big.Int).Sub(precision, discount)
	return new(big.Int).Div(new(big.Int).Mul(grossAmountOut, multiplier), precision)
}

// The adapter pays a swap in two floored steps, and the helpers below mirror them exactly:
//
//	getAmountOut(amountIn) = floor(amountIn × price × 10^outDec / (1e18 × 10^inDec))
//	payout(amountIn)       = floor(getAmountOut(amountIn) × (1e6 − discount) / 1e6)
//
// price is the adapter oracle price (1e18). discount is the signed discount of a discount swap,
// or the adapter minDiscount that bounds a direct swap. A single rate cannot reproduce the two
// floors for every amount, so exact pricing carries price and discount separately.

// OraclePriceProbe returns the amountIn for which getAmountOut returns the oracle price exactly:
// 10^(18 + inDec − outDec) cancels the decimal scaling. ok is false when outDec exceeds 18 + inDec.
func OraclePriceProbe(tokenInDecimals, tokenOutDecimals int) (amountIn *big.Int, ok bool) {
	exponent := 18 + tokenInDecimals - tokenOutDecimals
	if exponent < 0 {
		return nil, false
	}
	return pow10(exponent), true
}

// DiscountedRate is getMaxRate for an arbitrary discount: floor(price × (1e6 − discount) / 1e6).
// It ranks alternatives; amounts come from DiscountedAmountOut.
func DiscountedRate(price, discount *big.Int) *big.Int {
	return DiscountedRatePrecision(price, discount, DiscountPrecision)
}

// DiscountedRatePrecision applies the contract-native discount scale.
func DiscountedRatePrecision(price, discount *big.Int, scale int64) *big.Int {
	if !validPricingPrecision(price, discount, scale) {
		return new(big.Int)
	}
	return AmountOutAfterDiscountPrecision(price, discount, scale)
}

// DiscountedAmountOut returns exactly what the adapter pays for amountIn.
func DiscountedAmountOut(amountIn, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	return DiscountedAmountOutPrecision(amountIn, price, discount, tokenInDecimals, tokenOutDecimals, DiscountPrecision)
}

// DiscountedAmountOutPrecision applies the contract-native discount scale.
func DiscountedAmountOutPrecision(amountIn, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int, scale int64) *big.Int {
	if !validPricingPrecision(price, discount, scale) {
		return new(big.Int)
	}
	return AmountOutAfterDiscountPrecision(AmountOutForRate(amountIn, price, tokenInDecimals, tokenOutDecimals), discount, scale)
}

// MinAmountInForDiscountedAmountOut returns the smallest amountIn whose payout reaches amountOut:
// the gross output must reach ceil(amountOut × 1e6 / (1e6 − discount)), then getAmountOut inverts
// by rounding up. Zero marks invalid input.
func MinAmountInForDiscountedAmountOut(
	amountOut, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int,
) *big.Int {
	return MinAmountInForDiscountedAmountOutPrecision(amountOut, price, discount, tokenInDecimals, tokenOutDecimals, DiscountPrecision)
}

// MinAmountInForDiscountedAmountOutPrecision applies the contract-native discount scale.
func MinAmountInForDiscountedAmountOutPrecision(amountOut, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int, scale int64) *big.Int {
	if amountOut == nil || amountOut.Sign() <= 0 || !validPricingPrecision(price, discount, scale) {
		return new(big.Int)
	}
	grossAmountOut := MulDivUp(amountOut, big.NewInt(scale), new(big.Int).Sub(big.NewInt(scale), discount))
	return MinAmountInForAmountOut(grossAmountOut, price, tokenInDecimals, tokenOutDecimals)
}

// MaxAmountInForDiscountedAmountOut returns the largest amountIn whose payout stays within
// maxAmountOut. payout ≤ cap ⇔ gross ≤ floor(((cap+1) × 1e6 − 1) / (1e6 − discount)), and
// getAmountOut ≤ gross ⇔ amountIn ≤ floor(((gross+1) × 1e18 × 10^inDec − 1) / (price × 10^outDec)).
func MaxAmountInForDiscountedAmountOut(
	maxAmountOut, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int,
) *big.Int {
	return MaxAmountInForDiscountedAmountOutPrecision(maxAmountOut, price, discount, tokenInDecimals, tokenOutDecimals, DiscountPrecision)
}

// MaxAmountInForDiscountedAmountOutPrecision applies the contract-native discount scale.
func MaxAmountInForDiscountedAmountOutPrecision(maxAmountOut, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int, scale int64) *big.Int {
	if maxAmountOut == nil || maxAmountOut.Sign() < 0 || !validPricingPrecision(price, discount, scale) {
		return new(big.Int)
	}
	gross := new(big.Int).Add(maxAmountOut, big.NewInt(1))
	gross.Mul(gross, big.NewInt(scale))
	gross.Sub(gross, big.NewInt(1))
	gross.Div(gross, new(big.Int).Sub(big.NewInt(scale), discount))

	amountIn := gross.Add(gross, big.NewInt(1))
	amountIn.Mul(amountIn, rateScale)
	amountIn.Mul(amountIn, pow10(tokenInDecimals))
	amountIn.Sub(amountIn, big.NewInt(1))
	return amountIn.Div(amountIn, new(big.Int).Mul(price, pow10(tokenOutDecimals)))
}

func validPricingPrecision(price, discount *big.Int, scale int64) bool {
	return scale > 0 && price != nil && price.Sign() > 0 &&
		discount != nil && discount.Sign() >= 0 && discount.Cmp(big.NewInt(scale)) < 0
}
