package threef

import (
	"bytes"
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

// observation contains quantities read at the recipe's canonical block.
type observation struct {
	Market                                            mintMarket
	Budget, MinimumYield, AvailableCash, MaxPrincipal *big.Int
}

func observe(ctx context.Context, chain provider.Reader, snapshot provider.Snapshot, recipe recipes.Recipe, market mintMarket) (observation, error) {
	call := func(to common.Address, data []byte) ([]byte, error) {
		raw, err := chain.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, snapshot.BlockNumber)
		if err != nil {
			return nil, err
		}
		if len(raw) != 32 {
			return nil, errors.New("3f invalid scalar reply")
		}
		return raw, nil
	}
	raw, err := call(market.Policy, policyABI.PackAccount())
	if err != nil {
		return observation{}, errors.New("3f policy account read failed")
	}
	owner, err := policyABI.UnpackAccount(raw)
	if err != nil || !bytes.Equal(raw[:12], make([]byte, 12)) || owner != snapshot.Account {
		return observation{}, errors.New("3f policy belongs to another account")
	}
	raw, err = call(market.Policy, policyABI.PackRemainingAssets(market.Asset, market.UnderlyingAsset))
	if err != nil {
		return observation{}, errors.New("3f policy budget read failed")
	}
	budget, err := policyABI.UnpackRemainingAssets(raw)
	if err != nil {
		return observation{}, err
	}
	raw, err = call(market.Policy, policyABI.PackMinYieldPerRequest(market.UnderlyingAsset))
	if err != nil {
		return observation{}, errors.New("3f minimum yield read failed")
	}
	minimum, err := policyABI.UnpackMinYieldPerRequest(raw)
	if err != nil {
		return observation{}, err
	}
	raw, err = call(market.Asset, tokenABI.PackBalanceOf(snapshot.Account))
	if err != nil {
		return observation{}, errors.New("3f funding balance read failed")
	}
	cash, err := tokenABI.UnpackBalanceOf(raw)
	if err != nil {
		return observation{}, err
	}
	ceiling := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	// Bounds constrain offer capacity; the accepted authorization still selects mint amounts.
	for _, step := range recipe.Steps {
		if len(step) != 65 || step[0] != recipes.LessOrEqual || step[5] != recipes.Input || new(big.Int).SetBytes(step[6:33]).Uint64() != uint64(market.PrincipalInput) || step[37] != recipes.Literal {
			continue
		}
		value := new(big.Int).SetBytes(step[38:65])
		if value.Cmp(ceiling) < 0 {
			ceiling = value
		}
	}
	return observation{Market: market, Budget: budget, MinimumYield: minimum, AvailableCash: cash, MaxPrincipal: ceiling}, nil
}

var errIneligible = errors.New("3f request is not eligible for the pinned funding pair")

// eligible performs a read-only policy charge and verifies the exact pinned pair's
// budget delta. The policy resolves factory membership, Facility binding and
// collateral underlying itself. No simulated authorization or balance is invented.
func eligible(ctx context.Context, chain provider.Reader, snapshot provider.Snapshot, market mintMarket, request common.Address, minimum *big.Int) error {
	principal := big.NewInt(1)
	yield := new(big.Int).Quo(new(big.Int).Add(minimum, big.NewInt(99_999_999)), big.NewInt(100_000_000))
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
		var reverted rpc.DataError
		if errors.As(err, &reverted) && reverted.ErrorData() != nil {
			return errIneligible
		}
		return errors.New("3f eligibility read failed")
	}
	values, err := capability.UnpackExecute(raw)
	if err != nil || len(values) != 5 || values[3].Cmp(principal) != 0 || values[4].Sign() != 0 {
		return errors.New("3f invalid eligibility simulation")
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
