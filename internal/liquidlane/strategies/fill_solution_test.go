package strategies

import (
	"math/big"
	"testing"
)

func TestFillSolutionOwnsAmountsAndFinalizesWithGas(t *testing.T) {
	routes := []FillRoute{
		{AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(200), ReservedAmountOut: big.NewInt(210)},
		{AmountIn: big.NewInt(400), ExpectedAmountOut: big.NewInt(800), ReservedAmountOut: big.NewInt(840)},
	}
	gas := big.NewInt(3)
	solution := NewFillSolution(routes, gas)
	if solution == nil || solution.MaxAmountOut().Int64() != 997 {
		t.Fatal("incorrect net output")
	}
	// Neither caller-owned inputs nor a returned plan may alter the reusable solution.
	routes[0].AmountIn.SetInt64(1)
	routes[0].ExpectedAmountOut.SetInt64(1)
	routes[0].ReservedAmountOut.SetInt64(1)
	gas.SetInt64(1000)
	solution.MaxAmountOut().SetInt64(1)
	for range 2 {
		plan := solution.Finalize(big.NewInt(500))
		if len(plan) != 2 || plan[0].AmountIn.Int64() != 100 ||
			plan[0].ExpectedAmountOut.Int64() != 200 || plan[0].ReservedAmountOut.Int64() != 210 ||
			plan[0].MinAmountOut.Int64() != 100 || plan[1].MinAmountOut.Int64() != 403 {
			t.Fatalf("unexpected finalized plan: %+v", plan)
		}
		plan[0].AmountIn.SetInt64(1)
		plan[0].ExpectedAmountOut.SetInt64(1)
		plan[0].ReservedAmountOut.SetInt64(1)
		plan[0].MinAmountOut.SetInt64(1)
	}
	if len(solution.Finalize(big.NewInt(998))) != 0 {
		t.Fatal("accepted requirement above net output")
	}
	exact := solution.Finalize(big.NewInt(997))
	if len(exact) != 2 || exact[0].MinAmountOut.Int64() != 200 || exact[1].MinAmountOut.Int64() != 800 {
		t.Fatal("exact net output must remain fillable")
	}
}

func TestNewFillSolutionRejectsUnusableAllocations(t *testing.T) {
	for _, test := range []struct {
		name    string
		outputs []*big.Int
		gas     *big.Int
		wantNil bool
	}{
		{"no routes", nil, nil, true},
		{"missing output", []*big.Int{nil}, nil, true},
		{"zero output", []*big.Int{big.NewInt(0)}, nil, true},
		{"negative output", []*big.Int{big.NewInt(-1)}, nil, true},
		{"negative gas", []*big.Int{big.NewInt(10)}, big.NewInt(-1), true},
		{"gas consumes output", []*big.Int{big.NewInt(10)}, big.NewInt(10), true},
		{"gas exceeds output", []*big.Int{big.NewInt(10)}, big.NewInt(11), true},
		{"no gas", []*big.Int{big.NewInt(10)}, nil, false},
		{"positive net", []*big.Int{big.NewInt(10)}, big.NewInt(9), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			routes := make([]FillRoute, len(test.outputs))
			for i, output := range test.outputs {
				routes[i].ExpectedAmountOut = output
			}
			if solution := NewFillSolution(routes, test.gas); (solution == nil) != test.wantNil {
				t.Fatalf("solution=%v, want nil=%t", solution, test.wantNil)
			}
		})
	}
}
