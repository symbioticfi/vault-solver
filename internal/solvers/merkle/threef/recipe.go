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
	Asset, UnderlyingAsset, Policy common.Address
	Principal, Yield               recipes.Value
	RequestInput                   uint
}

func (m mintMarket) inputs() []uint {
	indices := []uint{m.RequestInput}
	for _, value := range []recipes.Value{m.Principal, m.Yield} {
		if value.Source == recipes.Input {
			indices = append(indices, uint(value.Value.Uint64()))
		}
	}
	return indices
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
			// Map the consume call's inner inputs through the executor's bindings.
			base := int(new(big.Int).SetBytes(leaf.Data[68:100]).Uint64()) + 4 + 32
			for _, destination := range []*recipes.Value{&market.Principal, &market.Yield} {
				value, err := action.Field(base + 32*int(destination.Value.Uint64()))
				if err != nil || (value.Source != recipes.Input && value.Source != recipes.Result) {
					return nil, errors.New("3f mint requires principal and yield bindings")
				}
				*destination = value
			}
			request, err := action.Field(base + 32*int(market.RequestInput))
			if err != nil || request.Source != recipes.Input {
				return nil, errors.New("3f mint requires a request input")
			}
			market.RequestInput = uint(request.Value.Uint64())
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
	inner := recipes.Recipe{Queries: decoded[0].([][]byte), Steps: decoded[1].([][]byte)}
	if _, err := recipes.Inspect(inner); err != nil {
		return mintMarket{}, err
	}
	var market mintMarket
	for _, step := range inner.Steps {
		if step[0] != recipes.Call {
			continue
		}
		values, err := callArgs.Unpack(step[1:])
		if err != nil {
			return mintMarket{}, err
		}
		data := values[1].([]byte)
		if len(data) < 4 || !bytes.Equal(data[:4], policySchema.Methods["consume"].ID) {
			continue
		}
		if market.Policy != (common.Address{}) {
			return mintMarket{}, errors.New("3f mint has multiple policy calls")
		}
		market.Policy = values[0].(common.Address)
		var indices [3]uint
		for i := range indices {
			indices[i], err = callInput(values[2].([]*big.Int), uint32(4+32*i))
			if err != nil {
				return mintMarket{}, err
			}
		}
		market.RequestInput = indices[0]
		market.Principal = recipes.Value{Source: recipes.Input, Value: new(big.Int).SetUint64(uint64(indices[1]))}
		market.Yield = recipes.Value{Source: recipes.Input, Value: new(big.Int).SetUint64(uint64(indices[2]))}
	}
	if market.Policy == (common.Address{}) {
		return market, nil
	}
	// A remainingAssets query may pin the pair. Otherwise discover the underlying
	// from the auction and let policy.consume attest the pair before execution.
	method := policySchema.Methods["remainingAssets"]
	for _, query := range decoded[0].([][]byte) {
		values, err := queryArgs.Unpack(query)
		if err != nil {
			return mintMarket{}, err
		}
		data := values[1].([]byte)
		if values[0].(common.Address) != market.Policy || len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
			continue
		}
		pair, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return mintMarket{}, err
		}
		market.Asset, market.UnderlyingAsset = pair[0].(common.Address), pair[1].(common.Address)
		return market, nil
	}
	for _, step := range inner.Steps {
		if step[0] != recipes.Call {
			continue
		}
		values, err := callArgs.Unpack(step[1:])
		if err != nil {
			return mintMarket{}, err
		}
		data := values[1].([]byte)
		if len(data) < 4 || !bytes.Equal(data[:4], tokenABI.GetABI().Methods["approve"].ID) {
			continue
		}
		bindings := values[2].([]*big.Int)
		spender, spenderErr := callInput(bindings, 4)
		amount, amountErr := callInput(bindings, 36)
		if spenderErr != nil || amountErr != nil || spender != market.RequestInput || uint64(amount) != market.Principal.Value.Uint64() {
			continue
		}
		asset := values[0].(common.Address)
		if market.Asset != (common.Address{}) && market.Asset != asset {
			return mintMarket{}, errors.New("3f mint has ambiguous funding tokens")
		}
		market.Asset = asset
	}
	if market.Asset == (common.Address{}) {
		return mintMarket{}, errors.New("3f mint funding token is missing")
	}
	return market, nil
}

// IRecipes binding words contain a uint32 offset, uint8 source and uint216 value.
func callInput(bindings []*big.Int, offset uint32) (uint, error) {
	for _, word := range bindings {
		var encoded [32]byte
		word.FillBytes(encoded[:])
		if uint32(new(big.Int).SetBytes(encoded[:4]).Uint64()) != offset {
			continue
		}
		index := new(big.Int).SetBytes(encoded[5:])
		if encoded[4] != recipes.Input || !index.IsUint64() {
			break
		}
		return uint(index.Uint64()), nil
	}
	return 0, errors.New("3f mint field requires a runtime input")
}
