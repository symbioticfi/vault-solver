package bigmath

import (
	"math/big"
	"testing"
)

func TestDecimal(t *testing.T) {
	for _, tc := range []struct {
		amount string
		scale  int
		want   string
	}{
		{"-1234000", 6, "-1.234"}, {"-1", 18, "-0.000000000000000001"}, {"0", 18, "0"}, {"1", 18, "0.000000000000000001"}, {"1234000", 6, "1.234"},
		{"990000000000000000", 18, "0.99"}, {"1000000000000000000", 18, "1"},
		{"1234500000000000000", 18, "1.2345"}, {"1000000000000000000000", 18, "1000"},
		{"1000000", 6, "1"}, {"1234", 0, "1234"}, {"1234", -1, "1234"},
		{"123456789012345678901234567890123456789", 18, "123456789012345678901.234567890123456789"},
	} {
		t.Run(tc.amount+"/"+tc.want, func(t *testing.T) {
			amount, ok := new(big.Int).SetString(tc.amount, 10)
			if !ok {
				t.Fatal("invalid fixture")
			}
			if got := Decimal(amount, tc.scale); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if amount.String() != tc.amount {
				t.Fatal("formatting mutated the amount")
			}
		})
	}
}
