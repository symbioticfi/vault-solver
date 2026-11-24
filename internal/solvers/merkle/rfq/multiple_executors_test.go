package rfq

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// Both executors spend the same account and funding asset, with identical swap
// permission but different recipe commitments/call targets.
func TestMultipleExecutorsShareRouting(t *testing.T) {
	first := connectorExecutionFixture(t, connectortest.DiscountFixture())
	second := connectorExecutionFixtureAt(t, connectortest.DiscountFixture(), common.HexToAddress("0x44"))
	reader := &multiRecipeReader{Chain: first.reader, readers: map[common.Address]*connectortest.Chain{
		first.reader.RecipeExecutor: first.reader, second.reader.RecipeExecutor: second.reader,
	}}
	workflow := *first.workflow
	workflow.Chain = reader
	workflow.Source = multiRecipeSource{first.reader.RecipeExecutor: first.workflow.Source, second.reader.RecipeExecutor: second.workflow.Source}
	_, _, backend := connectorExecutionAward(workflow.Filler)
	s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(&workflow, []common.Address{first.reader.RecipeExecutor, second.reader.RecipeExecutor}), Workflow: &workflow}, backend: backend}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatalf("routes: %v", err)
	}
	if s.Routes.Len() != 2 {
		t.Fatal("did not initialize both configured executors")
	}
	order, _, _ := connectorExecutionAward(workflow.Filler)
	listing := discounts.ListItem{DiscountPrecision: "100000000", Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900"}
	wrongRWA := listing
	wrongRWA.TokenToRedeem = common.HexToAddress("0xbad").Hex()
	intent, err := s.prepare(t.Context(), awardRow(t, order), []discounts.ListItem{wrongRWA, listing})
	if err != nil || intent != nil {
		t.Fatalf("ambiguous local recipes reached fill: %v", err)
	}
}

type multiRecipeReader struct {
	*connectortest.Chain

	readers map[common.Address]*connectortest.Chain
}

func (r *multiRecipeReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return r.readers[query.Addresses[0]].FilterLogs(ctx, query)
}
func (r *multiRecipeReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To != nil && *msg.To == r.RecipeCaller && bytes.HasPrefix(msg.Data, executorBinding.GetABI().Methods["fill"].ID) {
		values, err := executorBinding.GetABI().Methods["fill"].Inputs.Unpack(msg.Data[4:])
		if err != nil {
			return nil, err
		}
		encoded, err := executorCallsArgs.Unpack(values[4].([]byte))
		if err != nil {
			return nil, err
		}
		calls := *abi.ConvertType(encoded[0], new([]executorCall)).(*[]executorCall)
		return r.readers[calls[0].Target].CallContract(ctx, msg, block)
	}
	if reader := r.readers[*msg.To]; reader != nil {
		return reader.CallContract(ctx, msg, block)
	}
	// hasRole contains the executor in its second argument.
	if *msg.To == r.RecipeAccount && len(msg.Data) == 68 {
		if reader := r.readers[common.BytesToAddress(msg.Data[48:68])]; reader != nil {
			return reader.CallContract(ctx, msg, block)
		}
	}
	return r.Chain.CallContract(ctx, msg, block)
}

type multiRecipeSource map[common.Address]connector.PermissionSource

func (s multiRecipeSource) PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	return s[snapshot.Executor].PermissionTree(ctx, chainID, snapshot)
}
