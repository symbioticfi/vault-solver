package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

func TestRequestReadersNeverSendBlockchainLogScans(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x1"})
	}))
	defer server.Close()
	client, err := Dial(t.Context(), []string{server.URL}, "", protocolio.StandardMulticall3, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	before := calls
	if _, err := client.FilterLogs(t.Context(), ethereum.FilterQuery{}); err == nil {
		t.Fatal("single log scan admitted")
	}
	if _, err := client.BatchRead(t.Context(), []protocolio.ReadRequest{{Kind: protocolio.ReadHeader}, {Kind: protocolio.ReadLogs}}); err == nil {
		t.Fatal("batch log scan admitted")
	}
	if calls != before {
		t.Fatal("log scan reached RPC transport")
	}
}

func TestRawClientsRejectLogScansBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x1"})
	}))
	defer server.Close()
	client, err := Dial(t.Context(), []string{server.URL}, server.URL, common.HexToAddress("0x99").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	before := calls.Load()
	for name, raw := range map[string]*rpc.Client{"read": client.Client.Client(), "write": client.writeClient.Client()} {
		t.Run(name, func(t *testing.T) {
			for _, method := range []string{"eth_getLogs", "eth_newFilter", "eth_getFilterLogs", "eth_getFilterChanges"} {
				t.Run(method, func(t *testing.T) {
					var result any
					if err := raw.CallContext(t.Context(), &result, method, map[string]any{}); err == nil || !strings.Contains(err.Error(), "log scans belong to indexers") {
						t.Fatalf("raw log scan admitted: %v", err)
					}
					// Even a preceding unknown method must not hide a later log scan.
					batch := []rpc.BatchElem{{Method: "custom_other", Result: &result}, {Method: method, Result: &result}}
					if err := raw.BatchCallContext(t.Context(), batch); err == nil || !strings.Contains(err.Error(), "log scans belong to indexers") {
						t.Fatalf("raw batch log scan admitted: %v", err)
					}
				})
			}
		})
	}
	if _, err := client.Client.FilterLogs(t.Context(), ethereum.FilterQuery{}); err == nil {
		t.Fatal("embedded ethclient bypassed log scan policy")
	}
	if calls.Load() != before {
		t.Fatal("raw log scans reached an RPC endpoint")
	}
}
