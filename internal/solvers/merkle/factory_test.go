package merkle

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	executor "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type factorySigner struct{ address common.Address }

func (s factorySigner) Address() common.Address            { return s.address }
func (factorySigner) SignHash(common.Hash) ([]byte, error) { panic("factory must not sign") }
func (factorySigner) SignTx(context.Context, *types.Transaction, *big.Int) (*types.Transaction, error) {
	panic("factory must not sign")
}

func TestFactoryDiscoversSharedSignerAndNeverRewritesFailedState(t *testing.T) {
	for _, mode := range []string{"valid", "caller mismatch", "reorg", "zero account"} {
		t.Run(mode, func(t *testing.T) {
			target, account, caller := common.HexToAddress("0x10"), common.HexToAddress("0x20"), common.HexToAddress("0x30")
			header := &types.Header{Number: big.NewInt(12), Difficulty: new(big.Int)}
			getters := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var raw json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&raw)
				type request struct {
					ID     json.RawMessage   `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				var qs []request
				batch := raw[0] == '['
				if batch {
					_ = json.Unmarshal(raw, &qs)
				} else {
					qs = make([]request, 1)
					_ = json.Unmarshal(raw, &qs[0])
				}
				results := make([]any, len(qs))
				for i, q := range qs {
					var result any
					switch q.Method {
					case "eth_chainId":
						result = "0x1"
					case "eth_getBlockByNumber":
						observed := types.CopyHeader(header)
						if mode == "reorg" && string(q.Params[0]) != `"latest"` {
							observed.Extra = []byte{1}
						}
						result = observed
					case "eth_call":
						getters++
						var call struct {
							To    common.Address `json:"to"`
							Input hexutil.Bytes  `json:"input"`
						}
						_ = json.Unmarshal(q.Params[0], &call)
						binding := multicall3.NewMulticall3()
						method := binding.GetABI().Methods["aggregate3"]
						args, err := method.Inputs.Unpack(call.Input[4:])
						if err != nil {
							t.Error(err)
							return
						}
						calls := *abi.ConvertType(args[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
						if len(calls) != 2 || calls[0].Target != target || calls[1].Target != target {
							t.Error("discovery lost dedup")
						}
						values := make([]multicall3.Multicall3Result, 2)
						for j, c := range calls {
							executorABI := executor.NewRecipeExecutor().GetABI()
							m, err := executorABI.MethodById(c.CallData)
							if err != nil {
								t.Error(err)
								return
							}
							a := account
							if m.Name == "caller" {
								a = caller
								if mode == "caller mismatch" {
									a = common.HexToAddress("0x40")
								}
							} else if mode == "zero account" {
								a = common.Address{}
							}
							data, err := m.Outputs.Pack(a)
							if err != nil {
								t.Error(err)
								return
							}
							values[j] = multicall3.Multicall3Result{Success: true, ReturnData: data}
						}
						data, err := method.Outputs.Pack(values)
						if err != nil {
							t.Error(err)
							return
						}
						result = hexutil.Bytes(data)
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
			defer server.Close()
			client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.HexToAddress("0x99").Hex(), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			directory := filepath.Join(t.TempDir(), "state")
			raw := strings.Replace(validConfig, "state/merkle", directory, 1)
			owner := factorySigner{address: caller}
			manager := txmanager.New(client, owner, big.NewInt(1), txmanager.Config{}, logr.Discard())
			built, err := factory(configNode(t, raw), solver.Deps{Chain: client, Signer: owner, TxManager: manager, Log: logr.Discard()})
			if mode == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				current := built.(*Solver)
				defer func() { _ = current.Close() }()
				if getters != 1 || current.keeper.Planner.(*keeper.Planner).PermissionSource == nil || current.keeper.Planner.(*keeper.Planner).RecipeCache == nil || current.keeper.Strategies[0].Config.Account != account.Hex() {
					t.Fatal("host failed shared SDK integration")
				}
				if found, err := state.Read(directory, "txmanager.active.v1", new(any)); err != nil || found {
					t.Fatal("factory created owned transaction", err)
				}
			} else {
				if err == nil {
					_ = built.(*Solver).Close()
					t.Fatal("invalid discovery accepted")
				}
				if _, err := os.Stat(directory); !os.IsNotExist(err) {
					t.Fatal("failed discovery opened state", err)
				}
			}
		})
	}
}

func TestFactoryRejectsStrategyObjectsAndDuplicatesBeforeRPC(t *testing.T) {
	for name, body := range map[string]string{
		"named object":   "  - name: old-label\n    executor: '0x0000000000000000000000000000000000000010'\n",
		"unnamed object": "  - executor: '0x0000000000000000000000000000000000000010'\n",
		"duplicate":      "  - '0xaBcDeF0000000000000000000000000000000001'\n  - '0xabcdef0000000000000000000000000000000001'\n",
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var q struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
					return
				}
				if q.Method != "eth_chainId" {
					t.Error("invalid config triggered RPC", q.Method)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": "0x1"})
			}))
			defer server.Close()
			client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.HexToAddress("0x99").Hex(), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			calls.Store(0)
			directory := filepath.Join(t.TempDir(), "state")
			raw := strings.Replace(strategyConfig(body), "state/merkle", directory, 1)
			owner := factorySigner{address: common.HexToAddress("0x30")}
			manager := txmanager.New(client, owner, big.NewInt(1), txmanager.Config{}, logr.Discard())
			if _, err := factory(configNode(t, raw), solver.Deps{Chain: client, Signer: owner, TxManager: manager, Log: logr.Discard()}); err == nil {
				t.Fatal("obsolete strategy accepted")
			}
			if calls.Load() != 0 {
				t.Fatal("invalid strategy reached RPC")
			}
			if _, err := os.Stat(directory); !os.IsNotExist(err) {
				t.Fatal("invalid strategy opened state", err)
			}
		})
	}
}
