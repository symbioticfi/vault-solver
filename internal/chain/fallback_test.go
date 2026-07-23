package chain

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
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

// TestDial_WriteRPCRoutesOnlyBroadcasts confirms a separate writeRpcUrl carries ONLY the transaction
// broadcast (eth_sendRawTransaction); chain id, block number, and every other read stay on the
// primary endpoint. This is the mevblocker-style split: submit fills privately, read from a normal RPC.
func TestDial_WriteRPCRoutesOnlyBroadcasts(t *testing.T) {
	var readMethods, writeMethods []string
	read := rpcRecorder(&readMethods, func(m string) string {
		if m == "eth_chainId" {
			return `"0x7a69"` // 31337
		}
		return `"0x1"`
	})
	defer read.Close()
	write := rpcRecorder(&writeMethods, func(string) string {
		return `"0x0000000000000000000000000000000000000000000000000000000000000001"`
	})
	defer write.Close()

	const multicall = "0xcA11bde05977b3631167028862bE2a173976CA11"
	c, err := Dial(t.Context(), []string{read.URL}, write.URL, multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	// A read hits the primary endpoint.
	if _, err := c.BlockNumber(t.Context()); err != nil {
		t.Fatalf("BlockNumber: %v", err)
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
	if slices.Contains(writeMethods, "eth_chainId") || slices.Contains(writeMethods, "eth_blockNumber") {
		t.Fatalf("reads leaked onto the write endpoint: %v", writeMethods)
	}
	if slices.Contains(readMethods, "eth_sendRawTransaction") {
		t.Fatalf("broadcast leaked onto the read endpoint: %v", readMethods)
	}
	if !slices.Contains(readMethods, "eth_blockNumber") {
		t.Fatalf("read endpoint did not receive the read, saw: %v", readMethods)
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
