package liquidlanemath

import (
	"math/big"
	"testing"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

func TestAmountOutForRate(t *testing.T) {
	got := AmountOutForRate(
		mustBig(t, "1000000000000000000"),
		mustBig(t, "1000000000000000000"),
		18,
		6,
	)
	if got.String() != "1000000" {
		t.Fatalf("AmountOutForRate = %s, want 1000000", got)
	}
}

func TestMaxAmountInForRate(t *testing.T) {
	got := MaxAmountInForRate(
		mustBig(t, "1000000"),
		mustBig(t, "1000000000000000000"),
		18,
		6,
	)
	if got.String() != "1000000000000000000" {
		t.Fatalf("MaxAmountInForRate = %s, want 1000000000000000000", got)
	}
}

func TestMinAmountInForAmountOutRoundsUp(t *testing.T) {
	got := MinAmountInForAmountOut(
		mustBig(t, "1"),
		mustBig(t, "3000000000000000000"),
		18,
		6,
	)
	if got.String() != "333333333334" {
		t.Fatalf("MinAmountInForAmountOut = %s, want 333333333334", got)
	}
}

func TestRateForAmountOut(t *testing.T) {
	got := RateForAmountOut(
		mustBig(t, "1000000"),
		mustBig(t, "1000000000000000000"),
		18,
		6,
	)
	if got.String() != "1000000000000000000" {
		t.Fatalf("RateForAmountOut = %s, want 1000000000000000000", got)
	}
}
