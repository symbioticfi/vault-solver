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

// Payouts the live mainnet LiquidLane adapter 0x59CDDE0D345c0eE6ED453FE7d3fb365Fe0721E85 paid on a
// fork of block 26076942 (USDC collateral, minDiscount 0, so getMaxRate is the oracle price).
func TestDiscountedAmountOutMatchesMainnetAdapter(t *testing.T) {
	tests := []struct {
		name            string
		price, amountIn string
		inDec           int
		discount        int64
		payout          string
	}{
		{"mHYPER", "1128372140000000000", "1000000000000000000000", 18, 200, "1128146465"},
		{"PRIME", "1061774000000000000", "123456789", 6, 200, "131056991"},
		{"mROX", "1150136460000000000", "123456789012345678901", 18, 1_000, "141850161"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, amountIn := mustBig(t, tt.price), mustBig(t, tt.amountIn)
			got := DiscountedAmountOut(amountIn, price, big.NewInt(tt.discount), tt.inDec, 6)
			if got.String() != tt.payout {
				t.Fatalf("DiscountedAmountOut() = %s, want the executed payout %s", got, tt.payout)
			}
		})
	}
}

// contractPayout restates LiquidLaneAdapter independently of the helpers under test:
// amountIn.mulDiv(price * 10**outDec, 1e18 * 10**inDec).mulDiv(1e6 - discount, 1e6).
func contractPayout(amountIn, price *big.Int, discount int64, inDec, outDec int) *big.Int {
	ten := big.NewInt(10)
	gross := new(big.Int).Mul(price, new(big.Int).Exp(ten, big.NewInt(int64(outDec)), nil))
	gross.Mul(gross, amountIn)
	gross.Quo(gross, new(big.Int).Mul(big.NewInt(1e18), new(big.Int).Exp(ten, big.NewInt(int64(inDec)), nil)))
	gross.Mul(gross, big.NewInt(1_000_000-discount))
	return gross.Quo(gross, big.NewInt(1_000_000))
}

func TestDiscountedPricingMirrorsAdapterAndInvertsExactly(t *testing.T) {
	// 8- and 6-decimal oracle prices as read from the mainnet adapter's feeds, plus a round one.
	prices := []string{
		"1128372140000000000", "1150136460000000000", "1061774000000000000",
		"1117275000000000000", "989216000000000000", "1000000000000000000",
	}
	decimals := [][2]int{{18, 6}, {6, 6}, {18, 18}, {8, 6}, {6, 18}, {0, 6}}
	discounts := []int64{0, 1, 37, 200, 1_000, 40_000, 100_000, 999_999}
	for _, dec := range decimals {
		inDec, outDec := dec[0], dec[1]
		for _, raw := range prices {
			price := mustBig(t, raw)
			for _, ppm := range discounts {
				discount := big.NewInt(ppm)
				if want := AmountOutAfterDiscount(price, discount); DiscountedRate(price, discount).Cmp(want) != 0 {
					t.Fatalf("DiscountedRate = %s, want getMaxRate formula %s", DiscountedRate(price, discount), want)
				}
				// Sizes from dust to millions of tokens, off round numbers.
				for step := int64(1); step <= 40; step++ {
					amountIn := new(big.Int).Mul(big.NewInt(step*step*step*7_919+step*104_729), pow10(max(inDec-4, 0)))
					checkDiscountedPricing(t, amountIn, price, discount, inDec, outDec)
				}
			}
		}
	}
}

func checkDiscountedPricing(t *testing.T, amountIn, price, discount *big.Int, inDec, outDec int) {
	t.Helper()
	payout := DiscountedAmountOut(amountIn, price, discount, inDec, outDec)
	if want := contractPayout(amountIn, price, discount.Int64(), inDec, outDec); payout.Cmp(want) != 0 {
		t.Fatalf("payout(%s) = %s, contract %s", amountIn, payout, want)
	}
	if payout.Sign() > 0 {
		minimum := MinAmountInForDiscountedAmountOut(payout, price, discount, inDec, outDec)
		below := new(big.Int).Sub(minimum, big.NewInt(1))
		if DiscountedAmountOut(minimum, price, discount, inDec, outDec).Cmp(payout) < 0 ||
			DiscountedAmountOut(below, price, discount, inDec, outDec).Cmp(payout) >= 0 {
			t.Fatalf("minimum input %s for %s is not minimal", minimum, payout)
		}
	}
	maximum := MaxAmountInForDiscountedAmountOut(payout, price, discount, inDec, outDec)
	above := new(big.Int).Add(maximum, big.NewInt(1))
	if DiscountedAmountOut(maximum, price, discount, inDec, outDec).Cmp(payout) > 0 ||
		DiscountedAmountOut(above, price, discount, inDec, outDec).Cmp(payout) <= 0 {
		t.Fatalf("maximum input %s for cap %s is not maximal", maximum, payout)
	}
}

func TestOraclePriceProbeReturnsPriceThroughGetAmountOut(t *testing.T) {
	price := mustBig(t, "1128372140000000000")
	for _, dec := range [][2]int{{18, 6}, {6, 6}, {18, 18}, {0, 18}, {6, 24}} {
		probe, ok := OraclePriceProbe(dec[0], dec[1])
		if !ok {
			t.Fatalf("decimals %v: probe unavailable", dec)
		}
		if got := AmountOutForRate(probe, price, dec[0], dec[1]); got.Cmp(price) != 0 {
			t.Fatalf("decimals %v: getAmountOut(probe) = %s, want price", dec, got)
		}
	}
	if _, ok := OraclePriceProbe(0, 19); ok {
		t.Fatal("probe must be unavailable when outDec exceeds 18 + inDec")
	}
}

func TestDiscountedPricingRejectsInvalidInput(t *testing.T) {
	price := mustBig(t, "1000000000000000000")
	for name, discount := range map[string]*big.Int{
		"nil":           nil,
		"negative":      big.NewInt(-1),
		"full discount": big.NewInt(DiscountPrecision),
	} {
		t.Run(name, func(t *testing.T) {
			if DiscountedAmountOut(price, price, discount, 18, 6).Sign() != 0 ||
				MinAmountInForDiscountedAmountOut(big.NewInt(1), price, discount, 18, 6).Sign() != 0 ||
				MaxAmountInForDiscountedAmountOut(big.NewInt(1), price, discount, 18, 6).Sign() != 0 ||
				DiscountedRate(price, discount).Sign() != 0 {
				t.Fatal("invalid discount must price nothing")
			}
		})
	}
	if DiscountedRate(nil, new(big.Int)).Sign() != 0 || DiscountedAmountOut(price, new(big.Int), new(big.Int), 18, 6).Sign() != 0 {
		t.Fatal("missing price must price nothing")
	}
}
