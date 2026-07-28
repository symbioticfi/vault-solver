package lifi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestValidateZeroGovernanceFee(t *testing.T) {
	tests := []struct {
		name    string
		fee     uint64
		wantErr string
	}{
		{name: "zero", fee: 0},
		{name: "non-zero", fee: 1, wantErr: "governance fee is 1, expected zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := governanceFeeRPC(t, tt.fee)
			defer server.Close()

			client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.Address{}.Hex(), logr.Discard())
			if err != nil {
				t.Fatalf("chain.Dial: %v", err)
			}
			defer client.Close()

			err = (&reader{chain: client}).validateZeroGovernanceFee(
				t.Context(), common.HexToAddress("0x1111111111111111111111111111111111111111"),
			)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("validateZeroGovernanceFee: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("validateZeroGovernanceFee error = %v", err)
			}
		})
	}
}

func governanceFeeRPC(t *testing.T, fee uint64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var rpcRequest struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read RPC request: %v", err)
		}
		if err := json.Unmarshal(body, &rpcRequest); err != nil {
			t.Errorf("decode RPC request: %v", err)
		}
		result := `"0xaa36a7"`
		if rpcRequest.Method == "eth_call" {
			result = fmt.Sprintf(`"0x%064x"`, fee)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, rpcRequest.ID, result)
	}))
}
