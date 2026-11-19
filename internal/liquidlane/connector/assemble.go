package connector

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	binding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

type assembledRoute struct {
	Calldata []byte
	Leaves   []common.Hash
}

func assembleRoute(recipe recipes.Recipe, tree *recipes.Tree, venue, token, recipient common.Address, amountIn, amountOut *big.Int, signed binding.ILiquidLaneConnectorDiscountSwap, signature []byte) (assembledRoute, error) {
	data, err := connectorBinding.TryPackSwap(signed, signature, recipient, amountIn)
	if err != nil {
		return assembledRoute{}, errors.Errorf("encode connector swap: %w", err)
	}
	actions := map[string]hexutil.Bytes{}
	inputs := map[uint]string{0: amountIn.String(), 1: amountOut.String()}
	managed, err := recipes.ManagedActions(recipe, tree)
	if err != nil {
		return assembledRoute{}, err
	}
	for _, action := range managed {
		boundToken, err := actionToken(action, venue)
		if err != nil {
			return assembledRoute{}, err
		}
		if boundToken != token {
			continue
		}
		actions[action.Key] = data
	}
	if len(actions) == 0 {
		return assembledRoute{}, errors.New("connector swap action missing")
	}

	inner, leaves, err := recipes.Assemble(recipe, tree, inputs, actions, nil)
	if err != nil {
		return assembledRoute{}, err
	}
	return assembledRoute{inner, leaves}, nil
}

// actionToken identifies a swap's statically committed RWA. Unrelated
// actions and dynamically selected tokens cannot advertise a cached route.
func actionToken(action recipes.ManagedAction, venue common.Address) (common.Address, error) {
	if action.Leaf.Target != venue || len(action.Leaf.Data) < 164 || !bytes.Equal(action.Leaf.Data[:4], connectorBinding.GetABI().Methods["swap"].ID) {
		return common.Address{}, nil
	}
	if !bytes.Equal(action.Leaf.Data[4:36], common.LeftPadBytes([]byte{128}, 32)) {
		return common.Address{}, errors.New("connector swap tuple offset must be 128")
	}
	if err := action.Pinned(4, 32); err != nil {
		return common.Address{}, err
	}
	var token *big.Int
	if value, err := action.Field(132); err == nil {
		if value.Source != recipes.Literal {
			return common.Address{}, nil
		}
		token = value.Value
	} else {
		if err := action.Pinned(132, 32); err != nil {
			return common.Address{}, err
		}
		token = new(big.Int).SetBytes(action.Leaf.Data[132:164])
	}
	if token == nil || token.Sign() <= 0 || token.BitLen() > 160 {
		return common.Address{}, errors.New("invalid connector swap token")
	}
	return common.BigToAddress(token), nil
}

func paymentConnector(recipe recipes.Recipe, tree *recipes.Tree) (venue, asset common.Address, err error) {
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	selectPayment := func(token, spender common.Address) error {
		if venue != (common.Address{}) || spender == (common.Address{}) || token == (common.Address{}) {
			return errors.New("recipe requires one payment approval from input 1")
		}
		venue, asset = spender, token
		return nil
	}
	for _, slot := range inspection.Slots {
		if slot.Operation != recipes.Approve || slot.Amount.Source != recipes.Input || slot.Amount.Value == nil || slot.Amount.Value.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if err := selectPayment(slot.Token, slot.Spender); err != nil {
			return common.Address{}, common.Address{}, err
		}
	}
	actions, err := recipes.ManagedActions(recipe, tree)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	selector := erc20.NewERC20().PackApprove(common.Address{}, new(big.Int))[:4]
	for _, action := range actions {
		if len(action.Leaf.Data) != 68 || !bytes.Equal(action.Leaf.Data[:4], selector) {
			continue
		}
		amount, err := action.Field(36)
		if err != nil || amount.Source != recipes.Input || amount.Value.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if action.Slot.Gate.Source != recipes.Literal || action.Slot.Gate.Value.Cmp(big.NewInt(1)) != 0 {
			return common.Address{}, common.Address{}, errors.New("recipe payment approval must be unconditional")
		}
		if err := action.Pinned(4, 32); err != nil {
			return common.Address{}, common.Address{}, err
		}
		if err := selectPayment(action.Leaf.Target, common.BytesToAddress(action.Leaf.Data[4:36])); err != nil {
			return common.Address{}, common.Address{}, err
		}
	}
	if venue == (common.Address{}) {
		return venue, asset, errors.New("recipe payment approval from input 1 missing")
	}
	return venue, asset, nil
}
