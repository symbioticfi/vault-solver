package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestRunServesWhileOptionalStartupSymbolRPCBlocked(t *testing.T) {
	in, out := tIn, tOut
	decimals, err := erc20.NewERC20().GetABI().Methods["decimals"].Outputs.Pack(uint8(6))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := multicall3.NewMulticall3().GetABI().Methods["aggregate3"].Outputs.Pack([]multicall3.Multicall3Result{{Success: true, ReturnData: decimals}, {Success: true, ReturnData: decimals}})
	if err != nil {
		t.Fatal(err)
	}
	started, stopped, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	calls := 0 // JSON-RPC client serializes the preseed followed by the single optional call.
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		response := map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": "0x1"}
		if q.Method == "eth_call" {
			calls++
			if calls != 1 {
				once.Do(func() { close(started) })
				select {
				case <-release:
				case <-r.Context().Done():
				}
				close(stopped)
				return
			}
			response["result"] = hexutil.Encode(encoded)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer rpc.Close()
	defer close(release)
	client, err := chain.Dial(t.Context(), []string{rpc.URL}, "", "0x0000000000000000000000000000000000000011", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	metadata := chain.NewDecimals(client)
	if _, err := metadata.GetMany(t.Context(), []common.Address{in, out}); err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	st := newStore(time.Now)
	exec := newExec(t, st, &fakeBackend{}, &fakeTxm{})
	srv := testServer()
	srv.quotes.tokenMetadata = metadata
	policy := testPermissionedPolicy(t, in)
	s := &Solver{cfg: &Config{ListenAddr: addr, PollInterval: time.Hour, TokenPolicy: policy}, server: srv, exec: exec, log: logr.Discard()}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("Run stopped before symbol request: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("optional symbol request not started")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := (&net.Dialer{Timeout: 50 * time.Millisecond}).DialContext(t.Context(), "tcp", addr)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("HTTP listener blocked on optional symbol RPC")
		}
		time.Sleep(time.Millisecond)
	}
	raw, err := json.Marshal(validQuoteBody())
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/quote", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(sharedSecretHeader, testSecret)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP while symbol blocked=%d", response.StatusCode)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("optional symbol RPC did not cancel")
	}
}
