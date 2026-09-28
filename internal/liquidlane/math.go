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
	if !validPricing(price, discount) {
		return new(big.Int)
	}
	return AmountOutAfterDiscount(price, discount)
}

// DiscountedAmountOut returns exactly what the adapter pays for amountIn.
func DiscountedAmountOut(amountIn, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int) *big.Int {
	if !validPricing(price, discount) {
		return new(big.Int)
	}
	return AmountOutAfterDiscount(AmountOutForRate(amountIn, price, tokenInDecimals, tokenOutDecimals), discount)
}

// MinAmountInForDiscountedAmountOut returns the smallest amountIn whose payout reaches amountOut:
// the gross output must reach ceil(amountOut × 1e6 / (1e6 − discount)), then getAmountOut inverts
// by rounding up. Zero marks invalid input.
func MinAmountInForDiscountedAmountOut(
	amountOut, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int,
) *big.Int {
	if amountOut == nil || amountOut.Sign() <= 0 || !validPricing(price, discount) {
		return new(big.Int)
	}
	grossAmountOut := MulDivUp(amountOut, big.NewInt(DiscountPrecision), payoutMultiplier(discount))
	return MinAmountInForAmountOut(grossAmountOut, price, tokenInDecimals, tokenOutDecimals)
}

// MaxAmountInForDiscountedAmountOut returns the largest amountIn whose payout stays within
// maxAmountOut. payout ≤ cap ⇔ gross ≤ floor(((cap+1) × 1e6 − 1) / (1e6 − discount)), and
// getAmountOut ≤ gross ⇔ amountIn ≤ floor(((gross+1) × 1e18 × 10^inDec − 1) / (price × 10^outDec)).
func MaxAmountInForDiscountedAmountOut(
	maxAmountOut, price, discount *big.Int, tokenInDecimals, tokenOutDecimals int,
) *big.Int {
	if maxAmountOut == nil || maxAmountOut.Sign() < 0 || !validPricing(price, discount) {
		return new(big.Int)
	}
	gross := new(big.Int).Add(maxAmountOut, big.NewInt(1))
	gross.Mul(gross, big.NewInt(DiscountPrecision))
	gross.Sub(gross, big.NewInt(1))
	gross.Div(gross, payoutMultiplier(discount))

	amountIn := gross.Add(gross, big.NewInt(1))
	amountIn.Mul(amountIn, rateScale)
	amountIn.Mul(amountIn, pow10(tokenInDecimals))
	amountIn.Sub(amountIn, big.NewInt(1))
	return amountIn.Div(amountIn, new(big.Int).Mul(price, pow10(tokenOutDecimals)))
}

// validPricing accepts a positive price and a discount that leaves a positive payout.
func validPricing(price, discount *big.Int) bool {
	return price != nil && price.Sign() > 0 &&
		discount != nil && discount.Sign() >= 0 && discount.Cmp(big.NewInt(DiscountPrecision)) < 0
}

func payoutMultiplier(discount *big.Int) *big.Int {
	return new(big.Int).Sub(big.NewInt(DiscountPrecision), discount)
}
