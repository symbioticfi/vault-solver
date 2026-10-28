package chain

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
)

func TestSDKBatchReceiptFailover(t *testing.T) {
	for _, tc := range []struct {
		name, primary                     string
		fallback                          bool
		groupError, memberError, notFound bool
	}{
		{name: "all null retries unchanged batch", primary: "null", fallback: true},
		{name: "truncated response retries unchanged batch", primary: "truncated", fallback: true},
		{name: "partial receipt retained", primary: "receipt"},
		{name: "mixed member error retained", primary: "error", memberError: true},
		{name: "all endpoints null", primary: "all-null", fallback: true, notFound: true},
		{name: "duplicate response IDs", primary: "duplicate", groupError: true},
		{name: "missing response ID", primary: "missing", groupError: true},
		{name: "unknown response ID", primary: "unknown", groupError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fallbackHits int
			var primaryPayload string
			serve := func(w http.ResponseWriter, r *http.Request, fallback bool) {
				body, _ := io.ReadAll(r.Body)
				if len(body) == 0 || body[0] != '[' {
					var request struct {
						ID json.RawMessage `json:"id"`
					}
					_ = json.Unmarshal(body, &request)
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x1"})
					return
				}
				if r.Header.Get(erpcRetryEmptyHeader) != "false" {
					t.Error("pending batch lacks eRPC policy")
				}
				if fallback {
					fallbackHits++
					if string(body) != primaryPayload {
						t.Error("batch changed during failover")
					}
				} else {
					primaryPayload = string(body)
				}
				if !fallback && tc.primary == "truncated" {
					w.Header().Set("Content-Length", "1000")
					_, _ = io.WriteString(w, "[")
					return
				}
				responses := []map[string]any{
					{"jsonrpc": "2.0", "id": 2, "result": nil},
					{"jsonrpc": "2.0", "id": 1, "result": nil},
				}
				if tc.primary == "receipt" || (fallback && tc.primary != "all-null") {
					responses[1]["result"] = &types.Receipt{Status: 1, BlockNumber: big.NewInt(9), TxHash: common.HexToHash("0x01"), Logs: []*types.Log{}}
				}
				if !fallback {
					switch tc.primary {
					case "error":
						responses[1]["error"] = map[string]any{"code": -32000, "message": "synthetic member failure"}
					case "duplicate":
						responses[0]["id"] = 1
					case "missing":
						responses = responses[:1]
					case "unknown":
						responses[0]["id"] = 3
					}
				}
				_ = json.NewEncoder(w).Encode(responses)
			}
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serve(w, r, false) }))
			defer primary.Close()
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serve(w, r, true) }))
			defer fallback.Close()
			client, err := Dial(t.Context(), []string{primary.URL, fallback.URL}, "", common.HexToAddress("0x99").Hex(), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			results, err := protocolio.ReadGroup(t.Context(), client,
				protocolio.ReadRequest{Kind: protocolio.ReadReceipt, Hash: common.HexToHash("0x01")},
				protocolio.ReadRequest{Kind: protocolio.ReadReceipt, Hash: common.HexToHash("0x02")})
			if (err != nil) != tc.groupError {
				t.Fatalf("group error = %v", err)
			}
			if (fallbackHits > 0) != tc.fallback {
				t.Errorf("fallback hits = %d", fallbackHits)
			}
			if tc.groupError {
				return
			}
			first := results.At(0)
			switch {
			case tc.memberError:
				var memberErr rpc.Error
				if !errors.As(first.Err, &memberErr) || memberErr.ErrorCode() != -32000 {
					t.Error("member error hidden")
				}
			case tc.notFound:
				if !errors.Is(first.Err, ethereum.NotFound) {
					t.Errorf("missing receipt = %v", first.Err)
				}
			default:
				if first.Err != nil || first.Receipt == nil || first.Receipt.BlockNumber.Uint64() != 9 {
					t.Errorf("canonical receipt lost: %v", first.Err)
				}
			}
		})
	}
}

func TestFallbackTransportUnsafeNullBatches(t *testing.T) {
	for _, payload := range []string{
		`[{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionReceipt"},{"jsonrpc":"2.0","id":2,"method":"eth_sendRawTransaction"}]`,
		`[{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionReceipt"},{"jsonrpc":"2.0","method":"eth_getBlockByNumber"}]`,
		`[{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionReceipt"},{"jsonrpc":"2.0","id":1,"method":"eth_getBlockByNumber"}]`,
		`[{"jsonrpc":"2.0","id":1,"method":"unknown"}]`,
	} {
		primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `[{"jsonrpc":"2.0","id":1,"result":null}]`)
		}))
		fallback := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unsafe batch retried for null") }))
		resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), payload)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		primary.Close()
		fallback.Close()
	}
}

func TestFallbackTransportBatchInspectionEnvelope(t *testing.T) {
	for _, count := range []int{256, 257} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			requests := make([]map[string]any, count)
			responses := make([]map[string]any, count)
			for i := range count {
				requests[i] = map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "eth_getTransactionReceipt"}
				responses[i] = map[string]any{"jsonrpc": "2.0", "id": i + 1, "result": nil, "padding": strings.Repeat("x", 1024)}
			}
			payload, err := json.Marshal(requests)
			if err != nil {
				t.Fatal(err)
			}
			var fallbackHits int
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(responses) }))
			defer primary.Close()
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fallbackHits++; _ = json.NewEncoder(w).Encode(responses) }))
			defer fallback.Close()
			resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), string(payload))
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil || len(body) < 64<<10 {
				t.Fatal("large batch body not restored", err)
			}
			if (fallbackHits == 1) != (count == 256) {
				t.Fatalf("batch member bound violated: hits=%d", fallbackHits)
			}
		})
	}
}
