package types

import (
	"math/big"
	"testing"
)

func TestExpectedReturn(t *testing.T) {
	// 100,000 USDC (6 decimals) at 200 bps (2%) => 2,000 USDC.
	principal := new(big.Int).SetUint64(100_000_000_000)
	got := ExpectedReturn(principal, big.NewInt(2_000))
	want := new(big.Int).SetUint64(2_000_000_000)
	if got.Cmp(want) != 0 {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestExpectedReturnUsesExactDeciBps(t *testing.T) {
	principal := mustBig(t, "900719925474099300000")
	got := ExpectedReturn(principal, big.NewInt(501))
	want := new(big.Int).Quo(
		new(big.Int).Mul(principal, big.NewInt(501)),
		big.NewInt(100_000),
	)
	if got.Cmp(want) != 0 {
		t.Fatalf("return = %s, want %s", got, want)
	}
}

func TestExpectedReturnNilInputs(t *testing.T) {
	tests := []struct {
		name      string
		principal *big.Int
		rate      *big.Int
	}{
		{name: "nil principal", rate: big.NewInt(501)},
		{name: "nil rate", principal: big.NewInt(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpectedReturn(tc.principal, tc.rate); got.Sign() != 0 {
				t.Fatalf("return = %s, want 0", got)
			}
		})
	}
}
