package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

// This fixture leaves discovery, permission verification and calldata assembly in the SDK;
// only the chain and curator transports are replaced.
func TestConnectorBuildSimulatesCompleteAwardedOrder(t *testing.T) {
	fixture := connectorExecutionFixture(t)
	w, chain, recipe, leaf := fixture.workflow, fixture.reader, fixture.recipe, fixture.leaf
	order, selected, backend := connectorExecutionAward(w.caller)
	calldata, until, err := w.build(t.Context(), order, []byte{0x91, 0x92}, selected, backend)
	if err != nil {
		t.Fatal(err)
	}
	if !until.Equal(time.Unix(4102444800, 0)) {
		t.Fatalf("discount expiry = %v", until)
	}
	if chain.outerCalls != 1 || !bytes.Equal(calldata, chain.outerData) {
		t.Fatalf("returned fill was not the simulated fill: calls=%d", chain.outerCalls)
	}
	if len(calldata) < 4 || hexutil.Encode(calldata[:4]) != "0xdd416d68" {
		t.Fatalf("incorrect RecipeFill selector: %x", calldata)
	}
	values, err := executorBinding.GetABI().Methods["fill2"].Inputs.Unpack(calldata[4:])
	if err != nil {
		t.Fatal(err)
	}
	gotOrder := *abi.ConvertType(values[0], new(executor.IReactorOrder)).(*executor.IReactorOrder)
	if !reflect.DeepEqual(gotOrder, order) || !bytes.Equal(values[1].([]byte), []byte{0x91, 0x92}) {
		t.Fatal("awarded order or protocol signature changed")
	}
	if reflect.ValueOf(values[2]).Len() != 0 || reflect.ValueOf(values[3]).Len() != 0 {
		t.Fatal("connector input was also funded as a direct or discount leg")
	}
	fill := *abi.ConvertType(values[4], new(executor.IExecutorRecipeFill)).(*executor.IExecutorRecipeFill)
	if len(fill.Routes) != 1 || len(fill.PostCalls) != 0 {
		t.Fatalf("unexpected route composition: %+v", fill)
	}
	route := fill.Routes[0]
	if route.Executor != chain.recipeExecutor || route.Connector != vlt || route.AmountIn.Int64() != 123 || len(route.Inputs) != 2 || route.Inputs[0].Int64() != 123 || route.Inputs[1].Int64() != 456 {
		t.Fatalf("awarded amounts or route changed: %+v", route)
	}
	if !reflect.DeepEqual(route.Queries, recipe.Queries) || !reflect.DeepEqual(route.Steps, recipe.Steps) || len(route.Runtime) != 1 {
		t.Fatal("committed program or runtime slots changed")
	}
	actionsType := merkleaccount.NewMerkleAccount().GetABI().Methods["manage"].Inputs
	actions, err := actionsType.Unpack(route.Runtime[0])
	if err != nil {
		t.Fatal(err)
	}
	managed := *abi.ConvertType(actions[0], new([]merkleaccount.IMerkleAccountAction)).(*[]merkleaccount.IMerkleAccountAction)
	if len(managed) != 1 || managed[0].Target != vlt || managed[0].Value.Sign() != 0 || !bytes.Equal(managed[0].DataMask, leaf.DataMask) || len(managed[0].Proof) != 0 {
		t.Fatalf("curator permission changed: %+v", managed)
	}
	swapValues, err := connectorBinding.GetABI().Methods["swap0"].Inputs.Unpack(managed[0].Data[4:])
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

func TestConnectorBuildRejectsUnauthorizedOrUnsettledExecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*connectorWorkflow, *connectorExecutionReader)
		want   string
	}{
		{"connector account differs", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.account = common.HexToAddress("0x88") }, "account"},
		{"recipe caller differs", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.recipeCaller = common.HexToAddress("0x88") }, "caller"},
		{"permission role missing", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.allowed = false }, "role"},
		{"outer fill reverts", func(_ *connectorWorkflow, r *connectorExecutionReader) {
			r.outerErr = errors.New("Reactor output shortfall")
		}, "simulat"},
		{"snapshot changes during outer fill", func(_ *connectorWorkflow, r *connectorExecutionReader) { r.reorgDuringFill = true }, "reorganiz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t)
			w, chain := fixture.workflow, fixture.reader
			tc.mutate(w, chain)
			order, selected, backend := connectorExecutionAward(w.caller)
			calldata, _, err := w.build(t.Context(), order, []byte{0x91, 0x92}, selected, backend)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if len(calldata) != 0 {
				t.Fatal("failed preparation returned transaction calldata")
			}
		})
	}
}

func TestConnectorBuildRejectsIncompleteAwardsBeforeAcquisition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*executor.IReactorOrder, **fillPlan)
		want   string
	}{
		{"missing plan", func(_ *executor.IReactorOrder, p **fillPlan) { *p = nil }, "complete awarded input"},
		{"split input", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).Legs = append((*p).Legs, (*p).Legs[0]) }, "complete awarded input"},
		{"partial input", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).Legs[0].AmountIn = big.NewInt(122) }, "complete awarded input"},
		{"wrong input token", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).TokenIn = tOut }, "complete awarded input"},
		{"wrong output token", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).TokenOut = tIn }, "output obligation"},
		{"wrong output amount", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).Legs[0].AmountOut = big.NewInt(455) }, "output obligation"},
		{"unknown connector", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).Legs[0].Adapter = common.HexToAddress("0x88") }, "configured executor"},
		{"missing discount", func(_ *executor.IReactorOrder, p **fillPlan) { (*p).Legs[0].DiscountID = nil }, "signed discount"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &connectorWorkflow{caller: common.HexToAddress("0x10"), executors: map[common.Address]common.Address{vlt: common.HexToAddress("0x43")}}
			order, selected, backend := connectorExecutionAward(w.caller)
			tc.mutate(&order, &selected)
			calldata, _, err := w.build(t.Context(), order, nil, selected, backend)
			if err == nil || !strings.Contains(err.Error(), tc.want) || len(calldata) != 0 {
				t.Fatalf("calldata=%x error=%v, want %q", calldata, err, tc.want)
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

	recipeExecutor, recipeAccount, recipeCaller, signer common.Address
	recipeHash, root                                    common.Hash
	recipe                                              recipes.Recipe
	header                                              *types.Header
	allowed                                             bool
	outerCalls                                          int
	outerData                                           []byte
	outerErr                                            error
	reorgDuringFill                                     bool
}

func (r *connectorExecutionReader) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	if number != nil && number.Uint64() != 7 {
		return nil, errors.New("unexpected unpinned header")
	}
	return types.CopyHeader(r.header), nil
}

func (r *connectorExecutionReader) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	event := recipeBinding.GetABI().Events["SetRecipe"]
	if query.FromBlock.Uint64() != 7 || query.ToBlock.Uint64() != 7 || len(query.Addresses) != 1 || query.Addresses[0] != r.recipeExecutor || len(query.Topics) != 1 || len(query.Topics[0]) != 1 || query.Topics[0][0] != event.ID {
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
	if *msg.To == r.recipeCaller && bytes.Equal(msg.Data[:4], common.FromHex("0xdd416d68")) {
		if msg.From != r.signer || block != nil {
			return nil, errors.New("outer fill must simulate from the host signer at latest")
		}
		r.outerCalls++
		r.outerData = bytes.Clone(msg.Data)
		if r.reorgDuringFill {
			r.header.Extra = []byte("new canonical hash")
		}
		return nil, r.outerErr
	}
	if block == nil || block.Uint64() != 7 {
		return nil, errors.New("metadata read escaped recipe snapshot")
	}
	if *msg.To == vlt {
		return r.connectorReadFake.CallContract(ctx, msg, block)
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
			if common.Hash(args[0].([32]byte)) != r.root || args[1].(common.Address) != r.recipeExecutor {
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

func connectorExecutionFixture(t *testing.T) connectorExecutionFixtureData {
	t.Helper()
	account := common.HexToAddress("0x21")
	rfqExecutor := common.HexToAddress("0x10")
	recipeExecutor := common.HexToAddress("0x43")
	signer := common.HexToAddress("0x99")
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data := connectorBinding.PackSwap0(terms.Swap, terms.Signature, rfqExecutor, big.NewInt(0))
	mask := bytes.Repeat([]byte{0xff}, len(data))
	clear(mask[:4])
	clear(mask[132:164])
	leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	recipe := connectorDeltaRecipe(t, leaf.Hash, account, rfqExecutor, tOut, tIn)
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := json.Marshal(map[string]any{
		"root": leaf.Hash.Hex(), "chainId": "1", "account": account.Hex(),
		"leaves": []map[string]any{{"hash": leaf.Hash.Hex(), "target": vlt.Hex(), "value": "0", "data": hexutil.Encode(data), "dataMask": hexutil.Encode(mask), "proof": []string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &connectorExecutionReader{
		connectorReadFake: &connectorReadFake{remaining: big.NewInt(900), account: account},
		recipeExecutor:    recipeExecutor, recipeAccount: account, recipeCaller: rfqExecutor, signer: signer,
		recipeHash: hash, root: leaf.Hash, recipe: recipe,
		header: &types.Header{Number: big.NewInt(7), Time: 7}, allowed: true,
	}
	workflow := &connectorWorkflow{
		chain: reader, chainID: big.NewInt(1), executors: map[common.Address]common.Address{vlt: recipeExecutor},
		source: connectorExecutionPermissions{raw: permission, account: account, executor: recipeExecutor}, caller: rfqExecutor, signer: signer,
	}
	return connectorExecutionFixtureData{workflow, reader, recipe, leaf}
}

func connectorExecutionAward(caller common.Address) (executor.IReactorOrder, *fillPlan, *fakeBackend) {
	outputs := []executor.IReactorOutput{{Token: tOut, Amount: big.NewInt(456), Recipient: common.HexToAddress("0x55")}}
	order := executor.IReactorOrder{
		Request:          executor.IReactorRequest{TokenIn: tIn, AmountIn: big.NewInt(123), Outputs: outputs, Deadline: big.NewInt(4102444800), Nonce: big.NewInt(23), Protocol: common.HexToAddress("0x45")},
		SwapperSignature: []byte{0x81, 0x82}, Swapper: common.HexToAddress("0x51"), Filler: caller, Outputs: outputs,
	}
	selected := &fillPlan{TokenIn: tIn, TokenOut: tOut, AmountIn: big.NewInt(123), QuotedAmountOut: big.NewInt(456), Legs: []fillLeg{{Adapter: vlt, AmountIn: big.NewInt(123), AmountOut: big.NewInt(456), DiscountID: connectorDiscountID()}}}
	return order, selected, &fakeBackend{discount: connectorDiscountFixture()}
}
