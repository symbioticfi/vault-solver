package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

func TestSDKBatchAggregatesPinnedGettersAndKeepsSimulationCaller(t *testing.T) {
	first, second, caller := common.HexToAddress("0x01"), common.HexToAddress("0x02"), common.HexToAddress("0x03")
	batches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
			return
		}
		type request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if raw[0] != '[' {
			var q request
			_ = json.Unmarshal(raw, &q)
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": "0x1"})
			return
		}
		batches++
		var qs []request
		if err := json.Unmarshal(raw, &qs); err != nil || len(qs) != 2 {
			t.Error("wrong HTTP array shape", err)
			return
		}
		responses := make([]any, 2)
		for i, q := range qs {
			var call struct {
				From  common.Address `json:"from"`
				To    common.Address `json:"to"`
				Input hexutil.Bytes  `json:"input"`
			}
			if err := json.Unmarshal(q.Params[0], &call); err != nil {
				t.Error(err)
				return
			}
			if string(q.Params[1]) != `"0xc"` {
				t.Error("SDK call lost pinned height")
			}
			var result []byte
			if i == 0 {
				if call.To != common.HexToAddress(protocolio.StandardMulticall3) || call.From != (common.Address{}) {
					t.Error("getter aggregate context changed")
				}
				method := multicallB.GetABI().Methods["aggregate3"]
				args, err := method.Inputs.Unpack(call.Input[4:])
				if err != nil {
					t.Error(err)
					return
				}
				members := *abi.ConvertType(args[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
				if len(members) != 2 || members[0].Target != first || members[1].Target != second || !members[0].AllowFailure {
					t.Error("safe members changed")
				}
				result, err = method.Outputs.Pack([]multicall3.Multicall3Result{{Success: true, ReturnData: []byte{1}}, {Success: false}})
				if err != nil {
					t.Error(err)
					return
				}
			} else {
				if call.From != caller || call.To != first {
					t.Error("simulation caller or target changed")
				}
				result = []byte{3}
			}
			responses[1-i] = map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": hexutil.Bytes(result)}
		}
		_ = json.NewEncoder(w).Encode(responses)
	}))
	defer server.Close()
	client, err := Dial(t.Context(), []string{server.URL}, "", common.HexToAddress("0x99").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	qs := []protocolio.ReadRequest{protocolio.ContractRead(first, []byte{1}, big.NewInt(12), true), protocolio.ContractRead(second, []byte{2}, big.NewInt(12), true), protocolio.ContractRead(first, []byte{3}, big.NewInt(12), false)}
	qs[2].Call.From = caller
	results, err := client.BatchRead(t.Context(), qs)
	if err != nil || batches != 1 || len(results) != 3 || results[0].Data[0] != 1 || results[1].Err == nil || results[2].Data[0] != 3 {
		t.Fatalf("wrong SDK associations %+v %v", results, err)
	}
}

func TestSDKCodeOverridePreservesFullCallAndNumericBlock(t *testing.T) {
	target, caller := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		result := "0x1"
		if q.Method == "eth_call" {
			if len(q.Params) != 3 || string(q.Params[1]) != `"0xc"` {
				t.Error("override arguments changed")
			}
			var call map[string]any
			_ = json.Unmarshal(q.Params[0], &call)
			if call["from"] != caller.Hex() || call["to"] != target.Hex() || call["input"] != "0x0304" || call["value"] != "0x5" || call["gas"] != "0x6" || call["maxFeePerGas"] != "0x7" {
				t.Error("override call changed")
			}
			var overrides map[string]map[string]string
			_ = json.Unmarshal(q.Params[2], &overrides)
			if len(overrides) != 1 || overrides[target.Hex()]["code"] != "0x0809" {
				t.Error("override code changed")
			}
			result = "0x0a"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": result})
	}))
	defer server.Close()
	client, err := Dial(t.Context(), []string{server.URL}, "", common.HexToAddress("0x99").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallContractWithCode(t.Context(), ethereum.CallMsg{From: caller, To: &target, Data: []byte{3, 4}, Value: big.NewInt(5), Gas: 6, GasFeeCap: big.NewInt(7)}, big.NewInt(12), []byte{8, 9})
	if err != nil || len(result) != 1 || result[0] != 10 {
		t.Fatal("override failed", err)
	}
}

// HTTP batches preserve member errors and pinned call context.
func TestSDKBatchMemberIsolation(t *testing.T) {
	service := &sdkBatchAPI{}
	server := rpc.NewServer()
	if err := server.RegisterName("eth", service); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	client, err := Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x99").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	target, caller := common.HexToAddress("0x1000"), common.HexToAddress("0x2000")
	requests := []protocolio.ReadRequest{}
	for _, data := range []byte{1, 255, 3} {
		request := protocolio.ContractRead(target, []byte{data}, big.NewInt(12), false)
		request.Call.From = caller
		requests = append(requests, request)
	}
	results, err := client.BatchRead(t.Context(), requests)
	if err != nil || len(results) != 3 {
		t.Fatalf("HTTP batch failed: results=%+v err=%v", results, err)
	}
	if !bytes.Equal(results[0].Data, []byte{1}) || results[1].Err == nil || !bytes.Equal(results[2].Data, []byte{3}) {
		t.Fatalf("member isolation lost: %+v", results)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.calls) != 3 {
		t.Fatalf("calls=%d", len(service.calls))
	}
	for _, call := range service.calls {
		if call.from != caller || call.to != target || call.block != "0xc" {
			t.Fatalf("call context changed: %+v", call)
		}
	}
}

type sdkBatchCall struct {
	from, to common.Address
	block    string
}
type sdkBatchAPI struct {
	mu    sync.Mutex
	calls []sdkBatchCall
}

func (*sdkBatchAPI) ChainId() hexutil.Uint64 { return 1 }
func (s *sdkBatchAPI) Call(_ context.Context, args struct {
	From  common.Address `json:"from"`
	To    common.Address `json:"to"`
	Input hexutil.Bytes  `json:"input"`
}, block string) (hexutil.Bytes, error) {
	s.mu.Lock()
	s.calls = append(s.calls, sdkBatchCall{args.From, args.To, block})
	s.mu.Unlock()
	if len(args.Input) == 1 && args.Input[0] == 255 {
		return nil, errors.New("synthetic route failure")
	}
	return args.Input, nil
}

func TestSDKRecipePublicationLogsUseSharedTransport(t *testing.T) {
	executor, event := common.HexToAddress("0x01"), common.HexToHash("0x02")
	query := ethereum.FilterQuery{FromBlock: big.NewInt(12), ToBlock: big.NewInt(12), Addresses: []common.Address{executor}, Topics: [][]common.Hash{{event}}}
	server := rpc.NewServer()
	if err := server.RegisterName("eth", sdkLogsAPI{executor, event}); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	client, err := Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x99").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	logs, err := client.FilterLogs(t.Context(), query)
	if err != nil || len(logs) != 1 || logs[0].Address != executor {
		t.Fatalf("single publication lookup: logs=%+v err=%v", logs, err)
	}
	results, err := client.BatchRead(t.Context(), []protocolio.ReadRequest{{Kind: protocolio.ReadLogs, Filter: query}})
	if err != nil || len(results) != 1 || results[0].Err != nil || len(results[0].Logs) != 1 || results[0].Logs[0].Address != executor {
		t.Fatalf("batch publication lookup: results=%+v err=%v", results, err)
	}
}

type sdkLogsAPI struct {
	executor common.Address
	event    common.Hash
}

func (sdkLogsAPI) ChainId() hexutil.Uint64 { return 1 }
func (s sdkLogsAPI) GetLogs(_ context.Context, filter struct {
	FromBlock string           `json:"fromBlock"`
	ToBlock   string           `json:"toBlock"`
	Addresses []common.Address `json:"address"`
	Topics    [][]common.Hash  `json:"topics"`
}) ([]types.Log, error) {
	if filter.FromBlock != "0xc" || filter.ToBlock != "0xc" || len(filter.Addresses) != 1 || filter.Addresses[0] != s.executor || len(filter.Topics) != 1 || len(filter.Topics[0]) != 1 || filter.Topics[0][0] != s.event {
		return nil, errors.New("recipe publication filter changed")
	}
	return []types.Log{{Address: s.executor, Topics: []common.Hash{s.event}, Data: []byte{}, BlockNumber: 12}}, nil
}
