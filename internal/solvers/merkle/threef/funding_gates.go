package threef

import (
	"math/big"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

// Each bit proves a value is zero when that market's principal is zero, on
// every successful execution. Reads supply no proof. Inspect has already
// validated operand lengths, sources and backward-only result references.
type principalActivation struct {
	inputs  map[uint]uint32
	results []uint32
	all     uint32
}

func validateFundingActivation(recipe recipes.Recipe, slots []recipes.RuntimeSlot, markets []mintMarket) error {
	proof := principalActivation{inputs: make(map[uint]uint32, len(markets)), results: make([]uint32, len(recipe.Steps))}
	for i, market := range markets {
		bit := uint32(1) << i
		proof.inputs[market.PrincipalInput] = bit
		proof.all |= bit
	}
	for i, step := range recipe.Steps {
		if step[0] < recipes.Add || step[0] > recipes.MulDiv {
			continue
		}
		left, right := proof.operand(step[1:33]), proof.operand(step[33:65])
		switch step[0] {
		case recipes.Add, recipes.Max:
			proof.results[i] = left & right
		case recipes.Sub, recipes.SaturatingSub:
			// Checked subtraction either stays zero or reverts; it cannot wrap.
			proof.results[i] = left
		case recipes.Min, recipes.MulDiv:
			// A zero numerator remains zero, or division reverts atomically.
			proof.results[i] = left | right
		}
	}
	for _, slot := range slots {
		if slot.Operation == recipes.Manage && (slot.Gate.Source == recipes.Literal || proof.value(slot.Gate) == 0) {
			return errors.New("3f Manage gate must deactivate with a recognized principal input")
		}
	}
	return nil
}

func (p principalActivation) operand(raw []byte) uint32 {
	return p.value(recipes.Value{Source: raw[4], Value: new(big.Int).SetBytes(raw[5:])})
}

func (p principalActivation) value(value recipes.Value) uint32 {
	switch value.Source {
	case recipes.Input:
		return p.inputs[uint(value.Value.Uint64())]
	case recipes.Result:
		return p.results[value.Value.Uint64()]
	case recipes.Literal:
		if value.Value.Sign() == 0 {
			return p.all
		}
	}
	return 0
}
