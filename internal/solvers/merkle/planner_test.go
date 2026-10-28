package merkle

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

type plannerRPCRequest struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func TestFactoryPlannerAuthenticatesCuratorAndRejectsFreeInputs(t *testing.T) {
	for _, mode := range []string{"valid", "wrong chain", "wrong root", "unused invalid proof", "free input"} {
		t.Run(mode, func(t *testing.T) {
			adapter := common.HexToAddress("0x100")
			executor, account, caller := common.HexToAddress("0x10"), common.HexToAddress("0x20"), common.HexToAddress("0x30")
			root := common.HexToHash("0x40")
			header := &types.Header{Number: big.NewInt(12), Difficulty: new(big.Int)}
			recipe := recipes.Recipe{}
			if mode == "free input" {
				word, err := abi.NewType("uint256", "", nil)
				if err != nil {
					t.Fatal(err)
				}
				body, err := (abi.Arguments{{Type: word}, {Type: word}}).Pack(new(big.Int).Lsh(big.NewInt(int64(recipes.Input)), 216), new(big.Int))
				if err != nil {
					t.Fatal(err)
				}
				recipe.Steps = [][]byte{append([]byte{recipes.Add}, body...)}
			}
			recipeHash, err := recipes.HashRecipe(recipe)
			if err != nil {
				t.Fatal(err)
			}
			simulations, fetches := 0, 0
			curator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fetches++
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/merkle/"+strings.ToLower(adapter.Hex())+"/"+root.Hex() || r.URL.RawQuery != "" {
					t.Error("host requested a different permission artifact")
				}
				doc := map[string]any{"adapter": adapter.Hex(), "chainId": 1, "root": root.Hex(), "account": account.Hex(), "leaves": []any{}}
				switch mode {
				case "wrong chain":
					doc["chainId"] = 2
				case "wrong root":
					doc["root"] = common.HexToHash("0x50").Hex()
				case "unused invalid proof":
					leaf := recipes.Leaf{Target: account, Value: new(big.Int), Data: []byte{1, 2, 3, 4}, DataMask: make([]byte, 4)}
					hash, err := recipes.HashLeaf(leaf)
					if err != nil {
						t.Error(err)
						return
					}
					doc["leaves"] = []any{map[string]any{"hash": hash.Hex(), "target": account.Hex(), "value": "0", "data": "0x01020304", "dataMask": "0x00000000", "proof": []any{}}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"bundle": map[string]any{"chainId": 1, "adapter": adapter.Hex(), "account": account.Hex(), "root": root.Hex(), "leaves": []common.Hash{root}, "metadata": doc}})
			}))
			defer curator.Close()
			binding := executorbinding.NewRecipeExecutor()
			var contractCall func(common.Address, []byte) []byte
			contractCall = func(target common.Address, data []byte) []byte {
				if target == common.HexToAddress(protocolio.StandardMulticall3) {
					aggregate := multicall3.NewMulticall3()
					method := aggregate.GetABI().Methods["aggregate3"]
					args, err := method.Inputs.Unpack(data[4:])
					if err != nil {
						t.Error(err)
						return nil
					}
					calls := *abi.ConvertType(args[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
					results := make([]multicall3.Multicall3Result, len(calls))
					for i, call := range calls {
						results[i] = multicall3.Multicall3Result{Success: true, ReturnData: contractCall(call.Target, call.CallData)}
					}
					raw, err := method.Outputs.Pack(results)
					if err != nil {
						t.Error(err)
					}
					return raw
				}
				if target == account {
					accountABI := accountbinding.NewMerkleAccount().GetABI()
					method, err := accountABI.MethodById(data)
					if err != nil {
						t.Error(err)
						return nil
					}
					if method.Name == "adapter" {
						return genericPack(t, method.Outputs, adapter)
					}
					raw, err := accountbinding.NewMerkleAccount().GetABI().Methods["root"].Outputs.Pack(root)
					if err != nil {
						t.Error(err)
					}
					return raw
				}
				executorABI := binding.GetABI()
				method, err := executorABI.MethodById(data)
				if err != nil || target != executor {
					t.Error("unexpected contract call", err)
					return nil
				}
				var value any
				switch method.Name {
				case "account":
					value = account
				case "caller":
					value = caller
				case "recipeHash":
					value = recipeHash
				case "recipeBlock":
					value = big.NewInt(12)
				case "execute":
					simulations++
					returned, packErr := method.Outputs.Pack([]*big.Int{})
					if packErr != nil {
						t.Error(packErr)
					}
					return returned
				default:
					t.Error("unexpected getter", method.Name)
					return nil
				}
				raw, err := method.Outputs.Pack(value)
				if err != nil {
					t.Error(err)
				}
				return raw
			}
			rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var raw json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
					t.Error(err)
					return
				}
				batch := raw[0] == '['
				var qs []plannerRPCRequest
				if batch {
					_ = json.Unmarshal(raw, &qs)
				} else {
					qs = make([]plannerRPCRequest, 1)
					_ = json.Unmarshal(raw, &qs[0])
				}
				results := make([]any, len(qs))
				for i, q := range qs {
					var result any
					switch q.Method {
					case "eth_chainId":
						result = "0x1"
					case "eth_getBlockByNumber":
						result = header
					case "eth_getLogs":
						t.Error("request-time RPC log scan is forbidden")
						return
					case "eth_call":
						var call struct {
							From  common.Address `json:"from"`
							To    common.Address `json:"to"`
							Input hexutil.Bytes  `json:"input"`
						}
						_ = json.Unmarshal(q.Params[0], &call)
						if string(q.Params[1]) != `"0xc"` && string(q.Params[1]) != `"latest"` {
							t.Error("planner lost pinned block")
						}
						if call.To == executor && len(call.Input) >= 4 && string(call.Input[:4]) == string(binding.GetABI().Methods["execute"].ID) && call.From != caller {
							t.Error("simulation lost shared caller")
						}
						result = hexutil.Bytes(contractCall(call.To, call.Input))
					default:
						t.Error("unexpected RPC method", q.Method)
					}
					results[i] = map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": result}
				}
				if batch {
					_ = json.NewEncoder(w).Encode(results)
				} else {
					_ = json.NewEncoder(w).Encode(results[0])
				}
			}))
			defer rpcServer.Close()
			client, err := chain.Dial(t.Context(), []string{rpcServer.URL}, "", protocolio.StandardMulticall3, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			owner := factorySigner{address: caller}
			manager := txmanager.New(client, owner, big.NewInt(1), txmanager.Config{}, logr.Discard())
			raw := strings.Replace(validConfig, "http://127.0.0.1:1/api/v1", curator.URL+"/api/v1", 1)
			indexed := indexedRecipeServer(t, executor, recipeHash, header, recipe)
			raw = strings.Replace(raw, "indexerUrl: http://127.0.0.1:1", "indexerUrl: "+indexed.URL, 1)
			raw = strings.Replace(raw, "state/merkle", filepath.Join(t.TempDir(), "state"), 1)
			built, err := factory(configNode(t, raw), solver.Deps{Chain: client, Signer: owner, TxManager: manager, Log: logr.Discard()})
			if err != nil {
				t.Fatal(err)
			}
			current := built.(*Solver)
			defer func() { _ = current.Close() }()
			_, err = current.keeper.Planner.(*keeper.Planner).Prepare(t.Context(), current.keeper.Strategies[0])
			valid := mode == "valid"
			if valid && !errors.Is(err, provider.ErrNotReady) || !valid && (err == nil || errors.Is(err, provider.ErrNotReady)) || fetches != 1 {
				t.Fatalf("curator validation: mode %s, fetches %d, error %v", mode, fetches, err)
			}
			if (simulations != 0) != (mode == "valid") {
				t.Fatalf("invalid curator or free input reached original simulation: %d", simulations)
			}
		})
	}
}

func indexedRecipeServer(t *testing.T, executor common.Address, hash common.Hash, header *types.Header, recipe recipes.Recipe) *httptest.Server {
	t.Helper()
	encode := func(values [][]byte) []string {
		out := make([]string, len(values))
		for i, value := range values {
			out[i] = hexutil.Encode(value)
		}
		return out
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/recipes/"+strings.ToLower(executor.Hex()) || r.URL.Query().Get("hash") != hash.Hex() || r.URL.Query().Get("block") != header.Number.String() {
			t.Error("indexed lookup escaped the committed executor/hash/publication block")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"executor": executor.Hex(), "recipeHash": hash.Hex(), "blockNumber": header.Number.String(), "blockHash": header.Hash().Hex(), "transactionHash": common.HexToHash("0x1234").Hex(), "logIndex": 0, "queries": encode(recipe.Queries), "steps": encode(recipe.Steps)})
	}))
	t.Cleanup(server.Close)
	return server
}
