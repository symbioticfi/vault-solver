package liquidlane

import (
	"math/big"
	"testing"
)

func mustBig(t *testing.T, raw string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		t.Fatalf("invalid integer %q", raw)
	}
	return n
}

func TestRateMathAcrossDecimals(t *testing.T) {
	rate := mustBig(t, "1000000000000000000")
	amountIn := mustBig(t, "1000000000000000000")
	amountOut := AmountOutForRate(amountIn, rate, 18, 6)
	if amountOut.String() != "1000000" {
		t.Fatalf("amountOut = %s", amountOut)
	}
	if got := RateForAmountOut(amountOut, amountIn, 18, 6); got.Cmp(rate) != 0 {
		t.Fatalf("rate = %s", got)
	}
	if got := MaxAmountInForRate(amountOut, rate, 18, 6); got.Cmp(amountIn) != 0 {
		t.Fatalf("max amountIn = %s", got)
	}
}

func TestMinAmountInForAmountOutRoundsUp(t *testing.T) {
	got := MinAmountInForAmountOut(
		big.NewInt(1),
		mustBig(t, "3000000000000000000"),
		18,
		6,
	)
	if got.String() != "333333333334" {
		t.Fatalf("min amountIn = %s", got)
	}
}

func TestMulDivUp(t *testing.T) {
	tests := map[string]struct {
		left, right, denominator *big.Int
		want                     string
	}{
		"exact":    {left: big.NewInt(6), right: big.NewInt(2), denominator: big.NewInt(3), want: "4"},
		"round up": {left: big.NewInt(5), right: big.NewInt(2), denominator: big.NewInt(3), want: "4"},
		"invalid":  {left: nil, right: big.NewInt(1), denominator: big.NewInt(1), want: "0"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := MulDivUp(tt.left, tt.right, tt.denominator).String(); got != tt.want {
				t.Fatalf("MulDivUp() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRateMathRejectsInvalidInput(t *testing.T) {
	if AmountOutForRate(nil, big.NewInt(1), 18, 6).Sign() != 0 {
		t.Fatal("nil amount must produce zero")
	}
	if RateForAmountOut(big.NewInt(1), big.NewInt(0), 18, 6).Sign() != 0 {
		t.Fatal("zero input must produce zero")
	}
}

// adapterDiscountAmountOut mirrors what LiquidLaneAdapter pays for a discount swap: getAmountOut
// floors amountIn × price × 10^outDec / (1e18 × 10^inDec), then swap(DiscountSwap, ...) applies the
// ppm discount and floors again.
func adapterDiscountAmountOut(amountIn, price *big.Int, discountPpm int64, inDec, outDec int) *big.Int {
	return AmountOutAfterDiscount(AmountOutForRate(amountIn, price, inDec, outDec), big.NewInt(discountPpm))
}

// advertisedRate mirrors how a discount offer's maxRate is derived (and how the adapter derives
// getMaxRate): the oracle price with the ppm discount applied, rounded down.
func advertisedRate(price *big.Int, discountPpm int64) *big.Int {
	rate := new(big.Int).Mul(price, big.NewInt(DiscountPrecision-discountPpm))
	return rate.Div(rate, big.NewInt(DiscountPrecision))
}

// The raw advertised rate over-predicts by exactly one unit here: the adapter's nested rounding
// (floor getAmountOut, then discount) lands a unit below pricing off the pre-discounted rate.
func TestConservativeAdvertisedRateFixesKnownOverprediction(t *testing.T) {
	price := mustBig(t, "1034567891234567890")
	amountIn := mustBig(t, "1000000000000000")
	rate := advertisedRate(price, 1)

	adapter := adapterDiscountAmountOut(amountIn, price, 1, 18, 18)
	if adapter.String() != "1034566856666675" {
		t.Fatalf("adapter amountOut = %s", adapter)
	}
	if raw := AmountOutForRate(amountIn, rate, 18, 18); raw.String() != "1034566856666676" {
		t.Fatalf("raw advertised amountOut = %s, want one unit above the adapter", raw)
	}

	safe := AmountOutForRate(amountIn, ConservativeAdvertisedRate(amountIn, rate, 18, 18), 18, 18)
	if safe.Cmp(adapter) != 0 {
		t.Fatalf("conservative amountOut = %s, want exactly the adapter value %s", safe, adapter)
	}
}

// The pricing invariant the fill depends on: at a conservative advertised rate we never predict more
// output than the adapter pays, across decimal pairs, discounts, and sizes.
func TestConservativeAdvertisedRateNeverExceedsAdapter(t *testing.T) {
	price := mustBig(t, "1034567891234567890")
	decimals := [][2]int{{18, 18}, {18, 6}, {6, 6}, {8, 18}, {6, 18}}
	discounts := []int64{0, 1, 100, 5_000, 250_000}

	for _, dec := range decimals {
		inDec, outDec := dec[0], dec[1]
		for _, discount := range discounts {
			rate := advertisedRate(price, discount)
			for step := int64(1); step <= 500; step++ {
				amountIn := new(big.Int).Mul(big.NewInt(step*7_919), pow10(max(inDec-6, 0)))
				safeRate := ConservativeAdvertisedRate(amountIn, rate, inDec, outDec)
				if safeRate.Sign() <= 0 {
					continue // not quotable at this size; the caller drops the leg
				}
				got := AmountOutForRate(amountIn, safeRate, inDec, outDec)
				if want := adapterDiscountAmountOut(amountIn, price, discount, inDec, outDec); got.Cmp(want) > 0 {
					t.Fatalf(
						"in=%d out=%d discount=%d amountIn=%s: predicted %s > adapter %s",
						inDec, outDec, discount, amountIn, got, want,
					)
				}
			}
		}
	}
}

func TestConservativeAdvertisedRateRejectsInvalidInput(t *testing.T) {
	rate := mustBig(t, "1000000000000000000")
	tests := map[string]struct {
		amountIn *big.Int
		rate     *big.Int
	}{
		"nil amount":  {amountIn: nil, rate: rate},
		"zero amount": {amountIn: new(big.Int), rate: rate},
		"nil rate":    {amountIn: rate, rate: nil},
		"dust output": {amountIn: big.NewInt(1), rate: rate}, // one unit of output at most, shaved to zero
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ConservativeAdvertisedRate(tt.amountIn, tt.rate, 18, 6); got.Sign() != 0 {
				t.Fatalf("ConservativeAdvertisedRate() = %s, want 0", got)
			}
		})
	}
}

func TestAmountOutAfterDiscount(t *testing.T) {
	tests := map[string]struct {
		gross    *big.Int
		discount *big.Int
		want     string
	}{
		"zero":          {gross: big.NewInt(1_000), discount: big.NewInt(0), want: "1000"},
		"ten percent":   {gross: big.NewInt(1_000), discount: big.NewInt(100_000), want: "900"},
		"full discount": {gross: big.NewInt(1_000), discount: big.NewInt(DiscountPrecision), want: "0"},
		"invalid":       {gross: big.NewInt(1_000), discount: big.NewInt(DiscountPrecision + 1), want: "0"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := AmountOutAfterDiscount(tt.gross, tt.discount).String(); got != tt.want {
				t.Fatalf("AmountOutAfterDiscount() = %s, want %s", got, tt.want)
			}
		})
	}
}
