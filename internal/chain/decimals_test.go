package chain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

func TestDecimalsGet(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000018")
	malformed := []byte{0x01}
	_, unpackErr := erc20B.UnpackDecimals(malformed)

	tests := []struct {
		name        string
		result      multicall3.Multicall3Result
		want        int
		wantErr     string
		wantWrapped string
		secondGet   bool
		wantCalls   int64
	}{
		{
			name:      "decodes and caches decimals",
			result:    multicall3.Multicall3Result{Success: true, ReturnData: encodeDecimalsResult(t, 18)},
			want:      18,
			secondGet: true,
			wantCalls: 1,
		},
		{
			name:      "failed call result",
			result:    multicall3.Multicall3Result{Success: false},
			wantErr:   fmt.Sprintf("erc20.decimals() reverted for %s", token),
			wantCalls: 1,
		},
		{
			name:        "malformed return data",
			result:      multicall3.Multicall3Result{Success: true, ReturnData: malformed},
			wantErr:     fmt.Sprintf("unpack decimals: %v", unpackErr),
			wantWrapped: unpackErr.Error(),
			wantCalls:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, calls := newDecimalsTestClient(t, encodeMulticallResults(t, tt.result))
			decimals := NewDecimals(client)

			got, err := decimals.Get(t.Context(), token)
			assertDecimalsGetResult(t, got, err, tt.want, tt.wantErr, tt.wantWrapped)

			if tt.secondGet {
				got, err = decimals.Get(t.Context(), token)
				if err != nil {
					t.Fatalf("cached Get: %v", err)
				}
				if got != tt.want {
					t.Fatalf("cached Get = %d, want %d", got, tt.want)
				}
			}
			if gotCalls := calls.Load(); gotCalls != tt.wantCalls {
				t.Fatalf("Multicall count = %d, want %d", gotCalls, tt.wantCalls)
			}
		})
	}
}

func assertDecimalsGetResult(t *testing.T, got int, err error, want int, wantErr, wantWrapped string) {
	t.Helper()
	if wantErr == "" && err != nil {
		t.Fatalf("Get: %v", err)
	}
	if wantErr == "" && got != want {
		t.Fatalf("Get = %d, want %d", got, want)
	}
	if wantErr != "" && (err == nil || err.Error() != wantErr) {
		t.Fatalf("Get error = %v, want %q", err, wantErr)
	}
	if wantWrapped == "" {
		return
	}
	wrapped := errors.Unwrap(err)
	if wrapped == nil {
		t.Fatal("Get error does not unwrap")
	}
	for cause := errors.Unwrap(wrapped); cause != nil; cause = errors.Unwrap(wrapped) {
		wrapped = cause
	}
	if wrapped.Error() != wantWrapped {
		t.Fatalf("wrapped error = %v, want %q", wrapped, wantWrapped)
	}
}

func encodeDecimalsResult(t *testing.T, decimals uint8) []byte {
	t.Helper()
	contractABI, err := erc20.ERC20MetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse ERC20 ABI: %v", err)
	}
	encoded, err := contractABI.Methods["decimals"].Outputs.Pack(decimals)
	if err != nil {
		t.Fatalf("pack decimals result: %v", err)
	}
	return encoded
}

func encodeMulticallResults(t *testing.T, results ...multicall3.Multicall3Result) []byte {
	t.Helper()
	contractABI, err := multicall3.Multicall3MetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse Multicall3 ABI: %v", err)
	}
	encoded, err := contractABI.Methods["aggregate3"].Outputs.Pack(results)
	if err != nil {
		t.Fatalf("pack aggregate3 result: %v", err)
	}
	return encoded
}

func newDecimalsTestClient(t *testing.T, multicallResult []byte) (*Client, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "eth_chainId":
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x1"}`, request.ID)
		case "eth_call":
			calls.Add(1)
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x%x"}`, request.ID, multicallResult)
		default:
			_, _ = fmt.Fprintf(
				w,
				`{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`,
				request.ID,
			)
		}
	}))
	t.Cleanup(server.Close)

	const multicallAddress = "0x0000000000000000000000000000000000000001"
	client, err := Dial(t.Context(), []string{server.URL}, "", multicallAddress, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(client.Close)
	return client, &calls
}
