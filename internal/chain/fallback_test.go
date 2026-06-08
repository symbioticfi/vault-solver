package chain

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	c, err := Dial(t.Context(), []string{primary.URL, fallback.URL}, multicall, logr.Discard())
	if err != nil {
		t.Fatalf("Dial via fallback: %v", err)
	}
	defer c.Close()
	if got := c.ChainID().Uint64(); got != 31337 {
		t.Fatalf("chainID = %d, want 31337 (served by fallback)", got)
	}
}
