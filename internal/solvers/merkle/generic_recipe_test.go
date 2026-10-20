package merkle

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	executorbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestFactoryPlannerUsesRecipeReadinessIndependentlyOfRequestID(t *testing.T) {
	for _, tc := range []struct {
		name, simulation    string
		ready, requestID    uint64
		wantIdle, wantError bool
	}{
		{name: "ready", ready: 1, requestID: 7},
		{name: "waiting", requestID: 7, wantIdle: true},
		{name: "zero request ID remains executable", ready: 1},
		{name: "zero request ID with closed gate waits", wantIdle: true},
		{name: "malformed result remains an error", simulation: "malformed", wantError: true},
		{name: "failed simulation remains an error", simulation: "revert", wantError: true},
		{name: "failed recipe check remains an error", simulation: "check failed", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := newGenericRecipeHost(t, tc.ready, tc.requestID, tc.simulation)
			plan, err := host.solver.keeper.Planner.Prepare(t.Context(), host.solver.keeper.Strategies[0])
			if tc.wantIdle {
				if !errors.Is(err, provider.ErrNotReady) {
					t.Fatalf("closed recipe gate must wait after successful simulation: %v", err)
				}
			} else if tc.wantError {
				if err == nil || errors.Is(err, provider.ErrNotReady) {
					t.Fatalf("invalid execution was accepted or hidden as idle: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				targets, err := recipes.ManagedTargets(plan.Calldata)
				if err != nil || len(targets) != 1 || targets["2:0"] != common.HexToAddress("0x80") {
					t.Fatalf("host lost the unknown authorized action: %v, %v", targets, err)
				}
			}
			if host.simulations.Load() == 0 {
				t.Fatal("readiness bypassed the original executor simulation")
			}
		})
	}
}

func TestFactoryPlannerRechecksUnsignedRecipeReadiness(t *testing.T) {
	host := newGenericRecipeHost(t, 1, 0, "")
	planner := host.solver.keeper.Planner.(*keeper.Planner)
	plan, err := planner.Prepare(t.Context(), host.solver.keeper.Strategies[0])
	if err != nil {
		t.Fatal(err)
	}
	before := host.simulations.Load()
	host.ready.Store(0)
	if err := planner.Validate(t.Context(), plan); !errors.Is(err, provider.ErrNotReady) {
		t.Fatalf("unsigned admission accepted a recipe whose gate closed: %v", err)
	}
	if host.simulations.Load() <= before {
		t.Fatal("unsigned readiness recheck skipped the original executor")
	}
}

type genericRecipeHost struct {
	solver             *Solver
	ready, simulations atomic.Uint64
}

func newGenericRecipeHost(t *testing.T, ready, requestID uint64, simulation string) *genericRecipeHost {
	t.Helper()
	host := &genericRecipeHost{}
	host.ready.Store(ready)
	adapter := common.HexToAddress("0x100")
	executor, account, caller := common.HexToAddress("0x10"), common.HexToAddress("0x20"), common.HexToAddress("0x30")
	header := &types.Header{Number: big.NewInt(12), Difficulty: new(big.Int)}
	recipe, leaf := genericManagedRecipe(t)
	recipeHash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	curator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/merkle/"+strings.ToLower(adapter.Hex())+"/permission-sets/"+leaf.Hash.Hex() || r.URL.RawQuery != "" {
			t.Error("host requested another permission artifact")
		}
		metadata := map[string]any{"chainId": 1, "adapter": adapter.Hex(), "root": leaf.Hash.Hex(), "account": account.Hex(), "leaves": []any{
			map[string]any{"hash": leaf.Hash.Hex(), "target": leaf.Target.Hex(), "value": "0", "data": hexutil.Encode(leaf.Data), "dataMask": hexutil.Encode(leaf.DataMask), "proof": []string{}},
		}}
		_ = json.NewEncoder(w).Encode(map[string]any{"bundle": map[string]any{"chainId": 1, "adapter": adapter.Hex(), "account": account.Hex(), "root": leaf.Hash.Hex(), "leaves": []common.Hash{leaf.Hash}, "metadata": metadata}})
	}))
	t.Cleanup(curator.Close)
	binding := executorbinding.NewRecipeExecutor()
	executorABI := binding.GetABI()
	var contractCall func(common.Address, []byte) []byte
	contractCall = func(target common.Address, data []byte) []byte {
		if target == common.HexToAddress(protocolio.StandardMulticall3) {
			method := multicall3.NewMulticall3().GetABI().Methods["aggregate3"]
			args, unpackErr := method.Inputs.Unpack(data[4:])
			if unpackErr != nil {
				t.Error(unpackErr)
				return nil
			}
			calls := *abi.ConvertType(args[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
			results := make([]multicall3.Multicall3Result, len(calls))
			for i, call := range calls {
				results[i] = multicall3.Multicall3Result{Success: true, ReturnData: contractCall(call.Target, call.CallData)}
			}
			return genericPack(t, method.Outputs, results)
		}
		if target == account {
			accountABI := accountbinding.NewMerkleAccount().GetABI()
			method, methodErr := accountABI.MethodById(data)
			if methodErr != nil {
				t.Error(methodErr)
				return nil
			}
			if method.Name == "adapter" {
				return genericPack(t, method.Outputs, adapter)
			}
			if method.Name == "hasRole" {
				return genericPack(t, method.Outputs, true)
			}
			return genericPack(t, method.Outputs, leaf.Hash)
		}
		method, methodErr := executorABI.MethodById(data)
		if methodErr != nil || target != executor {
			t.Error("unexpected contract call", methodErr)
			return nil
		}
		switch method.Name {
		case "account":
			return genericPack(t, method.Outputs, account)
		case "caller":
			return genericPack(t, method.Outputs, caller)
		case "recipeHash":
			return genericPack(t, method.Outputs, recipeHash)
		case "recipeBlock":
			return genericPack(t, method.Outputs, big.NewInt(12))
		case "execute":
			host.simulations.Add(1)
			if simulation == "malformed" {
				return []byte{1}
			}
			return genericPack(t, method.Outputs, []*big.Int{new(big.Int).SetUint64(host.ready.Load()), new(big.Int).SetUint64(requestID), new(big.Int)})
		default:
			t.Error("unexpected executor method", method.Name)
			return nil
		}
	}
	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if decodeErr := json.NewDecoder(r.Body).Decode(&raw); decodeErr != nil {
			t.Error(decodeErr)
			return
		}
		var requests []plannerRPCRequest
		batch := raw[0] == '['
		if batch {
			_ = json.Unmarshal(raw, &requests)
		} else {
			requests = make([]plannerRPCRequest, 1)
			_ = json.Unmarshal(raw, &requests[0])
		}
		results := make([]any, len(requests))
		for i, request := range requests {
			response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
			switch request.Method {
			case "eth_chainId":
				response["result"] = "0x1"
			case "eth_getBlockByNumber":
				response["result"] = header
			case "eth_getLogs":
				event := binding.GetABI().Events["SetRecipe"]
				data := genericPack(t, event.Inputs.NonIndexed(), recipe.Queries, recipe.Steps)
				response["result"] = []types.Log{{Address: executor, BlockNumber: 12, BlockHash: header.Hash(), Topics: []common.Hash{event.ID, recipeHash}, Data: data}}
			case "eth_call":
				var call struct {
					From  common.Address `json:"from"`
					To    common.Address `json:"to"`
					Input hexutil.Bytes  `json:"input"`
				}
				_ = json.Unmarshal(request.Params[0], &call)
				if string(request.Params[1]) != `"0xc"` && string(request.Params[1]) != `"latest"` || call.To == executor && call.From != caller {
					t.Error("host lost the pinned block or shared caller")
				}
				response["result"] = hexutil.Bytes(contractCall(call.To, call.Input))
				if (simulation == "revert" || simulation == "check failed") && call.To == executor && len(call.Input) >= 4 && string(call.Input[:4]) == string(binding.GetABI().Methods["execute"].ID) {
					delete(response, "result")
					rpcError := map[string]any{"code": 3, "message": "execution reverted"}
					if simulation == "check failed" {
						rpcError["data"] = hexutil.Encode(executorbinding.RecipeExecutorCheckFailedErrorID().Bytes()[:4])
					}
					response["error"] = rpcError
				}
			default:
				t.Error("unexpected RPC method", request.Method)
			}
			results[i] = response
		}
		if batch {
			_ = json.NewEncoder(w).Encode(results)
		} else {
			_ = json.NewEncoder(w).Encode(results[0])
		}
	}))
	t.Cleanup(rpcServer.Close)
	client, err := chain.Dial(t.Context(), []string{rpcServer.URL}, "", protocolio.StandardMulticall3, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	owner := factorySigner{address: caller}
	manager := txmanager.New(client, owner, big.NewInt(1), txmanager.Config{}, logr.Discard())
	raw := strings.Replace(validConfig, "http://127.0.0.1:1/api/v1", curator.URL+"/api/v1", 1)
	raw = strings.Replace(raw, "state/merkle", filepath.Join(t.TempDir(), "state"), 1)
	built, err := factory(configNode(t, raw), solver.Deps{Chain: client, Signer: owner, TxManager: manager, Log: logr.Discard()})
	if err != nil {
		t.Fatal(err)
	}
	host.solver = built.(*Solver)
	t.Cleanup(func() { _ = host.solver.Close() })
	return host
}

func genericManagedRecipe(t *testing.T) (recipes.Recipe, recipes.Leaf) {
	t.Helper()
	word, _ := abi.NewType("uint256", "", nil)
	address, _ := abi.NewType("address", "", nil)
	data, _ := abi.NewType("bytes", "", nil)
	words, _ := abi.NewType("uint256[]", "", nil)
	hashes, _ := abi.NewType("bytes32[]", "", nil)
	bindings, _ := abi.NewType("uint256[][]", "", nil)
	leaf := recipes.Leaf{Target: common.HexToAddress("0x80"), Value: new(big.Int), Data: make([]byte, 36), DataMask: make([]byte, 36)}
	copy(leaf.Data, []byte{0xde, 0xad, 0xbe, 0xef})
	for i := 4; i < len(leaf.DataMask); i++ {
		leaf.DataMask[i] = 0xff
	}
	var err error
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	queryArgs := abi.Arguments{{Type: address}, {Type: data}, {Type: words}, {Type: word}}
	queries := [][]byte{
		genericPack(t, queryArgs, leaf.Target, []byte{1, 2, 3, 4}, []*big.Int{}, new(big.Int)),
		genericPack(t, queryArgs, leaf.Target, []byte{5, 6, 7, 8}, []*big.Int{}, new(big.Int)),
	}
	gate := new(big.Int).Lsh(big.NewInt(int64(recipes.Result)), 216)
	request := new(big.Int).Or(new(big.Int).Set(gate), big.NewInt(1))
	requestBinding := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(4), 224), request)
	return recipes.Recipe{Queries: queries, Steps: [][]byte{
		append([]byte{recipes.Read}, genericPack(t, abi.Arguments{{Type: word}}, new(big.Int))...),
		append([]byte{recipes.Read}, genericPack(t, abi.Arguments{{Type: word}}, big.NewInt(1))...),
		append([]byte{recipes.Manage}, genericPack(t, abi.Arguments{{Type: word}, {Type: hashes}, {Type: bindings}}, gate, [][32]byte{leaf.Hash}, [][]*big.Int{{requestBinding}})...),
	}}, leaf
}

func genericPack(t *testing.T, args abi.Arguments, values ...any) []byte {
	t.Helper()
	data, err := args.Pack(values...)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
