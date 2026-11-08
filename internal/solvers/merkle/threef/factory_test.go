package threef

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	executor "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

type factorySigner struct{ address common.Address }

func (s factorySigner) Address() common.Address            { return s.address }
func (factorySigner) SignHash(common.Hash) ([]byte, error) { panic("factory must not sign") }
func (factorySigner) SignTx(context.Context, *types.Transaction, *big.Int) (*types.Transaction, error) {
	panic("factory must not sign")
}

func TestFactoryDiscoversSharedSignerAndOwnsRAMRuntime(t *testing.T) {
	for _, mode := range []string{"valid", "send", "caller mismatch", "reorg", "zero account", "policy mismatch", "zero policy account", "sepolia"} {
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
						if mode == "sepolia" {
							result = "0xaa36a7"
						}
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
						if call.To == common.HexToAddress("0x50") {
							owner := account
							if mode == "policy mismatch" {
								owner = caller
							}
							if mode == "zero policy account" {
								owner = common.Address{}
							}
							data, packErr := policyABI.GetABI().Methods["account"].Outputs.Pack(owner)
							if packErr != nil {
								t.Error(packErr)
								return
							}
							results[i] = map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": hexutil.Bytes(data)}
							continue
						}
						binding := multicall3.NewMulticall3()
						method := binding.GetABI().Methods["aggregate3"]
						args, err := method.Inputs.Unpack(call.Input[4:])
						if err != nil {
							t.Error(err)
							return
						}
						calls := *abi.ConvertType(args[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
						wantCalls := 2
						if len(calls) != wantCalls || calls[0].Target != target || calls[1].Target != target {
							t.Error("discovery lost dedup")
						}
						values := make([]multicall3.Multicall3Result, len(calls))
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
			raw := validConfig
			if mode == "send" {
				raw = strings.Replace(raw, "mode: simulate", "mode: send", 1)
			}
			owner := factorySigner{address: caller}
			manager, err := txmanager.New(client, owner, big.NewInt(1), txmanager.Config{}, logr.Discard())
			if err != nil {
				t.Fatal(err)
			}
			built, err := factory(configNode(t, raw), solver.Deps{Chain: client, Signer: owner, TxManager: manager, MaxGas: 2_000_000, Log: logr.Discard()})
			if mode == "valid" || mode == "send" || mode == "sepolia" {
				if err != nil {
					t.Fatal(err)
				}
				current := built.(*Solver)
				if current.Name() != Name || current.RequiresTxManager() != (mode == "send") || len(current.keeper.Strategies[0].Providers) != 1 || current.offers.bot.Send != (mode == "send") || getters != 2 || current.keeper.Planner.(*mintPlanning).PermissionSource == nil || current.keeper.Planner.(*mintPlanning).RecipeCache == nil || current.keeper.Planner.(*mintPlanning).MaxGas != 2_000_000 || current.keeper.Strategies[0].Config.Account != account.Hex() {
					t.Fatal("host failed shared SDK integration")
				}
				if (current.keeper.Sender != nil) != (mode == "send") || current.keeper.Pending() {
					t.Fatal("factory must create a sender only for send mode and start idle")
				}
			} else if err == nil {
				t.Fatal("invalid discovery accepted")
			}
		})
	}
}
