package rfq

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// Both executors spend the same account and funding asset, with identical swap
// permission but different recipe commitments/call targets.
func TestMultipleExecutorsShareRouting(t *testing.T) {
	first := connectorExecutionFixture(t, connectorDiscountFixture())
	second := connectorExecutionFixtureAt(t, connectorDiscountFixture(), common.HexToAddress("0x44"))
	reader := &multiRecipeReader{connectorExecutionReader: first.reader, readers: map[common.Address]*connectorExecutionReader{
		first.reader.recipeExecutor: first.reader, second.reader.recipeExecutor: second.reader,
	}}
	workflow := *first.workflow
	workflow.chain = reader
	workflow.source = multiRecipeSource{first.reader.recipeExecutor: first.workflow.source, second.reader.recipeExecutor: second.workflow.source}
	_, _, backend := connectorExecutionAward(workflow.caller)
	s := &Solver{recipeExecutors: []common.Address{first.reader.recipeExecutor, second.reader.recipeExecutor}, workflow: &workflow, backend: backend}
	venues, err := s.Initialize(t.Context())
	if err != nil || len(venues) != 1 || venues[0] != vlt {
		t.Fatalf("routes: %v %v", venues, err)
	}
	for _, executor := range s.recipeExecutors {
		if !s.AcceptsRoute(executor, vlt, first.reader.recipeAccount, tOut) {
			t.Fatalf("missing executor %s", executor)
		}
	}
	if s.AcceptsRoute(common.HexToAddress("0xbad"), vlt, first.reader.recipeAccount, tOut) || s.AcceptsRoute(second.reader.recipeExecutor, vlt, common.HexToAddress("0xbad"), tOut) {
		t.Fatal("accepted an unconfigured route")
	}
	order, _, _ := connectorExecutionAward(workflow.caller)
	listing := discounts.ListItem{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: connectorDiscountID().Hex(), MaxAssets: "900", RecipeExecutor: second.reader.recipeExecutor.Hex()}
	wrongRWA := listing
	wrongRWA.TokenToRedeem = common.HexToAddress("0xbad").Hex()
	wrongRWA.RecipeExecutor = first.reader.recipeExecutor.Hex()
	intent, err := s.prepare(t.Context(), awardRow(t, order), []discounts.ListItem{wrongRWA, listing})
	if err != nil || intent == nil {
		t.Fatalf("prepare second executor: %v", err)
	}
	values, err := executorBinding.GetABI().Methods["fill"].Inputs.Unpack(intent.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := executorCallsArgs.Unpack(values[4].([]byte))
	if err != nil {
		t.Fatal(err)
	}
	calls := *abi.ConvertType(encoded[0], new([]executorCall)).(*[]executorCall)
	if len(calls) != 1 || calls[0].Target != second.reader.recipeExecutor {
		t.Fatal("used the wrong executor")
	}
	listing.RecipeExecutor = common.HexToAddress("0xbad").Hex()
	if unconfigured, err := s.prepare(t.Context(), awardRow(t, order), []discounts.ListItem{listing}); err != nil || unconfigured != nil {
		t.Fatalf("used unconfigured executor: %v", err)
	}
}

type multiRecipeReader struct {
	*connectorExecutionReader

	readers map[common.Address]*connectorExecutionReader
}

func (r *multiRecipeReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return r.readers[query.Addresses[0]].FilterLogs(ctx, query)
}
func (r *multiRecipeReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To != nil && *msg.To == r.recipeCaller && bytes.HasPrefix(msg.Data, executorBinding.GetABI().Methods["fill"].ID) {
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
	if *msg.To == r.recipeAccount && len(msg.Data) == 68 {
		if reader := r.readers[common.BytesToAddress(msg.Data[48:68])]; reader != nil {
			return reader.CallContract(ctx, msg, block)
		}
	}
	return r.connectorExecutionReader.CallContract(ctx, msg, block)
}

func (r *multiRecipeReader) CallContractWithCodeAt(ctx context.Context, msg ethereum.CallMsg, block *big.Int, target common.Address, code []byte) ([]byte, error) {
	return r.readers[target].CallContractWithCodeAt(ctx, msg, block, target, code)
}

type multiRecipeSource map[common.Address]permissionSource

func (s multiRecipeSource) PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	return s[snapshot.Executor].PermissionTree(ctx, chainID, snapshot)
}
