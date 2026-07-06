package strategytypes

import (
	"math/big"
	"testing"
)

func TestExpectedReturn(t *testing.T) {
	// 100,000 USDC (6 decimals) at 200 bps (2%) => 2,000 USDC.
	principal := new(big.Int).SetUint64(100_000_000_000)
	got := ExpectedReturn(principal, 200)
	want := new(big.Int).SetUint64(2_000_000_000)
	if got.Cmp(want) != 0 {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
