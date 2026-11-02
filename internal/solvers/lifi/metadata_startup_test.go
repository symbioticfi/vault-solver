package lifi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Trading has already cached decimals. Only optional symbols block, as in the reproduced startup bug.
// The caller runs the real LI.FI lifecycle and asserts that recovery/quote loops progress and shutdown
// cancels this RPC, rather than replacing the metadata implementation with a no-op mock.
func blockedLIFISymbolMetadata(t *testing.T, tokens []common.Address) (metadata *chain.Decimals, symbolStarted, symbolCanceled <-chan struct{}) {
	t.Helper()
	decimals, err := erc20.NewERC20().GetABI().Methods["decimals"].Outputs.Pack(uint8(6))
	if err != nil {
		t.Fatal(err)
	}
	results := make([]multicall3.Multicall3Result, len(tokens))
	for i := range results {
		results[i] = multicall3.Multicall3Result{Success: true, ReturnData: decimals}
	}
	encoded, err := multicall3.NewMulticall3().GetABI().Methods["aggregate3"].Outputs.Pack(results)
	if err != nil {
		t.Fatal(err)
	}
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var startedOnce, canceledOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0xaa36a7"}
		if request.Method == "eth_call" {
			if calls.Add(1) == 1 {
				response["result"] = hexutil.Encode(encoded)
			} else {
				startedOnce.Do(func() { close(started) })
				select {
				case <-r.Context().Done():
					canceledOnce.Do(func() { close(canceled) })
					return
				case <-release:
				}
				delete(response, "result")
				response["error"] = map[string]any{"code": -32000, "message": "optional symbol unavailable"}
			}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	// Unblock the test-owned server even if an assertion fails before the solver is canceled.
	t.Cleanup(func() { close(release); server.Close() })
	client, err := chain.Dial(t.Context(), []string{server.URL}, "", common.HexToAddress("0x11").Hex(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	metadata = chain.NewDecimals(client)
	if _, err := metadata.GetMany(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	return metadata, started, canceled
}
