package rfq

import (
	"bytes"
	"context"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// This fixture leaves discovery, permission verification and calldata assembly in the SDK;
// only the chain and curator transports are replaced.
func TestConnectorBuildSimulatesCompleteAwardedOrder(t *testing.T) {
	resolved := connectorDiscountFixture()
	resolved.SignerSignature = "0x" + strings.Repeat("ab", 130)
	resolved.ProtocolSignature = "0x" + strings.Repeat("cd", 1)
	fixture := connectorExecutionFixture(t, resolved)
	w, chain, recipe, leaf := fixture.workflow, fixture.reader, fixture.recipe, fixture.leaf
	if len(recipe.Queries) != 3 {
		t.Fatal("recipe contains queries beyond the original balance checks")
	}
	order, selected, backend := connectorExecutionAward(w.caller)
	backend.discount = resolved
	built, err := w.build(t.Context(), order, []byte{0x91, 0x92}, selected, backend)
	if err != nil {
		t.Fatal(err)
	}
	calldata, until := built.Calldata, built.ValidUntil
	if built.State.Account != chain.recipeAccount || built.State.Asset != tOut || chain.identityReads != 2 {
		t.Fatal("build did not return its single pinned connector identity")
	}
	if !until.Equal(time.Unix(4102444800, 0)) {
		t.Fatalf("discount expiry = %v", until)
	}
	if chain.outerCalls != 1 || !bytes.Equal(calldata, chain.outerData) {
		t.Fatalf("returned fill was not the simulated fill: calls=%d", chain.outerCalls)
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
	if len(funding) != 1 || funding[0].Adapter != vlt || funding[0].AmountIn.Int64() != 123 || funding[0].Recipient != w.caller || !bytes.Equal(funding[0].DiscountSwap.SignerSignature, bytes.Repeat([]byte{0xab}, 130)) || !bytes.Equal(funding[0].ProtocolSignature, []byte{0xcd}) {
		t.Fatalf("connector funding changed: %+v", funding)
	}
	callValues, err := executorCallsArgs.Unpack(values[4].([]byte))
	if err != nil {
		t.Fatal(err)
	}
	calls := *abi.ConvertType(callValues[0], new([]executorCall)).(*[]executorCall)
	if len(calls) != 1 || calls[0].Target != chain.recipeExecutor || calls[0].Value.Sign() != 0 {
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
	if swapValues[2].(common.Address) != w.caller || swapValues[3].(*big.Int).Int64() != 123 {
		t.Fatal("connector payment recipient or amount differs from the awarded order")
	}
	if !bytes.Equal(managed[0].Data[132:], leaf.Data[132:]) {
		t.Fatal("backend signed terms or signatures changed")
	}
}

func TestConnectorBuildRejectsIncompatibleSignaturePermission(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectorDiscountFixture())
	order, selected, backend := connectorExecutionAward(fixture.workflow.caller)
	backend.discount.SignerSignature = "0x" + strings.Repeat("ab", 130)
	built, err := fixture.workflow.build(t.Context(), order, []byte{0x91, 0x92}, selected, backend)
	calldata := built.Calldata
	if err == nil || !strings.Contains(err.Error(), "changes calldata length") {
		t.Fatalf("error = %v, want committed calldata length rejection", err)
	}
	if len(calldata) != 0 || fixture.reader.outerCalls != 0 {
		t.Fatal("incompatible permission reached simulation or returned transaction calldata")
	}
}

func TestConnectorBuildRejectsUnauthorizedOrUnsettledExecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*connectorWorkflow, *connectorExecutionReader)
		want   string
	}{

		{"host signer missing", func(w *connectorWorkflow, _ *connectorExecutionReader) { w.signer = common.Address{} }, "host signer"},
		{"connector account differs", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.account = common.HexToAddress("0x88") }, "account"},
		{"recipe caller differs", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.recipeCaller = common.HexToAddress("0x88") }, "caller"},
		{"permission role missing", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.allowed = false }, "role"},
		{"outer fill reverts", func(_ *connectorWorkflow, r *connectorExecutionReader) {
			r.outerErr = errors.New("Reactor output shortfall")
		}, "simulat"},
		{"binding snapshot reorganizes after discovery", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.reorgBeforeDiscovery = true }, "block hash"},
		{"snapshot changes during outer fill", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.reorgDuringFill = true }, "reorganiz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved := connectorDiscountFixture()
			resolved.SignerSignature = "0x" + strings.Repeat("ab", 130)
			fixture := connectorExecutionFixture(t, resolved)
			w, chain := fixture.workflow, fixture.reader
			tc.mutate(w, chain)
			order, selected, backend := connectorExecutionAward(w.caller)
			backend.discount = resolved
			built, err := w.build(t.Context(), order, []byte{0x91, 0x92}, selected, backend)
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
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			fixture.workflow.maxGas = tc.maxGas
			fixture.reader.header.GasLimit = tc.blockGas
			order, selected, backend := connectorExecutionAward(fixture.workflow.caller)
			if _, err := fixture.workflow.build(t.Context(), order, []byte{0x91, 0x92}, selected, backend); err != nil {
				t.Fatal(err)
			}
			if fixture.reader.outerCalls != 1 || fixture.reader.outerGas != tc.wantGas {
				t.Fatalf("outer fill simulation: calls=%d gas=%d, want %d", fixture.reader.outerCalls, fixture.reader.outerGas, tc.wantGas)
			}
		})
	}
}

type connectorExecutionPermissions struct {
	raw      []byte
	account  common.Address
	executor common.Address
}

func (s connectorExecutionPermissions) PermissionTree(_ context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	if chainID.Cmp(big.NewInt(1)) != 0 || snapshot.Account != s.account || snapshot.Executor != s.executor || snapshot.BlockNumber != 7 {
		return nil, errors.New("permission request escaped authenticated route")
	}
	return bytes.Clone(s.raw), nil
}

type connectorExecutionReader struct {
	*connectorReadFake

	recipeExecutor, recipeAccount, recipeCaller, signer, rfqCaller common.Address
	recipeHash, root, approvalHash, swapHash                       common.Hash
	recipe                                                         recipes.Recipe
	header                                                         *types.Header
	allowed                                                        bool
	outerCalls                                                     int
	outerGas                                                       uint64
	identityReads                                                  int
	outerData                                                      []byte
	outerErr                                                       error
	reorgDuringFill                                                bool
	reorgBeforeDiscovery                                           bool
	headerDelay                                                    time.Duration
	headerStarted                                                  time.Time
}

func (r *connectorExecutionReader) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if r.headerStarted.IsZero() {
		r.headerStarted = time.Now()
	}
	if r.headerDelay > 0 {
		timer := time.NewTimer(r.headerDelay)
		r.headerDelay = 0
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if number != nil && number.Uint64() != 7 {
		return nil, errors.New("unexpected unpinned header")
	}
	return types.CopyHeader(r.header), nil
}

func (r *connectorExecutionReader) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	event := recipeBinding.GetABI().Events["SetRecipe"]
	if query.FromBlock == nil || query.ToBlock == nil || query.FromBlock.Uint64() != 7 || query.ToBlock.Uint64() != 7 || len(query.Addresses) != 1 || query.Addresses[0] != r.recipeExecutor || len(query.Topics) != 1 || len(query.Topics[0]) != 1 || query.Topics[0][0] != event.ID {
		return nil, errors.New("SetRecipe evidence escaped pinned executor")
	}
	data, err := event.Inputs.NonIndexed().Pack(r.recipe.Queries, r.recipe.Steps)
	if err != nil {
		return nil, err
	}
	return []types.Log{{Address: r.recipeExecutor, Topics: []common.Hash{event.ID, r.recipeHash}, Data: data, BlockNumber: 7, BlockHash: r.header.Hash()}}, nil
}

func (r *connectorExecutionReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To == nil || len(msg.Data) < 4 {
		return nil, errors.New("invalid contract call")
	}
	if *msg.To == r.rfqCaller && bytes.Equal(msg.Data[:4], executorBinding.GetABI().Methods["fill"].ID) {
		if msg.From != r.signer || block == nil || block.Uint64() != 7 || msg.Gas == 0 {
			return nil, errors.New("outer fill must simulate from the host signer at the pinned block and gas budget")
		}
		r.outerCalls++
		r.outerGas = msg.Gas
		r.outerData = bytes.Clone(msg.Data)
		if r.reorgDuringFill {
			r.header.Extra = []byte("new canonical hash")
		}
		return nil, r.outerErr
	}
	if *msg.To == vlt && (bytes.Equal(msg.Data, connectorBinding.PackAccount()) || bytes.Equal(msg.Data, connectorBinding.PackOutputAsset())) {
		r.identityReads++
	}
	if block == nil || block.Uint64() != 7 {
		return nil, errors.New("metadata read escaped recipe snapshot")
	}
	if *msg.To == vlt {
		result, err := r.connectorReadFake.CallContract(ctx, msg, block)
		if r.reorgBeforeDiscovery && bytes.Equal(msg.Data[:4], connectorBinding.PackOutputAsset()[:4]) {
			r.header.Extra = []byte("new binding block")
		}
		return result, err
	}
	if *msg.To == r.recipeExecutor {
		contractABI := recipeBinding.GetABI()
		method, err := contractABI.MethodById(msg.Data)
		if err != nil {
			return nil, err
		}
		switch method.RawName {
		case "account":
			return method.Outputs.Pack(r.recipeAccount)
		case "caller":
			return method.Outputs.Pack(r.recipeCaller)
		case "recipeHash":
			return method.Outputs.Pack(r.recipeHash)
		case "recipeBlock":
			return method.Outputs.Pack(big.NewInt(7))
		}
	}
	if *msg.To == r.recipeAccount {
		contractABI := merkleaccount.NewMerkleAccount().GetABI()
		method, err := contractABI.MethodById(msg.Data)
		if err != nil {
			return nil, err
		}
		switch method.RawName {
		case "root":
			return method.Outputs.Pack(r.root)
		case "hasRole":
			args, err := method.Inputs.Unpack(msg.Data[4:])
			if err != nil {
				return nil, err
			}
			if !recipes.Proof(r.root, common.Hash(args[0].([32]byte)), []common.Hash{r.approvalHash}) && !recipes.Proof(r.root, common.Hash(args[0].([32]byte)), []common.Hash{r.swapHash}) || args[1].(common.Address) != r.recipeExecutor {
				return nil, errors.New("role check used a different leaf or EOA holder")
			}
			return method.Outputs.Pack(r.allowed)
		}
	}
	return nil, errors.New("unexpected contract call or direct inner simulation")
}

type connectorExecutionFixtureData struct {
	workflow *connectorWorkflow
	reader   *connectorExecutionReader
	recipe   recipes.Recipe
	leaf     recipes.Leaf
}

func connectorExecutionFixture(t *testing.T, resolved *discounts.Resolved) connectorExecutionFixtureData {
	t.Helper()
	return connectorExecutionFixtureAt(t, resolved, common.HexToAddress("0x43"))
}

func connectorExecutionFixtureAt(t *testing.T, resolved *discounts.Resolved, recipeExecutor common.Address) connectorExecutionFixtureData {
	t.Helper()
	account := common.HexToAddress("0x21")
	rfqExecutor := common.HexToAddress("0x10")
	signer := common.HexToAddress("0x99")
	terms, err := parseConnectorDiscount(resolved, *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data, mask := connectorSignatureTemplate(terms, rfqExecutor, big.NewInt(0))
	for i := 100; i < 132; i++ {
		mask[i] = 255
	}
	leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	recipe, tree, permission := connectorCommittedFunding(t, leaf, account, rfqExecutor, tOut, tIn)
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}

	reader := &connectorExecutionReader{
		connectorReadFake: &connectorReadFake{remaining: big.NewInt(900), account: account, executor: recipeExecutor},
		recipeExecutor:    recipeExecutor, recipeAccount: account, recipeCaller: rfqExecutor, signer: signer, rfqCaller: rfqExecutor,
		recipeHash: hash, root: tree.Root, recipe: recipe, approvalHash: tree.Leaves[0].Hash, swapHash: leaf.Hash,
		header: &types.Header{Number: big.NewInt(7), Time: 7, GasLimit: 30_000_000}, allowed: true,
	}
	workflow := &connectorWorkflow{
		chain: reader, chainID: big.NewInt(1),
		source: connectorExecutionPermissions{raw: permission, account: account, executor: recipeExecutor}, caller: rfqExecutor, signer: signer,
	}
	return connectorExecutionFixtureData{workflow, reader, recipe, leaf}
}

func connectorExecutionAward(caller common.Address) (executor.IReactorOrder, selectedRoute, *fakeBackend) {
	outputs := []executor.IReactorOutput{{Token: tOut, Amount: big.NewInt(456), Recipient: common.HexToAddress("0x55")}}
	order := executor.IReactorOrder{
		Request:          executor.IReactorRequest{TokenIn: tIn, AmountIn: big.NewInt(123), Outputs: outputs, Deadline: big.NewInt(4102444800), Nonce: big.NewInt(23), Protocol: common.HexToAddress("0x45")},
		SwapperSignature: []byte{0x81, 0x82}, Swapper: common.HexToAddress("0x51"), Filler: caller, Outputs: outputs,
	}
	selected := selectedRoute{Venue: vlt, Executor: common.HexToAddress("0x43"), DiscountID: *connectorDiscountID(), MaxAssets: big.NewInt(900)}
	return order, selected, &fakeBackend{discount: connectorDiscountFixture()}
}

func TestConnectorBuildRequiresSelectedRoute(t *testing.T) {
	for _, field := range []string{"executor", "venue"} {
		t.Run(field, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			order, selected, backend := connectorExecutionAward(fixture.workflow.caller)
			if field == "executor" {
				selected.Executor = common.Address{}
			} else {
				selected.Venue = common.Address{}
			}
			_, err := fixture.workflow.build(t.Context(), order, nil, selected, backend)
			if err == nil || fixture.reader.outerCalls != 0 {
				t.Fatalf("missing %s did not fail closed: %v", field, err)
			}
		})
	}
}

func TestConnectorBuildAcceptsHeadAdvanceBetweenDiscoveryStages(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	reader := &headAdvanceReader{connectorExecutionReader: f.reader}
	f.workflow.chain = reader
	f.workflow.source = advancedPermissions{f.workflow.source}
	order, selected, backend := connectorExecutionAward(f.workflow.caller)
	if built, err := f.workflow.build(t.Context(), order, nil, selected, backend); err != nil || len(built.Calldata) == 0 {
		t.Fatalf("ordinary head advance rejected: %v", err)
	}
	if reader.connectorReads != 3 || reader.simulationBlock != 8 || f.reader.outerCalls != 1 {
		t.Fatalf("connector or settlement escaped prepared block: reads=%d simulation=%d", reader.connectorReads, reader.simulationBlock)
	}
}

// Discovery starts at head 7, then head 8 becomes available. The recipe event
// remains in canonical block 7, while the prepared connector state is block 8.
type headAdvanceReader struct {
	*connectorExecutionReader

	latestReads     int
	connectorReads  int
	simulationBlock uint64
}

func (r *headAdvanceReader) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	header := types.CopyHeader(r.header)
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
	if msg.To != nil && *msg.To == r.recipeCaller {
		r.simulationBlock = block.Uint64()
	}
	return r.connectorExecutionReader.CallContract(ctx, msg, big.NewInt(7))
}

type advancedPermissions struct{ permissionSource }

func (s advancedPermissions) PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	if snapshot.BlockNumber != 8 {
		return nil, errors.New("permissions did not use the prepared head")
	}
	snapshot.BlockNumber = 7 // The shared fixture asserts its original transport height.
	return s.permissionSource.PermissionTree(ctx, chainID, snapshot)
}
