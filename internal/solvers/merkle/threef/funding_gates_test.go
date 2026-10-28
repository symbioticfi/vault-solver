package threef

import (
	"math/big"
	"testing"

	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func TestFundingResultActivation(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		ops                      []uint8
		left, right, denominator *big.Int
		want                     bool
	}{
		{"min principal cash", []uint8{recipes.Min}, encodedValue(recipes.Input, 1), encodedValue(recipes.Result, 3), nil, true},
		{"min cash principal", []uint8{recipes.Min}, encodedValue(recipes.Result, 3), encodedValue(recipes.Input, 1), nil, true},
		{"saturating shortfall", []uint8{recipes.SaturatingSub, recipes.Min}, encodedValue(recipes.Input, 1), encodedValue(recipes.Result, 3), nil, true},
		{"checked shortfall", []uint8{recipes.Sub}, encodedValue(recipes.Input, 1), encodedValue(recipes.Result, 3), nil, true},
		{"principal plus zero", []uint8{recipes.Add}, encodedValue(recipes.Input, 1), encodedValue(recipes.Literal, 0), nil, true},
		{"principal max zero", []uint8{recipes.Max}, encodedValue(recipes.Input, 1), encodedValue(recipes.Literal, 0), nil, true},
		{"scaled principal", []uint8{recipes.MulDiv}, encodedValue(recipes.Input, 1), encodedValue(recipes.Literal, 10), encodedValue(recipes.Literal, 3), true},
		{"second numerator principal", []uint8{recipes.MulDiv}, encodedValue(recipes.Result, 3), encodedValue(recipes.Input, 1), encodedValue(recipes.Literal, 10), true},
		{"cash plus principal", []uint8{recipes.Add}, encodedValue(recipes.Result, 3), encodedValue(recipes.Input, 1), nil, false},
		{"cash max principal", []uint8{recipes.Max}, encodedValue(recipes.Result, 3), encodedValue(recipes.Input, 1), nil, false},
		{"reversed shortfall", []uint8{recipes.SaturatingSub}, encodedValue(recipes.Result, 3), encodedValue(recipes.Input, 1), nil, false},
		{"denominator only principal", []uint8{recipes.MulDiv}, encodedValue(recipes.Literal, 10), encodedValue(recipes.Literal, 10), encodedValue(recipes.Input, 1), false},
		{"request minimum cash", []uint8{recipes.Min}, encodedValue(recipes.Input, 0), encodedValue(recipes.Result, 3), nil, false},
		{"yield minimum cash", []uint8{recipes.Min}, encodedValue(recipes.Input, 2), encodedValue(recipes.Result, 3), nil, false},
		{"other market keeps sum active", []uint8{recipes.Add}, encodedValue(recipes.Input, 1), encodedValue(recipes.Input, 4), nil, false},
		{"both markets constrain minimum", []uint8{recipes.Min}, encodedValue(recipes.Input, 1), encodedValue(recipes.Input, 4), nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, recipe, tree := groupedFixture(t)
			left := tc.left
			var gate *big.Int
			for _, op := range tc.ops {
				step := abiBinary(op, left, tc.right)
				if op == recipes.MulDiv {
					step = append(step, tc.denominator.FillBytes(make([]byte, 32))...)
				}
				gate = encodedValue(recipes.Result, uint64(len(recipe.Steps)))
				recipe.Steps = append(recipe.Steps, step)
				left = gate
			}
			fundingManage(t, &recipe, tree, gate, false)
			_, err := inspect(snapshot, recipe, tree)
			if (err == nil) != tc.want {
				t.Fatalf("funding activation accepted=%v want=%v: %v", err == nil, tc.want, err)
			}
		})
	}
}
