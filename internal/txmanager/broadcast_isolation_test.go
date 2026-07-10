package txmanager

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestBroadcastFailureDoesNotFallThroughToReadFallback(t *testing.T) {
	type rpcRequest struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}

	var primaryBroadcasts, fallbackBroadcasts atomic.Int64
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode primary request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Method == "eth_sendRawTransaction" {
			primaryBroadcasts.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  "0x7a69",
		})
	}))
	defer primary.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode fallback request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Method == "eth_sendRawTransaction" {
			fallbackBroadcasts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]any{
					"code":    -32000,
					"message": "insufficient funds for gas * price + value",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  "0x7a69",
		})
	}))
	defer fallback.Close()

	client, err := chain.Dial(
		t.Context(),
		[]string{primary.URL, fallback.URL},
		"",
		"0xcA11bde05977b3631167028862bE2a173976CA11",
		31337,
		logr.Discard(),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(31337),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1),
		Gas:       21_000,
	})
	err = client.SendTransaction(t.Context(), tx)
	if err == nil {
		t.Fatal("SendTransaction succeeded, want ambiguous primary transport failure")
	}
	if got := classifyBroadcastError(err); got != broadcastAmbiguous {
		t.Fatalf("broadcast classification = %v for %q, want ambiguous", got, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "insufficient funds") {
		t.Fatalf("broadcast error was replaced by read fallback rejection: %v", err)
	}
	if got := primaryBroadcasts.Load(); got != 1 {
		t.Fatalf("primary broadcasts = %d, want 1", got)
	}
	if got := fallbackBroadcasts.Load(); got != 0 {
		t.Fatalf("read fallback broadcasts = %d, want 0", got)
	}
}
