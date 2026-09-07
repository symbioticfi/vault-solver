package liquidlane

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"
)

// scaledRatio computes amount*multiplier/divisor with a decimal shift and one
// final rounding decision. Canceling decimal powers first avoids constructing
// both token scales for every pricing operation. All inputs remain unmodified.
func scaledRatio(amount, multiplier, divisor *big.Int, shift int, roundUp bool) *big.Int {
	for _, value := range []*big.Int{amount, multiplier, divisor} {
		if value == nil || value.Sign() <= 0 {
			return new(big.Int)
		}
	}
	numerator, denominator := new(big.Int).Mul(amount, multiplier), divisor
	if shift > 0 {
		numerator.Mul(numerator, bigmath.Exp10(shift))
	}
	if shift < 0 {
		denominator = new(big.Int).Mul(divisor, bigmath.Exp10(-shift))
	}
	if roundUp {
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
	}
	return numerator.Quo(numerator, denominator)
}

// MulDivUp returns ceil(left * right / denominator), or zero for invalid input.
func MulDivUp(left, right, denominator *big.Int) *big.Int {
	return scaledRatio(left, right, denominator, 0, true)
}

func AmountOutForRate(amountIn, rate *big.Int, inDecimals, outDecimals int) *big.Int {
	return scaledRatio(amountIn, rate, big.NewInt(1), outDecimals-inDecimals-18, false)
}

func MaxAmountInForRate(maxAssets, rate *big.Int, inDecimals, outDecimals int) *big.Int {
	return scaledRatio(maxAssets, big.NewInt(1), rate, 18+inDecimals-outDecimals, false)
}

func MinAmountInForAmountOut(amountOut, rate *big.Int, inDecimals, outDecimals int) *big.Int {
	return scaledRatio(amountOut, big.NewInt(1), rate, 18+inDecimals-outDecimals, true)
}

func RateForAmountOut(amountOut, amountIn *big.Int, inDecimals, outDecimals int) *big.Int {
	return scaledRatio(amountOut, big.NewInt(1), amountIn, 18+inDecimals-outDecimals, false)
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
func ConservativeAdvertisedRate(amountIn, advertisedRate *big.Int, inDecimals, outDecimals int) *big.Int {
	net := AmountOutForRate(amountIn, advertisedRate, inDecimals, outDecimals)
	return RateForAmountOut(net.Sub(net, big.NewInt(1)), amountIn, inDecimals, outDecimals)
}

// AmountOutAfterDiscount applies a LiquidLane ppm discount, rounding down.
func AmountOutAfterDiscount(gross, discount *big.Int) *big.Int {
	if discount == nil || discount.Sign() < 0 || discount.Cmp(big.NewInt(DiscountPrecision)) > 0 {
		return new(big.Int)
	}
	return scaledRatio(gross, new(big.Int).Sub(big.NewInt(DiscountPrecision), discount), big.NewInt(DiscountPrecision), 0, false)
}

// MaxRateForAmountOut is the largest integer rate whose rounded output at
// amountIn does not exceed amountOut. The first rate reaching amountOut+1 is an
// exclusive upper bound, so ceil(bound)-1 also handles exact divisibility.
func MaxRateForAmountOut(amountOut, amountIn *big.Int, inDecimals, outDecimals int) *big.Int {
	if amountOut == nil || amountOut.Sign() < 0 || amountIn == nil || amountIn.Sign() <= 0 {
		return new(big.Int)
	}
	nextOutput := new(big.Int).Add(amountOut, big.NewInt(1))
	bound := scaledRatio(nextOutput, big.NewInt(1), amountIn, 18+inDecimals-outDecimals, true)
	return bound.Sub(bound, big.NewInt(1))
}
