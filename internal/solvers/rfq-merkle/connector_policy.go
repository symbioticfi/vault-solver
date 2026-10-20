package rfqmerkle

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
)

var connectorBalanceBinding = erc4626.NewIERC4626()

var connectorBalanceQueryArgs = func() abi.Arguments {
	args := make(abi.Arguments, 0, 4)
	for _, name := range []string{"address", "bytes", "uint256[]", "uint256"} {
		typ, err := abi.NewType(name, "", nil)
		if err != nil {
			panic(err)
		}
		args = append(args, abi.Argument{Type: typ})
	}
	return args
}()

type connectorDelta struct {
	token, holder common.Address
	input         uint64
	decrease      bool
}

// These committed comparisons bind upstream inputs to the swap's actual transfers.
func validateConnectorDeltas(recipe recipes.Recipe, swapStep uint, account, recipient, asset, rwa common.Address) error {
	if _, err := recipes.Inspect(recipe); err != nil {
		return errors.Errorf("inspect connector balance deltas: %w", err)
	}
	if swapStep >= uint(len(recipe.Steps)) || recipe.Steps[swapStep][0] != recipes.Manage {
		return errors.New("connector balance delta swap step is not Manage")
	}
	previous, next := -1, len(recipe.Steps)
	for i, step := range recipe.Steps {
		if step[0] != recipes.Manage && step[0] != recipes.Approve && step[0] != recipes.Call {
			continue
		}
		if i < int(swapStep) {
			previous = i
		} else if i > int(swapStep) {
			next = i
			break
		}
	}
	for _, delta := range []connectorDelta{{asset, account, 1, true}, {asset, recipient, 1, false}, {rwa, account, 0, false}} {
		found := false
		for i := int(swapStep) + 1; i < next; i++ {
			if connectorMatchesDelta(recipe, i, int(swapStep), previous, delta) {
				found = true
				break
			}
		}
		if !found {
			return errors.New("connector recipe must enforce exact account cash, RFQ payment and RWA balance deltas")
		}
	}
	return nil
}

func connectorMatchesDelta(recipe recipes.Recipe, comparison, swapStep, previous int, delta connectorDelta) bool {
	left, right, ok := connectorBinaryOperands(recipe, comparison, recipes.Equal)
	if !ok {
		return false
	}
	input, inputOK := connectorOperandIndex(left, recipes.Input)
	difference, differenceOK := connectorOperandIndex(right, recipes.Result)
	if !inputOK || !differenceOK {
		input, inputOK = connectorOperandIndex(right, recipes.Input)
		difference, differenceOK = connectorOperandIndex(left, recipes.Result)
	}
	if !inputOK || !differenceOK || input != delta.input || difference <= uint64(swapStep) || difference >= uint64(comparison) {
		return false
	}
	left, right, ok = connectorBinaryOperands(recipe, int(difference), recipes.Sub)
	if !ok {
		return false
	}
	beforeWord, afterWord := right, left
	if delta.decrease {
		beforeWord, afterWord = left, right
	}
	before, beforeOK := connectorOperandIndex(beforeWord, recipes.Result)
	after, afterOK := connectorOperandIndex(afterWord, recipes.Result)
	if !beforeOK || !afterOK || int(before) <= previous || before >= uint64(swapStep) || after <= uint64(swapStep) || after >= difference {
		return false
	}
	return connectorStaticBalance(recipe, before, delta.token, delta.holder) && connectorStaticBalance(recipe, after, delta.token, delta.holder)
}

func connectorBinaryOperands(recipe recipes.Recipe, index int, operation byte) (left, right []byte, ok bool) {
	if index < 0 || index >= len(recipe.Steps) || len(recipe.Steps[index]) < 65 || recipe.Steps[index][0] != operation {
		return nil, nil, false
	}
	step := recipe.Steps[index]
	return step[1:33], step[33:65], true
}

// The interpreter ignores the upper four bytes and decodes uint8/uint216 operands.
func connectorOperandIndex(word []byte, source uint8) (uint64, bool) {
	if len(word) != 32 || word[4] != source {
		return 0, false
	}
	index := new(big.Int).SetBytes(word[5:])
	return index.Uint64(), index.IsUint64()
}

func connectorStaticBalance(recipe recipes.Recipe, step uint64, token, holder common.Address) bool {
	if step >= uint64(len(recipe.Steps)) || len(recipe.Steps[step]) < 33 || recipe.Steps[step][0] != recipes.Read {
		return false
	}
	query := new(big.Int).SetBytes(recipe.Steps[step][1:33])
	if !query.IsUint64() || query.Uint64() >= uint64(len(recipe.Queries)) {
		return false
	}
	values, err := connectorBalanceQueryArgs.Unpack(recipe.Queries[query.Uint64()])
	if err != nil || len(values) != 4 {
		return false
	}
	return values[0].(common.Address) == token &&
		bytes.Equal(values[1].([]byte), connectorBalanceBinding.PackBalanceOf(holder)) &&
		len(values[2].([]*big.Int)) == 0 && values[3].(*big.Int).Sign() == 0
}
