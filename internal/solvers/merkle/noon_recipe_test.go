package merkle

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	executorbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func TestFactoryPlansCuratorNoonRedemption(t *testing.T) {
	raw, err := os.ReadFile("testdata/curator-noon-redemption.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Account    common.Address `json:"account"`
		Adapter    common.Address `json:"adapter"`
		Recipe     hexutil.Bytes  `json:"recipe"`
		RecipeHash common.Hash    `json:"recipeHash"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	recipe, err := recipes.DecodeRecipe(fixture.Recipe)
	if err != nil {
		t.Fatal(err)
	}
	if hash, err := recipes.HashRecipe(recipe); err != nil || hash != fixture.RecipeHash {
		t.Fatal("frontend recipe commitment changed", err)
	}
	tree, err := recipes.DecodeTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, simulation string
		balance          uint64
		idle, failed     bool
	}{
		{name: "empty account", idle: true},
		{name: "received USN", balance: 1},
		{name: "issuer rejects execution", balance: 1, simulation: "revert", failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := newRecipeHost(t, tc.balance, 0, tc.simulation, &recipeHostFixture{
				recipe: recipe, tree: tree, account: fixture.Account, adapter: fixture.Adapter,
				results: func(balance uint64) []*big.Int {
					values := make([]*big.Int, len(recipe.Steps))
					for i := range values {
						values[i] = new(big.Int)
					}
					values[0].SetUint64(balance)
					return values
				},
			})
			plan, err := host.solver.keeper.Planner.PrepareExecution(t.Context(), host.solver.keeper.Strategies)
			if tc.idle {
				if !errors.Is(err, provider.ErrNotReady) {
					t.Fatalf("empty recipe must remain idle: %v", err)
				}
				return
			}
			if tc.failed {
				if err == nil || errors.Is(err, provider.ErrNotReady) {
					t.Fatalf("issuer failure must block execution: %v", err)
				}
				return
			}
			if err != nil || len(plan.Members) != 1 {
				t.Fatal("standalone recipe was not planned", err)
			}
			member := plan.Members[0].Plan
			if member.Recipe.RecipeHash != fixture.RecipeHash || member.Snapshot.Account != fixture.Account || len(member.Leaves) != 2 {
				t.Fatal("host changed recipe identity or required permissions")
			}
			calls, err := recipes.Executions(member.Calldata)
			if err != nil || len(calls) != 1 {
				t.Fatal("balance-consuming redemption must execute once", err)
			}
			args, err := executorbinding.NewRecipeExecutor().GetABI().Methods["execute"].Inputs.Unpack(calls[0][4:])
			if err != nil || len(args[2].([]*big.Int)) != 0 {
				t.Fatal("native redemption unexpectedly requires keeper inputs", err)
			}
		})
	}
}
