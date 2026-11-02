package uniswapx

import (
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
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

type metadataStartupReader struct{ *startupChainReader }

func (*metadataStartupReader) latestBlockTime(context.Context) (time.Time, error) {
	return time.Now(), nil
}
func (*metadataStartupReader) quoteSnapshot(context.Context, []liquidlane.Route, common.Address) (snapshot, error) {
	return snapshot{}, nil
}

func TestRunServesWhileOptionalStartupSymbolRPCBlocked(t *testing.T) {
	in, out := common.HexToAddress("0x1"), common.HexToAddress("0x2")
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
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatal(err)
	}
	route := liquidlane.Route{Adapter: common.HexToAddress("0x3"), TokenIn: in, TokenOut: out}
	s := &Solver{chainID: 1, cfg: &Config{SolverMode: solverModeExternal, Adapters: []common.Address{route.Adapter}, TokenPolicy: policy,
		QuoteServer: QuoteServerConfig{ListenAddress: addr, RefreshInterval: time.Hour, QuoteTTL: time.Minute}, OrderServer: OrderServerConfig{PollInterval: time.Hour}},
		reader: &metadataStartupReader{&startupChainReader{routes: []liquidlane.Route{route}}}, tokenMetadata: metadata,
		orders: orderPollerFunc(func(context.Context, int64, *common.Address) ([]orderEntry, error) { return nil, nil }), log: logr.Discard()}
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
	clientHTTP := &http.Client{Timeout: 50 * time.Millisecond}
	for {
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/quote", nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response, err := clientHTTP.Do(request)
		if err == nil {
			_ = response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("HTTP listener blocked on optional symbol RPC")
		}
		time.Sleep(time.Millisecond)
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
