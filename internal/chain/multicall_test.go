package chain

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

func TestMulticallWithTimeUsesOneLatestCall(t *testing.T) {
	parsed, err := abi.JSON(strings.NewReader(multicall3.Multicall3MetaData.ABI))
	if err != nil {
		t.Fatal(err)
	}
	const timestamp = 1_800_000_024
	originalResult := multicall3.Multicall3Result{Success: false, ReturnData: []byte{0xab}}
	withTimestamp := func(value *big.Int) []multicall3.Multicall3Result {
		data, err := parsed.Methods["getCurrentBlockTimestamp"].Outputs.Pack(value)
		if err != nil {
			t.Fatal(err)
		}
		return []multicall3.Multicall3Result{originalResult, {Success: true, ReturnData: data}}
	}
	for _, tc := range []struct {
		name      string
		results   []multicall3.Multicall3Result
		rpcError  bool
		wantError string
	}{
		{name: "batch time and original call results", results: withTimestamp(big.NewInt(timestamp))},
		{name: "failed timestamp", results: []multicall3.Multicall3Result{originalResult, {}}, wantError: "timestamp call failed"},
		{name: "malformed timestamp", results: []multicall3.Multicall3Result{originalResult, {Success: true, ReturnData: []byte{0xff}}}, wantError: "block timestamp:"},
		{name: "missing timestamp", results: []multicall3.Multicall3Result{originalResult}, wantError: "got 1 results, want 2"},
		{name: "zero timestamp", results: withTimestamp(new(big.Int)), wantError: "invalid block timestamp"},
		{name: "overflow timestamp", results: withTimestamp(new(big.Int).Lsh(big.NewInt(1), 255)), wantError: "invalid block timestamp"},
		{name: "rpc error", rpcError: true, wantError: "test RPC error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			multicall := common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")
			original := Call{Target: common.HexToAddress("0x1234"), AllowFailure: true, Data: []byte{1, 2, 3}}
			wantData := multicallB.PackAggregate3([]multicall3.Multicall3Call3{
				{Target: original.Target, AllowFailure: true, CallData: original.Data},
				{Target: multicall, CallData: multicallB.PackGetCurrentBlockTimestamp()},
			})
			encoded, err := parsed.Methods["aggregate3"].Outputs.Pack(tc.results)
			if err != nil {
				t.Fatal(err)
			}
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     json.RawMessage   `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x7a69"}
				switch request.Method {
				case "eth_chainId":
				case rpcMethodCall:
					calls++
					if len(request.Params) != 2 || string(request.Params[1]) != `"latest"` {
						t.Errorf("expected latest eth_call, got %s", request.Params)
						return
					}
					var call struct {
						To    common.Address `json:"to"`
						Input hexutil.Bytes  `json:"input"`
					}
					if err := json.Unmarshal(request.Params[0], &call); err != nil {
						t.Error(err)
					}
					if call.To != multicall || !bytes.Equal(call.Input, wantData) {
						t.Errorf("oracle call and timestamp must share the aggregate3 batch: %s", request.Params[0])
					}
					response["result"] = hexutil.Encode(encoded)
					if tc.rpcError {
						delete(response, "result")
						response["error"] = map[string]any{"code": -32000, "message": "test RPC error"}
					}
				default:
					t.Errorf("unexpected separate RPC read: %s", request.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			client, err := Dial(t.Context(), []string{server.URL}, "", multicall.Hex(), logr.Discard())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			results, now, err := client.MulticallWithTime(t.Context(), []Call{original})
			if calls != 1 {
				t.Fatalf("eth_call count = %d, want 1", calls)
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || !now.Equal(time.Unix(timestamp, 0)) {
				t.Fatalf("time = %s, error = %v", now, err)
			}
			if len(results) != 1 || results[0].Success || !bytes.Equal(results[0].ReturnData, []byte{0xab}) {
				t.Fatalf("original result was not preserved: %+v", results)
			}
		})
	}
}
