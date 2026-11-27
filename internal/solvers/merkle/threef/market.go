package threef

import (
	"context"
	"fmt"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	vschain "github.com/symbioticfi/vault-solver/internal/chain"
)

// reverts names reverts from the Merkle account and executor; the vendored policy and capability
// ABIs declare no custom errors.
var reverts = vschain.RevertDecoderFor(&accountbinding.MerkleAccountMetaData, &recipeexecutor.RecipeExecutorMetaData)

// readFailure names a failed read. A revert is decoded and cancellation is kept so shutdown is not
// reported as a failure; any other error is dropped because RPC transport errors can carry the
// endpoint URL and its key.
func readFailure(what string, err error) error {
	if data, reverted := vschain.RevertData(err); reverted {
		return errors.Errorf("3f %s reverted: %s", what, reverts.Describe(data))
	}
	if errors.Is(err, context.Canceled) {
		return errors.Errorf("3f %s failed: %w", what, context.Canceled)
	}
	return errors.Errorf("3f %s failed", what)
}

// observation contains quantities read at the recipe's canonical block.
type observation struct {
	Market                                            mintMarket
	Budget, MinimumYield, AvailableCash, MaxPrincipal *big.Int
}

func observe(ctx context.Context, chain provider.Reader, snapshot provider.Snapshot, recipe recipes.Recipe, market mintMarket) (observation, error) {
	observed, err := observePolicy(ctx, chain, snapshot, market)
	if err != nil {
		return observation{}, err
	}
	// Execution additionally obeys the committed recipe's principal bounds.
	for _, step := range recipe.Steps {
		if len(step) != 65 || step[0] != recipes.LessOrEqual || market.Principal.Source != recipes.Input || step[5] != recipes.Input || new(big.Int).SetBytes(step[6:33]).Cmp(market.Principal.Value) != 0 || step[37] != recipes.Literal {
			continue
		}
		value := new(big.Int).SetBytes(step[38:65])
		if value.Cmp(observed.MaxPrincipal) < 0 {
			observed.MaxPrincipal = value
		}
	}
	return observed, nil
}

func observePolicy(ctx context.Context, chain provider.Reader, snapshot provider.Snapshot, market mintMarket) (observation, error) {
	call := func(what string, to common.Address, data []byte) ([]byte, error) {
		raw, err := chain.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, snapshot.BlockNumber)
		if err != nil {
			return nil, readFailure(what+" read", err)
		}
		if len(raw) != 32 {
			return nil, errors.Errorf("3f %s read returned %d bytes, want a 32-byte scalar", what, len(raw))
		}
		return raw, nil
	}
	raw, err := call("policy budget", market.Policy, policyABI.PackRemainingAssets(market.Asset, market.UnderlyingAsset))
	if err != nil {
		return observation{}, err
	}
	budget, err := policyABI.UnpackRemainingAssets(raw)
	if err != nil {
		return observation{}, err
	}
	raw, err = call("minimum yield", market.Policy, policyABI.PackMinYieldPerRequest(market.UnderlyingAsset))
	if err != nil {
		return observation{}, err
	}
	minimum, err := policyABI.UnpackMinYieldPerRequest(raw)
	if err != nil {
		return observation{}, err
	}
	raw, err = call("funding balance", market.Asset, tokenABI.PackBalanceOf(snapshot.Account))
	if err != nil {
		return observation{}, err
	}
	cash, err := tokenABI.UnpackBalanceOf(raw)
	if err != nil {
		return observation{}, err
	}
	ceiling := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	return observation{Market: market, Budget: budget, MinimumYield: minimum, AvailableCash: cash, MaxPrincipal: ceiling}, nil
}

var errIneligible = errors.New("3f request is not eligible for the pinned funding pair")

// eligible performs a read-only policy charge and verifies the exact pinned pair's
// budget delta. The policy resolves factory membership, Facility binding and
// collateral underlying itself. No simulated authorization or balance is invented.
func eligible(ctx context.Context, chain provider.Reader, snapshot provider.Snapshot, market mintMarket, request common.Address, minimum *big.Int) error {
	principal := big.NewInt(1)
	yield := minimumReturn(principal, minimum)
	query, err := queryArgs.Pack(market.Policy, policyABI.PackRemainingAssets(market.Asset, market.UnderlyingAsset), []*big.Int{}, new(big.Int))
	if err != nil {
		return err
	}
	consume, err := callArgs.Pack(market.Policy, policyABI.PackConsume(request, principal, yield), []*big.Int{})
	if err != nil {
		return err
	}
	unary := abiRead(0)
	steps := [][]byte{unary, append([]byte{recipes.Call}, consume...), unary, abiBinary(recipes.Sub, encodedValue(recipes.Result, 0), encodedValue(recipes.Result, 2)), abiBinary(recipes.Equal, encodedValue(recipes.Result, 3), principal)}
	data, err := capability.TryPackExecute([][]byte{query}, steps, []*big.Int{})
	if err != nil {
		return err
	}
	raw, err := chain.CallContract(ctx, ethereum.CallMsg{From: snapshot.Account, To: &snapshot.Account, Data: data}, snapshot.BlockNumber)
	if err != nil {
		// Only a revert carrying data is the policy's verdict; anything else fails the pass.
		var reverted rpc.DataError
		if errors.As(err, &reverted) && reverted.ErrorData() != nil {
			detail := fmt.Sprintf("undecodable revert data %v", reverted.ErrorData())
			if payload, _ := vschain.RevertData(err); payload != nil {
				detail = reverts.Describe(payload)
			}
			return errors.Errorf("%w: policy consume for request %s reverted: %s", errIneligible, request.Hex(), detail)
		}
		return readFailure("eligibility read for request "+request.Hex(), err)
	}
	values, err := capability.UnpackExecute(raw)
	switch {
	case err != nil:
		return errors.Errorf("3f eligibility simulation for request %s: decode: %w", request.Hex(), err)
	case len(values) != 5:
		return errors.Errorf("3f eligibility simulation for request %s returned %d results, want 5", request.Hex(), len(values))
	case values[3].Cmp(principal) != 0 || values[4].Sign() != 0:
		return errors.Errorf("3f eligibility simulation for request %s: budget delta %s, want %s; equality check %s, want 0",
			request.Hex(), values[3], principal, values[4])
	}
	return nil
}

func encodedValue(source uint8, value uint64) *big.Int {
	word := new(big.Int).Lsh(new(big.Int).SetUint64(uint64(source)), 216)
	return word.Or(word, new(big.Int).SetUint64(value))
}
func abiRead(query uint64) []byte {
	return append([]byte{recipes.Read}, common.LeftPadBytes(new(big.Int).SetUint64(query).Bytes(), 32)...)
}
func abiBinary(op uint8, left, right *big.Int) []byte {
	return append(append([]byte{op}, common.LeftPadBytes(left.Bytes(), 32)...), common.LeftPadBytes(right.Bytes(), 32)...)
}
