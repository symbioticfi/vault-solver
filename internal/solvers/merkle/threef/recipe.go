package threef

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/merkle/recipecapability"
	"github.com/symbioticfi/vault-solver/api/bindings/merkle/threefpolicy"
)

var capability = recipecapability.NewIRecipeCapability()
var policyABI = threefpolicy.NewIThreeFPolicy()
var tokenABI = erc4626.NewIERC4626()
var capabilitySchema = bindingABI(&recipecapability.IRecipeCapabilityMetaData)
var policySchema = bindingABI(&threefpolicy.IThreeFPolicyMetaData)

// IRecipes encodes queries and calls as internal instruction payloads.
var uintType = instructionType("uint256")
var addressType = instructionType("address")
var bytesType = instructionType("bytes")
var bindingsType = instructionType("uint256[]")
var queryArgs = abi.Arguments{{Type: addressType}, {Type: bytesType}, {Type: bindingsType}, {Type: uintType}}
var callArgs = queryArgs[:3]

// mintMarket identifies the policy and funding pair committed by the guarded leaf.
type mintMarket struct {
	Asset, UnderlyingAsset, Policy           common.Address
	PrincipalInput, YieldInput, RequestInput uint
}

func bindingABI(metadata *bind.MetaData) abi.ABI {
	parsed, err := metadata.ParseABI()
	if err != nil {
		panic(err)
	}
	return *parsed
}

func instructionType(name string) abi.Type {
	t, err := abi.NewType(name, "", nil)
	if err != nil {
		panic(err)
	}
	return t
}

// inspect extracts the funding pair and runtime inputs; guard policy belongs to
// the curator's recipe and is evaluated by the account during simulation.
func inspect(snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree) ([]mintMarket, error) {
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return nil, err
	}
	leaves := make(map[common.Hash]recipes.Leaf, len(tree.Leaves))
	for _, leaf := range tree.Leaves {
		leaves[leaf.Hash] = leaf
	}
	var markets []mintMarket
	for _, slot := range inspection.Slots {
		if slot.Operation != recipes.Manage {
			continue
		}
		for i, hash := range slot.Leaves {
			leaf := leaves[hash]
			action := recipes.ManagedAction{Leaf: leaf, Bindings: slot.Bindings[i]}
			if leaf.Target != snapshot.Account || len(leaf.Data) < 4 || !bytes.Equal(leaf.Data[:4], capabilitySchema.Methods["execute"].ID) {
				continue
			}
			market, err := inspectGuard(leaf)
			if err != nil {
				return nil, err
			}
			if market.Policy == (common.Address{}) {
				continue
			}
			// The 3F capability exposes PT, YT, and request in inner input words 0, 1, 4.
			base := int(new(big.Int).SetBytes(leaf.Data[68:100]).Uint64()) + 4 + 32
			for offset, destination := range map[int]*uint{base: &market.PrincipalInput, base + 32: &market.YieldInput, base + 128: &market.RequestInput} {
				value, err := action.Field(offset)
				if err != nil || value.Source != recipes.Input {
					return nil, errors.New("3f mint requires principal, yield and request inputs")
				}
				*destination = uint(value.Value.Uint64())
			}
			markets = append(markets, market)
		}
	}
	if len(markets) == 0 {
		return nil, errors.New("3f mint leaf missing")
	}
	return markets, nil
}

func inspectGuard(leaf recipes.Leaf) (mintMarket, error) {
	decoded, err := capabilitySchema.Methods["execute"].Inputs.Unpack(leaf.Data[4:])
	if err != nil {
		return mintMarket{}, errors.New("3f invalid mint execute")
	}
	method := policySchema.Methods["remainingAssets"]
	for _, query := range decoded[0].([][]byte) {
		values, err := queryArgs.Unpack(query)
		if err != nil {
			return mintMarket{}, err
		}
		data := values[1].([]byte)
		if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
			continue
		}
		pair, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return mintMarket{}, err
		}
		return mintMarket{Policy: values[0].(common.Address), Asset: pair[0].(common.Address), UnderlyingAsset: pair[1].(common.Address)}, nil
	}
	return mintMarket{}, nil
}
