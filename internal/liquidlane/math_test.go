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
