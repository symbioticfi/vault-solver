package rfq

import (
	"bytes"
	"context"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	llconnector "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// This fixture leaves discovery, permission verification and calldata assembly in the SDK;
// only the chain and curator transports are replaced.
func TestConnectorBuildSimulatesCompleteAwardedOrder(t *testing.T) {
	resolved := connectortest.DiscountFixture()
	resolved.SignerSignature = "0x" + strings.Repeat("ab", 130)
	resolved.ProtocolSignature = "0x" + strings.Repeat("cd", 1)
	fixture := connectorExecutionFixture(t, resolved)
	w, chain, recipe, leaf := fixture.workflow, fixture.reader, fixture.recipe, fixture.leaf
	if len(recipe.Queries) != 3 {
		t.Fatal("recipe contains queries beyond the original balance checks")
	}
	order, selected, backend := connectorExecutionAward(w.Filler)
	backend.discount = resolved
	built, err := buildFill(t.Context(), w, order, []byte{0x91, 0x92}, []filler.Leg{{Source: selected}}, backend)
	if err != nil {
		t.Fatal(err)
	}
	calldata, until := built.Calldata, built.ValidUntil
	if len(built.Legs) != 1 || built.Legs[0].State.Account != chain.RecipeAccount || built.Legs[0].State.Asset != tOut || chain.IdentityReads != 2 {
		t.Fatal("build did not return its single pinned connector identity")
	}
	if !until.Equal(time.Unix(4102444800, 0)) {
		t.Fatalf("discount expiry = %v", until)
	}
	if chain.OuterCalls != 1 || !bytes.Equal(calldata, chain.OuterData) {
		t.Fatalf("returned fill was not the simulated fill: calls=%d", chain.OuterCalls)
	}
	if len(calldata) < 4 || !bytes.Equal(calldata[:4], executorBinding.GetABI().Methods["fill"].ID) {
		t.Fatalf("incorrect fill selector: %x", calldata)
	}
	values, err := executorBinding.GetABI().Methods["fill"].Inputs.Unpack(calldata[4:])
	if err != nil {
		t.Fatal(err)
	}
	gotOrder := *abi.ConvertType(values[0], new(executor.IReactorOrder)).(*executor.IReactorOrder)
	if !reflect.DeepEqual(gotOrder, order) || !bytes.Equal(values[1].([]byte), []byte{0x91, 0x92}) {
		t.Fatal("awarded order or protocol signature changed")
	}
	if reflect.ValueOf(values[2]).Len() != 0 {
		t.Fatal("discounted connector was funded as permissioned")
	}
	funding := *abi.ConvertType(values[3], new([]executor.IReactorDiscountSwapInput)).(*[]executor.IReactorDiscountSwapInput)
	if len(funding) != 1 || funding[0].Adapter != vlt || funding[0].AmountIn.Int64() != 123 || funding[0].Recipient != w.Filler || !bytes.Equal(funding[0].DiscountSwap.SignerSignature, bytes.Repeat([]byte{0xab}, 130)) || !bytes.Equal(funding[0].ProtocolSignature, []byte{0xcd}) {
		t.Fatalf("connector funding changed: %+v", funding)
	}
	callValues, err := executorCallsArgs.Unpack(values[4].([]byte))
	if err != nil {
		t.Fatal(err)
	}
	calls := *abi.ConvertType(callValues[0], new([]executorCall)).(*[]executorCall)
	if len(calls) != 1 || calls[0].Target != chain.RecipeExecutor || calls[0].Value.Sign() != 0 {
		t.Fatalf("recipe call changed: %+v", calls)
	}
	route, err := recipeBinding.GetABI().Methods["execute"].Inputs.Unpack(calls[0].Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	inputs := route[2].([]*big.Int)
	runtime := route[3].([][]byte)
	if len(inputs) != 2 || inputs[0].Int64() != 123 || inputs[1].Int64() != 456 || !reflect.DeepEqual(route[0], recipe.Queries) || !reflect.DeepEqual(route[1], recipe.Steps) || len(runtime) != 2 {
		t.Fatal("awarded inputs or committed recipe changed")
	}
	actionsType := merkleaccount.NewMerkleAccount().GetABI().Methods["manage"].Inputs
	actions, err := actionsType.Unpack(runtime[1])
	if err != nil {
		t.Fatal(err)
	}
	managed := *abi.ConvertType(actions[0], new([]merkleaccount.IMerkleAccountAction)).(*[]merkleaccount.IMerkleAccountAction)
	if len(managed) != 1 || managed[0].Target != vlt || managed[0].Value.Sign() != 0 || !bytes.Equal(managed[0].DataMask, leaf.DataMask) || len(managed[0].Proof) != 1 {
		t.Fatalf("curator permission changed: %+v", managed)
	}
	swapValues, err := connectorBinding.GetABI().Methods["swap"].Inputs.Unpack(managed[0].Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if swapValues[2].(common.Address) != w.Filler || swapValues[3].(*big.Int).Int64() != 123 {
		t.Fatal("connector payment recipient or amount differs from the awarded order")
	}
	if !bytes.Equal(managed[0].Data[132:], leaf.Data[132:]) {
		t.Fatal("backend signed terms or signatures changed")
	}
}

func TestConnectorBuildRejectsIncompatibleSignaturePermission(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
	order, selected, backend := connectorExecutionAward(fixture.workflow.Filler)
	backend.discount.SignerSignature = "0x" + strings.Repeat("ab", 130)
	built, err := buildFill(t.Context(), fixture.workflow, order, []byte{0x91, 0x92}, []filler.Leg{{Source: selected}}, backend)
	calldata := built.Calldata
	if err == nil || !strings.Contains(err.Error(), "changes calldata length") {
		t.Fatalf("error = %v, want committed calldata length rejection", err)
	}
	if len(calldata) != 0 || fixture.reader.OuterCalls != 0 {
		t.Fatal("incompatible permission reached simulation or returned transaction calldata")
	}
}

func TestConnectorBuildRejectsUnauthorizedOrUnsettledExecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*connector.Workflow, *connectortest.Chain)
		want   string
	}{

		{"host signer missing", func(w *connector.Workflow, _ *connectortest.Chain) { w.Signer = common.Address{} }, "host signer"},
		{"connector account differs", func(_ *connector.Workflow, r *connectortest.Chain) { r.ConnectorAccount = common.HexToAddress("0x88") }, "account"},
		{"recipe caller differs", func(_ *connector.Workflow, r *connectortest.Chain) { r.RecipeCaller = common.HexToAddress("0x88") }, "caller"},
		{"permission role missing", func(_ *connector.Workflow, r *connectortest.Chain) { r.Allowed = false }, "role"},
		{"outer fill reverts", func(_ *connector.Workflow, r *connectortest.Chain) {
			r.OuterErr = errors.New("Reactor output shortfall")
		}, "simulat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved := connectortest.DiscountFixture()
			resolved.SignerSignature = "0x" + strings.Repeat("ab", 130)
			fixture := connectorExecutionFixture(t, resolved)
			w, chain := fixture.workflow, fixture.reader
			tc.mutate(w, chain)
			order, selected, backend := connectorExecutionAward(w.Filler)
			backend.discount = resolved
			built, err := buildFill(t.Context(), w, order, []byte{0x91, 0x92}, []filler.Leg{{Source: selected}}, backend)
			calldata := built.Calldata
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if len(calldata) != 0 {
				t.Fatal("failed preparation returned transaction calldata")
			}
		})
	}
}

func TestConnectorBuildUsesHostAndBlockGasBudgets(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		maxGas, blockGas, wantGas uint64
	}{
		{name: "default", blockGas: 30_000_000, wantGas: 16_777_216},
		{name: "host budget", maxGas: 2_000_000, blockGas: 30_000_000, wantGas: 2_000_000},
		{name: "block limit", maxGas: 5_000_000, blockGas: 1_000_000, wantGas: 1_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
			fixture.workflow.MaxGas = tc.maxGas
			fixture.reader.Header.GasLimit = tc.blockGas
			order, selected, backend := connectorExecutionAward(fixture.workflow.Filler)
			if _, err := buildFill(t.Context(), fixture.workflow, order, []byte{0x91, 0x92}, []filler.Leg{{Source: selected}}, backend); err != nil {
				t.Fatal(err)
			}
			if fixture.reader.OuterCalls != 1 || fixture.reader.OuterGas != tc.wantGas {
				t.Fatalf("outer fill simulation: calls=%d gas=%d, want %d", fixture.reader.OuterCalls, fixture.reader.OuterGas, tc.wantGas)
			}
		})
	}
}

var connectorBinding = llconnector.NewLiquidLaneConnector()
var recipeBinding = recipeexecutor.NewRecipeExecutor()

type connectorExecutionFixtureData struct {
	workflow *connector.Workflow
	reader   *connectortest.Chain
	recipe   recipes.Recipe
	leaf     recipes.Leaf
}

func connectorExecutionFixture(t *testing.T, resolved *discounts.Resolved) connectorExecutionFixtureData {
	t.Helper()
	return connectorExecutionFixtureAt(t, resolved, common.HexToAddress("0x43"))
}

func connectorExecutionFixtureAt(t *testing.T, resolved *discounts.Resolved, recipeExecutor common.Address) connectorExecutionFixtureData {
	t.Helper()
	rfqExecutor := common.HexToAddress("0x10")
	terms, err := connector.ParseDiscount(resolved, *connectortest.DiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	f := connectortest.NewFixture(t, terms.Swap, terms.Signature, recipeExecutor, rfqExecutor, [4]byte(executorBinding.GetABI().Methods["fill"].ID))
	workflow := &connector.Workflow{Chain: f.Chain, ChainID: big.NewInt(1), Source: f.Permissions, Filler: rfqExecutor, Signer: f.Chain.Signer}
	return connectorExecutionFixtureData{workflow, f.Chain, f.Recipe, f.Leaf}
}

func connectorExecutionAward(caller common.Address) (executor.IReactorOrder, filler.Source, *fakeBackend) {
	outputs := []executor.IReactorOutput{{Token: tOut, Amount: big.NewInt(456), Recipient: common.HexToAddress("0x55")}}
	order := executor.IReactorOrder{
		Request:          executor.IReactorRequest{TokenIn: tIn, AmountIn: big.NewInt(123), Outputs: outputs, Deadline: big.NewInt(4102444800), Nonce: big.NewInt(23), Protocol: common.HexToAddress("0x45")},
		SwapperSignature: []byte{0x81, 0x82}, Swapper: common.HexToAddress("0x51"), Filler: caller, Outputs: outputs,
	}
	selected := filler.Source{Venue: vlt, RecipeExecutor: common.HexToAddress("0x43"), DiscountID: *connectortest.DiscountID(), MaxAssets: big.NewInt(900)}
	return order, selected, &fakeBackend{discount: connectortest.DiscountFixture()}
}

func TestConnectorBuildRequiresSelectedRoute(t *testing.T) {
	for _, field := range []string{"executor", "venue"} {
		t.Run(field, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
			order, selected, backend := connectorExecutionAward(fixture.workflow.Filler)
			if field == "executor" {
				selected.RecipeExecutor = common.Address{}
			} else {
				selected.Venue = common.Address{}
			}
			_, err := buildFill(t.Context(), fixture.workflow, order, nil, []filler.Leg{{Source: selected}}, backend)
			if err == nil || fixture.reader.OuterCalls != 0 {
				t.Fatalf("missing %s did not fail closed: %v", field, err)
			}
		})
	}
}

func TestConnectorBuildAcceptsHeadAdvanceBetweenDiscoveryStages(t *testing.T) {
	f := connectorExecutionFixture(t, connectortest.DiscountFixture())
	reader := &headAdvanceReader{Chain: f.reader}
	f.workflow.Chain = reader
	f.workflow.Source = advancedPermissions{f.workflow.Source}
	order, selected, backend := connectorExecutionAward(f.workflow.Filler)
	if built, err := buildFill(t.Context(), f.workflow, order, nil, []filler.Leg{{Source: selected}}, backend); err != nil || len(built.Calldata) == 0 {
		t.Fatalf("ordinary head advance rejected: %v", err)
	}
	if reader.connectorReads != 3 || reader.simulationBlock != 8 || f.reader.OuterCalls != 1 {
		t.Fatalf("connector or settlement escaped prepared block: reads=%d simulation=%d", reader.connectorReads, reader.simulationBlock)
	}
}

// Discovery starts at head 7, then head 8 becomes available. The recipe event
// remains in canonical block 7, while the prepared connector state is block 8.
type headAdvanceReader struct {
	*connectortest.Chain

	latestReads     int
	connectorReads  int
	simulationBlock uint64
}

func (r *headAdvanceReader) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	header := types.CopyHeader(r.Header)
	if number == nil {
		r.latestReads++
		if r.latestReads > 1 {
			header.Number = big.NewInt(8)
		}
	} else {
		header.Number = new(big.Int).Set(number)
	}
	return header, nil
}

func (r *headAdvanceReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To != nil && *msg.To == vlt {
		if block == nil || block.Uint64() != 8 {
			return nil, errors.New("connector read escaped prepared block")
		}
		r.connectorReads++
	}
	if msg.To != nil && *msg.To == r.RecipeCaller {
		r.simulationBlock = block.Uint64()
	}
	return r.Chain.CallContract(ctx, msg, big.NewInt(7))
}

type advancedPermissions struct{ connector.PermissionSource }

func (s advancedPermissions) PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	if snapshot.BlockNumber != 8 {
		return nil, errors.New("permissions did not use the prepared head")
	}
	snapshot.BlockNumber = 7 // The shared fixture asserts its original transport height.
	return s.PermissionSource.PermissionTree(ctx, chainID, snapshot)
}
