package liquidlane

import (
	"math/big"
	"testing"
)

func TestMaximumRateBoundsRoundedOutput(t *testing.T) {
	for _, decimals := range [][2]int{{0, 0}, {6, 18}, {18, 6}, {0, 36}, {36, 0}} {
		for input := int64(1); input <= 19; input++ {
			for output := int64(0); output <= 19; output++ {
				in, out := big.NewInt(input), big.NewInt(output)
				rate := MaxRateForAmountOut(out, in, decimals[0], decimals[1])
				if actual := AmountOutForRate(in, rate, decimals[0], decimals[1]); actual.Cmp(out) > 0 {
					t.Fatalf("input=%s output=%s decimals=%v: rate %s overquotes %s", in, out, decimals, rate, actual)
				}
				next := new(big.Int).Add(rate, big.NewInt(1))
				if actual := AmountOutForRate(in, next, decimals[0], decimals[1]); actual.Cmp(out) <= 0 {
					t.Fatalf("input=%s output=%s decimals=%v: rate %s is not maximal", in, out, decimals, rate)
				}
				if in.Int64() != input || out.Int64() != output {
					t.Fatal("rate computation mutated its input")
				}
			}
		}
	}
}

func TestMaximumRateInvalidInput(t *testing.T) {
	for _, pair := range [][2]*big.Int{{nil, big.NewInt(1)}, {big.NewInt(-1), big.NewInt(1)}, {big.NewInt(1), nil}, {big.NewInt(1), new(big.Int)}, {big.NewInt(1), big.NewInt(-1)}} {
		if got := MaxRateForAmountOut(pair[0], pair[1], 6, 18); got.Sign() != 0 {
			t.Fatalf("invalid pair %v: %s", pair, got)
		}
	}
}
