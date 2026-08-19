package chain

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

// mustEndpoints parses raw URLs into endpoints for a fallbackTransport, failing the test on error.
func mustEndpoints(t *testing.T, raws ...string) []*url.URL {
	t.Helper()
	eps, err := parseHTTPEndpoints(raws)
	if err != nil {
		t.Fatalf("parseHTTPEndpoints: %v", err)
	}
	return eps
}

func roundTrip(t *testing.T, eps []*url.URL, payload string) (*http.Response, error) {
	t.Helper()
	rt := &fallbackTransport{endpoints: eps, base: http.DefaultTransport, log: logr.Discard()}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, eps[0].String(), strings.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return rt.RoundTrip(req)
}

func TestFallbackTransport_FallsOverOn5xx(t *testing.T) {
	var primaryHits, fallbackHits int
	var gotBody string
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryHits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `ok`)
	}))
	defer fallback.Close()

	resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), `{"jsonrpc":"2.0"}`)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fell over to fallback)", resp.StatusCode)
	}
	if primaryHits != 1 || fallbackHits != 1 {
		t.Fatalf("hits: primary=%d fallback=%d, want 1/1", primaryHits, fallbackHits)
	}
	if gotBody != `{"jsonrpc":"2.0"}` {
		t.Fatalf("fallback got body %q, want the original payload (replayed)", gotBody)
	}
}

func TestFallbackTransport_PrimaryOKNoFallover(t *testing.T) {
	var fallbackHits int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `ok`)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		fallbackHits++
	}))
	defer fallback.Close()

	resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), `{}`)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || fallbackHits != 0 {
		t.Fatalf("status=%d fallbackHits=%d, want 200/0 (no fallover on healthy primary)", resp.StatusCode, fallbackHits)
	}
}

func TestFallbackTransport_FallsOverOnNullAvailabilityResult(t *testing.T) {
	methods := []string{
		"eth_getTransactionReceipt",
		"eth_getBlockByHash",
		"eth_getBlockByNumber",
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var primaryHits, fallbackHits int
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryHits++
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":7,"result":null}`)
			}))
			defer primary.Close()
			const fallbackBody = `{"jsonrpc":"2.0","id":7,"result":{"endpoint":"fallback"}}`
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fallbackHits++
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fallbackBody)
			}))
			defer fallback.Close()

			payload := `{"jsonrpc":"2.0","id":7,"method":"` + method + `","params":[]}`
			resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), payload)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatalf("read response: %v", readErr)
			}
			if string(body) != fallbackBody {
				t.Fatalf("response body = %s, want fallback body %s", body, fallbackBody)
			}
			if primaryHits != 1 || fallbackHits != 1 {
				t.Fatalf("hits: primary=%d fallback=%d, want 1/1", primaryHits, fallbackHits)
			}
		})
	}
}

func TestFallbackTransport_PreservesNonAvailabilityResponses(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		response string
	}{
		{
			name:     "non target method null",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_chainId","params":[]}`,
			response: `{"jsonrpc":"2.0","id":7,"result":null}`,
		},
		{
			name:     "json rpc error",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_getTransactionReceipt","params":[]}`,
			response: `{"jsonrpc":"2.0","id":7,"error":{"code":-32000,"message":"not found"}}`,
		},
		{
			name:     "error alongside null result",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_getBlockByHash","params":[]}`,
			response: `{"jsonrpc":"2.0","id":7,"result":null,"error":{"code":-32000,"message":"not found"}}`,
		},
		{
			name:     "batch request",
			request:  `[{"jsonrpc":"2.0","id":7,"method":"eth_getBlockByNumber","params":[]}]`,
			response: `[{"jsonrpc":"2.0","id":7,"result":null}]`,
		},
		{
			name:     "malformed request",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_getBlockByHash"`,
			response: `{"jsonrpc":"2.0","id":7,"result":null}`,
		},
		{
			name:     "malformed response",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_getBlockByNumber","params":[]}`,
			response: `{"jsonrpc":"2.0","id":7,"result":`,
		},
		{
			name:     "mismatched response id",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_getTransactionReceipt","params":[]}`,
			response: `{"jsonrpc":"2.0","id":8,"result":null}`,
		},
		{
			name:     "non null body is restored",
			request:  `{"jsonrpc":"2.0","id":7,"method":"eth_getBlockByHash","params":[]}`,
			response: "  {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{}}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fallbackHits int
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.response)
			}))
			defer primary.Close()
			fallback := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				fallbackHits++
			}))
			defer fallback.Close()

			resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), tt.request)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatalf("read response: %v", readErr)
			}
			if string(body) != tt.response {
				t.Fatalf("response body = %q, want preserved primary body %q", body, tt.response)
			}
			if fallbackHits != 0 {
				t.Fatalf("fallback hits = %d, want 0", fallbackHits)
			}
		})
	}
}

func TestFallbackTransport_PreservesNullResultFromFinalEndpoint(t *testing.T) {
	var primaryHits, fallbackHits int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":7,"result":null,"endpoint":"primary"}`)
	}))
	defer primary.Close()
	const fallbackBody = `{"jsonrpc":"2.0","id":7,"result":null,"endpoint":"fallback"}`
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fallbackBody)
	}))
	defer fallback.Close()

	payload := `{"jsonrpc":"2.0","id":7,"method":"eth_getTransactionReceipt","params":[]}`
	resp, err := roundTrip(t, mustEndpoints(t, primary.URL, fallback.URL), payload)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		t.Fatalf("read response: %v", readErr)
	}
	if string(body) != fallbackBody {
		t.Fatalf("response body = %s, want final fallback body %s", body, fallbackBody)
	}
	if primaryHits != 1 || fallbackHits != 1 {
		t.Fatalf("hits: primary=%d fallback=%d, want 1/1", primaryHits, fallbackHits)
	}
}

func TestFallbackTransport_EachReadCanSelectAHealthyEndpoint(t *testing.T) {
	var primaryHits, fallbackHits int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "eth_getTransactionReceipt") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `primary`)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackHits++
		_, _ = io.WriteString(w, `fallback`)
	}))
	defer fallback.Close()

	eps := mustEndpoints(t, primary.URL, fallback.URL)
	rt := &fallbackTransport{endpoints: eps, base: http.DefaultTransport, log: logr.Discard()}
	request := func(payload string) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, primary.URL, strings.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		_ = resp.Body.Close()
	}

	request(`{"method":"eth_getBlockByNumber"}`)
	request(`{"method":"eth_getTransactionReceipt"}`)
	request(`{"method":"eth_getBlockByNumber"}`)
	if primaryHits != 3 || fallbackHits != 1 {
		t.Fatalf("endpoint hits primary/fallback = %d/%d, want 3/1", primaryHits, fallbackHits)
	}
}

func TestFallbackTransport_AllFail(t *testing.T) {
	down := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
	}
	a, b := down(), down()
	defer a.Close()
	defer b.Close()

	resp, err := roundTrip(t, mustEndpoints(t, a.URL, b.URL), `{}`)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected an error when all endpoints fail")
	}
}

func TestFallbackTransport_ShortCallerDeadlineStillReachesFallback(t *testing.T) {
	releasePrimary := make(chan struct{})
	primary := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-releasePrimary:
		}
	}))
	defer func() {
		close(releasePrimary)
		primary.Close()
	}()
	var fallbackHits int
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackHits++
		_, _ = io.WriteString(w, `ok`)
	}))
	defer fallback.Close()

	eps := mustEndpoints(t, primary.URL, fallback.URL)
	rt := &fallbackTransport{endpoints: eps, base: http.DefaultTransport, log: logr.Discard()}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, primary.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || fallbackHits != 1 {
		t.Fatalf("status=%d fallbackHits=%d, want 200/1", resp.StatusCode, fallbackHits)
	}
}

func TestParseHTTPEndpoints_Dedups(t *testing.T) {
	eps, err := parseHTTPEndpoints([]string{
		"https://a.example", "https://b.example", "https://a.example", // dup of #1
	})
	if err != nil {
		t.Fatalf("parseHTTPEndpoints: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("got %d endpoints, want 2 (duplicate dropped)", len(eps))
	}
	if eps[0].String() != "https://a.example" || eps[1].String() != "https://b.example" {
		t.Fatalf("order/content wrong: %v", eps)
	}
}

func TestParseHTTPEndpoints_RejectsNonHTTP(t *testing.T) {
	if _, err := parseHTTPEndpoints([]string{"https://ok.example", "ws://node.example"}); err == nil {
		t.Fatal("expected ws:// to be rejected by the http(s)-only fallback")
	}
	if _, err := parseHTTPEndpoints([]string{"http://a.example", "https://b.example"}); err != nil {
		t.Fatalf("valid http(s) endpoints rejected: %v", err)
	}
}

func TestIsHTTPURL(t *testing.T) {
	cases := map[string]bool{
		"http://node.example":  true,
		"https://node.example": true,
		"ws://node.example":    false,
		"wss://node.example":   false,
		"/tmp/geth.ipc":        false,
		"":                     false,
	}
	for raw, want := range cases {
		if got := isHTTPURL(raw); got != want {
			t.Errorf("isHTTPURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

// TestDial_SingleHTTPEndpointServesChainID confirms a lone http(s) endpoint is dialed through the
// bounded fallback transport (not the timeout-less plain dial), so its RPC calls are time-bounded.
func TestDial_SingleHTTPEndpointServesChainID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x7a69"}`)) // 31337
	}))
	defer srv.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{srv.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial single http endpoint: %v", err)
	}
	defer c.Close()
	if got := c.ChainID().Uint64(); got != 31337 {
		t.Fatalf("chainID = %d, want 31337", got)
	}
}

// TestDial_FallbackServesChainID exercises the full wiring: a down primary and a JSON-RPC fallback
// that answers eth_chainId, so Dial succeeds via the fallback endpoint.
func TestDial_FallbackServesChainID(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x7a69"}`)) // 31337
	}))
	defer fallback.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{primary.URL, fallback.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial via fallback: %v", err)
	}
	defer c.Close()
	if got := c.ChainID().Uint64(); got != 31337 {
		t.Fatalf("chainID = %d, want 31337 (served by fallback)", got)
	}
}

func TestDial_FallbackServesReceiptAndHeadersAfterPrimaryNull(t *testing.T) {
	txHash := common.HexToHash("0x1234")
	blockHash := common.HexToHash("0x5678")
	receiptJSON, err := json.Marshal(&types.Receipt{
		Type:              types.DynamicFeeTxType,
		Status:            types.ReceiptStatusSuccessful,
		CumulativeGasUsed: 21_000,
		Logs:              []*types.Log{},
		TxHash:            txHash,
		GasUsed:           21_000,
		EffectiveGasPrice: big.NewInt(1),
		BlockHash:         blockHash,
		BlockNumber:       big.NewInt(42),
	})
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	headerJSON, err := json.Marshal(&types.Header{
		ParentHash:  common.HexToHash("0x01"),
		UncleHash:   types.EmptyUncleHash,
		Root:        common.HexToHash("0x02"),
		TxHash:      types.EmptyTxsHash,
		ReceiptHash: types.EmptyReceiptsHash,
		Difficulty:  big.NewInt(0),
		Number:      big.NewInt(42),
		GasLimit:    30_000_000,
		Time:        1,
		Extra:       []byte{},
	})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	var primaryMethods, fallbackMethods []string
	primary := rpcRecorder(&primaryMethods, func(method string) string {
		if method == "eth_chainId" {
			return `"0x7a69"`
		}
		return "null"
	})
	defer primary.Close()
	fallback := rpcRecorder(&fallbackMethods, func(method string) string {
		if method == "eth_getTransactionReceipt" {
			return string(receiptJSON)
		}
		return string(headerJSON)
	})
	defer fallback.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{primary.URL, fallback.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	receipt, err := c.TransactionReceipt(t.Context(), txHash)
	if err != nil {
		t.Fatalf("TransactionReceipt: %v", err)
	}
	if receipt.TxHash != txHash || receipt.BlockHash != blockHash {
		t.Fatalf("receipt hashes = tx %s block %s, want %s/%s", receipt.TxHash, receipt.BlockHash, txHash, blockHash)
	}
	headerByHash, err := c.HeaderByHash(t.Context(), blockHash)
	if err != nil {
		t.Fatalf("HeaderByHash: %v", err)
	}
	if headerByHash.Number.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("HeaderByHash number = %s, want 42", headerByHash.Number)
	}
	headerByNumber, err := c.HeaderByNumber(t.Context(), big.NewInt(42))
	if err != nil {
		t.Fatalf("HeaderByNumber: %v", err)
	}
	if headerByNumber.Number.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("HeaderByNumber number = %s, want 42", headerByNumber.Number)
	}

	availabilityMethods := []string{
		"eth_getTransactionReceipt",
		"eth_getBlockByHash",
		"eth_getBlockByNumber",
	}
	for _, method := range availabilityMethods {
		if got := countMethod(primaryMethods, method); got != 1 {
			t.Errorf("primary %s hits = %d, want 1", method, got)
		}
		if got := countMethod(fallbackMethods, method); got != 1 {
			t.Errorf("fallback %s hits = %d, want 1", method, got)
		}
	}
}

func TestDial_FinalNullReceiptReturnsNotFound(t *testing.T) {
	var primaryMethods, fallbackMethods []string
	primary := rpcRecorder(&primaryMethods, func(method string) string {
		if method == "eth_chainId" {
			return `"0x7a69"`
		}
		return "null"
	})
	defer primary.Close()
	fallback := rpcRecorder(&fallbackMethods, func(string) string { return "null" })
	defer fallback.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{primary.URL, fallback.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if receipt, receiptErr := c.TransactionReceipt(t.Context(), common.HexToHash("0x1234")); receipt != nil ||
		!errors.Is(receiptErr, ethereum.NotFound) {
		t.Fatalf("TransactionReceipt = (%v, %v), want nil/ethereum.NotFound", receipt, receiptErr)
	}
	if got := countMethod(primaryMethods, "eth_getTransactionReceipt"); got != 1 {
		t.Fatalf("primary receipt hits = %d, want 1", got)
	}
	if got := countMethod(fallbackMethods, "eth_getTransactionReceipt"); got != 1 {
		t.Fatalf("fallback receipt hits = %d, want 1", got)
	}
}

func countMethod(methods []string, want string) int {
	var count int
	for _, method := range methods {
		if method == want {
			count++
		}
	}
	return count
}

// rpcRecorder is a JSON-RPC httptest server that records the methods it is asked and replies with a
// canned result per method.
func rpcRecorder(methods *[]string, result func(method string) string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		*methods = append(*methods, req.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + result(req.Method) + `}`))
	}))
}

// TestDial_WriteRPCRoutesBroadcastsAndNonces confirms a separate writeRpcUrl carries the
// transaction broadcast, sender telemetry, and both startup nonce reads. The write endpoint's chain id is
// validated once; block number and every other read stay on the primary endpoint. This is the mevblocker-style split:
// private submissions and their nonce view share one endpoint while ordinary reads use a normal RPC.
func TestDial_WriteRPCRoutesBroadcastsAndNonces(t *testing.T) {
	var readMethods, writeMethods []string
	read := rpcRecorder(&readMethods, func(m string) string {
		if m == "eth_chainId" {
			return `"0x7a69"` // 31337
		}
		return `"0x1"`
	})
	defer read.Close()
	write := rpcRecorder(&writeMethods, func(m string) string {
		if m == "eth_chainId" {
			return `"0x7a69"`
		}
		if m == rpcMethodGetTransactionCount {
			return `"0x2"`
		}
		if m == "eth_getBalance" {
			return `"0x3"`
		}
		return `"0x0000000000000000000000000000000000000000000000000000000000000001"`
	})
	defer write.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	rpcMetrics, err := NewRPCMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	c, err := DialWithMetrics(
		t.Context(), []string{read.URL}, write.URL, multicall, rpcMetrics, logr.Discard(),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	// A read hits the primary endpoint.
	if _, err := c.BlockNumber(t.Context()); err != nil {
		t.Fatalf("BlockNumber: %v", err)
	}
	// The pending nonce hits the write endpoint so it observes private transactions.
	if nonce, err := c.PendingNonceAt(t.Context(), common.Address{}); err != nil {
		t.Fatalf("PendingNonceAt: %v", err)
	} else if nonce != 2 {
		t.Fatalf("PendingNonceAt = %d, want 2", nonce)
	}
	if nonce, err := c.NonceAt(t.Context(), common.Address{}, nil); err != nil {
		t.Fatalf("NonceAt: %v", err)
	} else if nonce != 2 {
		t.Fatalf("NonceAt = %d, want 2", nonce)
	}
	if balance, err := c.TransactionSenderBalanceAt(t.Context(), common.Address{}, nil); err != nil {
		t.Fatalf("TransactionSenderBalanceAt: %v", err)
	} else if balance.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("TransactionSenderBalanceAt = %s, want 3", balance)
	}
	// A broadcast hits the write endpoint only.
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(31337),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1),
		Gas:       21000,
		Value:     big.NewInt(0),
	})
	if err := c.SendTransaction(t.Context(), tx); err != nil {
		t.Fatalf("SendTransaction: %v", err)
	}

	if !slices.Contains(writeMethods, "eth_sendRawTransaction") {
		t.Fatalf("write endpoint did not receive the broadcast, saw: %v", writeMethods)
	}
	if !slices.Contains(writeMethods, rpcMethodGetTransactionCount) {
		t.Fatalf("write endpoint did not receive startup nonce reads, saw: %v", writeMethods)
	}
	if !slices.Contains(writeMethods, "eth_getBalance") {
		t.Fatalf("write endpoint did not receive the sender balance read, saw: %v", writeMethods)
	}
	if !slices.Contains(writeMethods, "eth_chainId") {
		t.Fatalf("write endpoint chain id was not validated, saw: %v", writeMethods)
	}
	if slices.Contains(writeMethods, "eth_blockNumber") {
		t.Fatalf("reads leaked onto the write endpoint: %v", writeMethods)
	}
	if slices.Contains(readMethods, rpcMethodSendRawTransaction) ||
		slices.Contains(readMethods, rpcMethodGetTransactionCount) ||
		slices.Contains(readMethods, "eth_getBalance") {
		t.Fatalf("write-side operation leaked onto the read endpoint: %v", readMethods)
	}
	if !slices.Contains(readMethods, "eth_blockNumber") {
		t.Fatalf("read endpoint did not receive the read, saw: %v", readMethods)
	}
	metricstest.RequireValue(t, rpcMetrics.requests.WithLabelValues(
		rpcRoleWrite, "eth_getBalance", string(rpcOutcomeSuccess),
	), 1)
	metricstest.RequireValue(t, rpcMetrics.requests.WithLabelValues(
		rpcRoleRead, "eth_blockNumber", string(rpcOutcomeSuccess),
	), 1)
}

func TestDial_RejectsMismatchedWriteRPCChainID(t *testing.T) {
	var readMethods, writeMethods []string
	read := rpcRecorder(&readMethods, func(string) string { return `"0x7a69"` })
	defer read.Close()
	write := rpcRecorder(&writeMethods, func(string) string { return `"0x1"` })
	defer write.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{read.URL}, write.URL, multicall, logr.Discard())
	if c != nil || err == nil || !strings.Contains(err.Error(), "write rpc chain id mismatch") {
		t.Fatalf("Dial mismatch result = (%v, %v)", c, err)
	}
	if !slices.Contains(readMethods, "eth_chainId") || !slices.Contains(writeMethods, "eth_chainId") {
		t.Fatalf("chain id calls read/write = %v/%v", readMethods, writeMethods)
	}
}

func TestDial_BroadcastDoesNotFallBackAcrossReadEndpoints(t *testing.T) {
	var primaryBroadcasts, fallbackBroadcasts, primaryPendingReads, fallbackPendingReads int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		if req.Method == "eth_sendRawTransaction" {
			primaryBroadcasts++
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if req.Method == "eth_getTransactionCount" {
			primaryPendingReads++
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x7a69"}`))
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		if req.Method == "eth_sendRawTransaction" {
			fallbackBroadcasts++
		}
		if req.Method == "eth_getTransactionCount" {
			fallbackPendingReads++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x1"}`))
	}))
	defer fallback.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{primary.URL, fallback.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(31337), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1),
		Gas: 21_000, Value: big.NewInt(0),
	})
	if err := c.SendTransaction(t.Context(), tx); err == nil {
		t.Fatal("expected the isolated primary write endpoint failure")
	}
	if _, err := c.PendingNonceAt(t.Context(), common.Address{}); err == nil {
		t.Fatal("expected the isolated primary pending-nonce failure")
	}
	if primaryBroadcasts != 1 || fallbackBroadcasts != 0 {
		t.Fatalf(
			"broadcasts primary/fallback = %d/%d, want 1/0",
			primaryBroadcasts, fallbackBroadcasts,
		)
	}
	if primaryPendingReads != 1 || fallbackPendingReads != 0 {
		t.Fatalf(
			"pending reads primary/fallback = %d/%d, want 1/0",
			primaryPendingReads, fallbackPendingReads,
		)
	}
}

func TestMulticallUsesLatestBlockTag(t *testing.T) {
	var callParams []json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		result := `"0x7a69"`
		if req.Method == "eth_call" {
			callParams = req.Params
			// ABI encoding of an empty aggregate3 Result[] return.
			result = `"0x0000000000000000000000000000000000000000000000000000000000000020` +
				`0000000000000000000000000000000000000000000000000000000000000000"`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + result + `}`))
	}))
	defer server.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{server.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if _, err = c.Multicall(t.Context(), nil); err != nil {
		t.Fatalf("Multicall: %v", err)
	}
	if len(callParams) != 2 {
		t.Fatalf("eth_call params = %s", callParams)
	}
	var blockTag string
	if err = json.Unmarshal(callParams[1], &blockTag); err != nil || blockTag != "latest" {
		t.Fatalf("eth_call block tag = %q, err=%v", blockTag, err)
	}
}

// TestDial_NoWriteRPCReusesPrimary confirms that with no writeRpcUrl, broadcasts fall back to the
// primary endpoint (unchanged behaviour).
func TestDial_NoWriteRPCReusesPrimary(t *testing.T) {
	var methods []string
	srv := rpcRecorder(&methods, func(m string) string {
		if m == "eth_chainId" {
			return `"0x7a69"`
		}
		return `"0x0000000000000000000000000000000000000000000000000000000000000001"`
	})
	defer srv.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{srv.URL}, "", multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(31337), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Gas: 21000, Value: big.NewInt(0)})
	if err := c.SendTransaction(t.Context(), tx); err != nil {
		t.Fatalf("SendTransaction: %v", err)
	}
	if !slices.Contains(methods, "eth_sendRawTransaction") {
		t.Fatalf("primary endpoint did not receive the broadcast, saw: %v", methods)
	}
}
