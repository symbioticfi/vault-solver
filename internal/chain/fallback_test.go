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
	"github.com/go-logr/logr/funcr"
)

const testMulticallAddress = "0xcA11bde05977b3631167028862bE2a173976CA11"

func captureLogger(logs *strings.Builder) logr.Logger {
	return funcr.NewJSON(func(obj string) {
		logs.WriteString(obj)
		logs.WriteByte('\n')
	}, funcr.Options{Verbosity: 1})
}

func assertEndpointSecretsRedacted(t *testing.T, err error, logs string, secrets ...string) {
	t.Helper()
	combined := logs
	if err != nil {
		combined += err.Error()
	}
	for _, secret := range secrets {
		if strings.Contains(combined, secret) {
			t.Fatalf("endpoint secret %q leaked: err=%v logs=%s", secret, err, logs)
		}
	}
}

func secretEndpoint(t *testing.T, base, marker string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse test endpoint: %v", err)
	}
	u.User = url.UserPassword("rpc-user-"+marker, "pw-"+marker)
	u.Path = "/route-" + marker
	u.RawQuery = "credential=query-" + marker
	u.Fragment = "fragment-" + marker
	return u.String()
}

func chainIDMethodCount(methods []string) int {
	var count int
	for _, method := range methods {
		if method == "eth_chainId" {
			count++
		}
	}
	return count
}

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
	return roundTripWithLogger(t, eps, payload, logr.Discard())
}

func roundTripWithLogger(
	t *testing.T,
	eps []*url.URL,
	payload string,
	log logr.Logger,
) (*http.Response, error) {
	t.Helper()
	rt := &fallbackTransport{endpoints: eps, base: http.DefaultTransport, log: log}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, eps[0].String(), strings.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return rt.RoundTrip(req)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type observedResponseBody struct {
	reader    *strings.Reader
	readCalls int
	closed    bool
}

func (b *observedResponseBody) Read(p []byte) (int, error) {
	b.readCalls++
	return b.reader.Read(p)
}

func (b *observedResponseBody) Close() error {
	b.closed = true
	return nil
}

func TestFallbackTransport_FallsOverOnRetryableStatus(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var primaryHits, fallbackHits int
			var gotBody string
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryHits++
				w.WriteHeader(status)
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
		})
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

func TestFallbackTransport_NonRetryableHTTPStatusSanitized(t *testing.T) {
	const (
		requestSecret  = "request-body-secret"
		responseSecret = "response-body-secret"
		locationSecret = "location-secret"
	)
	tests := []struct {
		name       string
		status     int
		wantStatus string
		location   string
	}{
		{
			name:       "unauthorized response body",
			status:     http.StatusUnauthorized,
			wantStatus: "HTTP 401",
		},
		{
			name:       "redirect location",
			status:     http.StatusFound,
			wantStatus: "HTTP 302",
			location:   "https://location-user:location-pass@redirect.example/route-" + locationSecret + "?token=" + locationSecret + "#fragment-" + locationSecret,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := mustEndpoints(t,
				"https://rpc-user-status:pw-status@rpc.example/route-status?credential=query-status#fragment-status",
			)[0]
			body := &observedResponseBody{
				reader: strings.NewReader(responseSecret + ": " + requestSecret),
			}
			attempts := 0
			base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts > 1 {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`)),
						Request:    req,
					}, nil
				}
				header := make(http.Header)
				if tt.location != "" {
					header.Set("Location", tt.location)
				}
				return &http.Response{
					StatusCode:    tt.status,
					Header:        header,
					Body:          body,
					ContentLength: int64(body.reader.Len()),
					Request:       req,
				}, nil
			})
			var logs strings.Builder
			client := &http.Client{Transport: &fallbackTransport{
				endpoints: []*url.URL{endpoint},
				base:      base,
				log:       captureLogger(&logs),
			}}
			req, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				endpointLabel(endpoint),
				strings.NewReader(requestSecret),
			)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}

			resp, err := client.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil {
				t.Fatalf("Do returned status response, want sanitized transport error")
			}
			if resp != nil {
				t.Fatalf("response = %#v, want nil so body and headers cannot escape", resp)
			}
			if attempts != 1 {
				t.Fatalf("transport attempts = %d, want 1 (non-retry status must not fall over or redirect)", attempts)
			}
			if !body.closed {
				t.Fatal("response body was not closed")
			}
			if body.readCalls != 0 {
				t.Fatalf("response body reads = %d, want 0", body.readCalls)
			}
			if !strings.Contains(err.Error(), "endpoint 1 (https://rpc.example)") ||
				!strings.Contains(err.Error(), tt.wantStatus) {
				t.Fatalf("error = %q, want safe endpoint ordinal/origin and HTTP status", err)
			}
			assertEndpointSecretsRedacted(t, err, logs.String(),
				"rpc-user-status", "pw-status", "route-status", "query-status", "fragment-status",
				requestSecret, responseSecret,
				"location-user", "location-pass", locationSecret,
			)
		})
	}
}

func TestFallbackTransport_HTTP200JSONRPCErrorPassesThrough(t *testing.T) {
	const rpcError = `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"execution reverted"}}`
	var fallbackHits int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, rpcError)
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
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(got) != rpcError || fallbackHits != 0 {
		t.Fatalf("status=%d body=%q fallbackHits=%d, want unchanged HTTP 200 JSON-RPC error and no fallback", resp.StatusCode, got, fallbackHits)
	}
}

func TestFallbackTransport_EndpointFailureRedacted(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	b := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	aURL, bURL := a.URL, b.URL
	a.Close()
	b.Close()

	rawA := secretEndpoint(t, aURL, "runtime-alpha")
	rawB := secretEndpoint(t, bURL, "runtime-beta")
	var logs strings.Builder
	resp, err := roundTripWithLogger(
		t,
		mustEndpoints(t, rawA, rawB),
		`{"jsonrpc":"2.0"}`,
		captureLogger(&logs),
	)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected an error when every endpoint is unreachable")
	}
	assertEndpointSecretsRedacted(t, err, logs.String(),
		"rpc-user-runtime-alpha", "pw-runtime-alpha", "route-runtime-alpha", "query-runtime-alpha", "fragment-runtime-alpha",
		"rpc-user-runtime-beta", "pw-runtime-beta", "route-runtime-beta", "query-runtime-beta", "fragment-runtime-beta",
	)
}

func TestDialClient_RuntimeEndpointFailureRedacted(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x1"}`))
	}))
	b := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rawA := secretEndpoint(t, a.URL, "runtime-client-alpha")
	rawB := secretEndpoint(t, b.URL, "runtime-client-beta")
	var logs strings.Builder
	c, err := dialClient(t.Context(), []string{rawA, rawB}, captureLogger(&logs))
	if err != nil {
		a.Close()
		b.Close()
		t.Fatalf("dialClient: %v", err)
	}
	a.Close()
	b.Close()
	defer c.Close()

	_, err = c.ChainID(t.Context())
	if err == nil {
		t.Fatal("expected a runtime error when every endpoint is unreachable")
	}
	assertEndpointSecretsRedacted(t, err, logs.String(),
		"rpc-user-runtime-client-alpha", "pw-runtime-client-alpha", "route-runtime-client-alpha", "query-runtime-client-alpha", "fragment-runtime-client-alpha",
		"rpc-user-runtime-client-beta", "pw-runtime-client-beta", "route-runtime-client-beta", "query-runtime-client-beta", "fragment-runtime-client-beta",
	)
}

func TestDial_PreservesEndpointBasicAuth(t *testing.T) {
	const (
		username = "rpc-user-auth"
		password = "pw-auth"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPassword, ok := r.BasicAuth()
		if !ok || gotUser != username || gotPassword != password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x1"}`))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test endpoint: %v", err)
	}
	u.User = url.UserPassword(username, password)

	c, err := Dial(t.Context(), []string{u.String()}, "", testMulticallAddress, 1, logr.Discard())
	if err != nil {
		t.Fatalf("Dial with endpoint basic auth: %v", err)
	}
	defer c.Close()
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

func TestParseHTTPEndpoints_EndpointErrorsRedacted(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantClass string
	}{
		{
			name:      "malformed",
			raw:       "http://rpc-user-parse:pw-parse@%zz/route-parse?credential=query-parse#fragment-parse",
			wantClass: "invalid endpoint",
		},
		{
			name:      "unsupported scheme",
			raw:       secretEndpoint(t, "ftp://node.example", "parse"),
			wantClass: "unsupported scheme",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseHTTPEndpoints([]string{"https://healthy.example", tt.raw})
			if err == nil {
				t.Fatal("expected endpoint validation error")
			}
			if !strings.Contains(err.Error(), "endpoint 2") || !strings.Contains(err.Error(), tt.wantClass) {
				t.Fatalf("error = %q, want safe ordinal and class %q", err, tt.wantClass)
			}
			assertEndpointSecretsRedacted(t, err, "",
				"rpc-user-parse", "pw-parse", "route-parse", "query-parse", "fragment-parse",
			)
		})
	}
}

func TestEndpointLabel_RedactsCredentialsAndRoute(t *testing.T) {
	u, err := url.Parse("https://rpc-user-label:pw-label@node.example:8545/route-label?credential=query-label#fragment-label")
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	if got, want := endpointLabel(u), "https://node.example:8545"; got != want {
		t.Fatalf("endpointLabel = %q, want %q", got, want)
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

	c, err := Dial(t.Context(), []string{srv.URL}, "", testMulticallAddress, 31337, logr.Discard())
	if err != nil {
		t.Fatalf("Dial single http endpoint: %v", err)
	}
	defer c.Close()
	if got := c.ChainID().Uint64(); got != 31337 {
		t.Fatalf("chainID = %d, want 31337", got)
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

func chainIDRecorder(methods *[]string, chainID string) *httptest.Server {
	return rpcRecorder(methods, func(method string) string {
		if method == "eth_chainId" {
			return `"` + chainID + `"`
		}
		return `"0x1"`
	})
}

func TestDial_PreflightsEveryEndpointChainID(t *testing.T) {
	t.Run("all distinct endpoints match", func(t *testing.T) {
		var primaryMethods, fallbackMethods, writeMethods []string
		primary := chainIDRecorder(&primaryMethods, "0x1")
		fallback := chainIDRecorder(&fallbackMethods, "0x1")
		write := chainIDRecorder(&writeMethods, "0x1")
		defer primary.Close()
		defer fallback.Close()
		defer write.Close()

		c, err := Dial(t.Context(), []string{
			secretEndpoint(t, primary.URL, "healthy-primary"),
			secretEndpoint(t, fallback.URL, "healthy-fallback"),
		}, secretEndpoint(t, write.URL, "healthy-write"), testMulticallAddress, 1, logr.Discard())
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer c.Close()
		if chainIDMethodCount(primaryMethods) != 1 ||
			chainIDMethodCount(fallbackMethods) != 1 ||
			chainIDMethodCount(writeMethods) != 1 {
			t.Fatalf("preflight methods: primary=%v fallback=%v write=%v", primaryMethods, fallbackMethods, writeMethods)
		}
		if got := c.ChainID().Uint64(); got != 1 {
			t.Fatalf("cached chain id = %d, want 1", got)
		}
	})

	t.Run("wrong fallback rejected", func(t *testing.T) {
		var primaryMethods, fallbackMethods, writeMethods []string
		primary := chainIDRecorder(&primaryMethods, "0x1")
		fallback := chainIDRecorder(&fallbackMethods, "0x2")
		write := chainIDRecorder(&writeMethods, "0x1")
		defer primary.Close()
		defer fallback.Close()
		defer write.Close()

		var logs strings.Builder
		_, err := Dial(t.Context(), []string{
			secretEndpoint(t, primary.URL, "wrong-fallback-primary"),
			secretEndpoint(t, fallback.URL, "wrong-fallback-secondary"),
		}, secretEndpoint(t, write.URL, "wrong-fallback-write"), testMulticallAddress, 1, captureLogger(&logs))
		if err == nil {
			t.Fatal("expected wrong-chain fallback rejection")
		}
		if !strings.Contains(err.Error(), "rpc endpoint 2") || !strings.Contains(err.Error(), "got 2, want 1") {
			t.Fatalf("error = %q, want safe fallback ordinal and mismatch", err)
		}
		assertEndpointSecretsRedacted(t, err, logs.String(),
			"rpc-user-wrong-fallback-primary", "pw-wrong-fallback-primary", "route-wrong-fallback-primary", "query-wrong-fallback-primary", "fragment-wrong-fallback-primary",
			"rpc-user-wrong-fallback-secondary", "pw-wrong-fallback-secondary", "route-wrong-fallback-secondary", "query-wrong-fallback-secondary", "fragment-wrong-fallback-secondary",
			"rpc-user-wrong-fallback-write", "pw-wrong-fallback-write", "route-wrong-fallback-write", "query-wrong-fallback-write", "fragment-wrong-fallback-write",
		)
	})

	t.Run("wrong write endpoint rejected", func(t *testing.T) {
		var primaryMethods, writeMethods []string
		primary := chainIDRecorder(&primaryMethods, "0x1")
		write := chainIDRecorder(&writeMethods, "0x2")
		defer primary.Close()
		defer write.Close()

		var logs strings.Builder
		_, err := Dial(t.Context(), []string{
			secretEndpoint(t, primary.URL, "wrong-write-primary"),
		}, secretEndpoint(t, write.URL, "wrong-write-relay"), testMulticallAddress, 1, captureLogger(&logs))
		if err == nil {
			t.Fatal("expected wrong-chain write endpoint rejection")
		}
		if !strings.Contains(err.Error(), "write rpc endpoint 1") || !strings.Contains(err.Error(), "got 2, want 1") {
			t.Fatalf("error = %q, want safe write endpoint ordinal and mismatch", err)
		}
		assertEndpointSecretsRedacted(t, err, logs.String(),
			"rpc-user-wrong-write-primary", "pw-wrong-write-primary", "route-wrong-write-primary", "query-wrong-write-primary", "fragment-wrong-write-primary",
			"rpc-user-wrong-write-relay", "pw-wrong-write-relay", "route-wrong-write-relay", "query-wrong-write-relay", "fragment-wrong-write-relay",
		)
	})

	t.Run("duplicate raw endpoints preflight once", func(t *testing.T) {
		var methods []string
		srv := chainIDRecorder(&methods, "0x1")
		defer srv.Close()
		raw := secretEndpoint(t, srv.URL, "duplicate")

		c, err := Dial(t.Context(), []string{raw, raw}, raw, testMulticallAddress, 1, logr.Discard())
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer c.Close()
		if got := chainIDMethodCount(methods); got != 1 {
			t.Fatalf("eth_chainId requests = %d, want 1 for one distinct raw endpoint; methods=%v", got, methods)
		}
	})
}

func TestDial_EndpointErrorsRedacted(t *testing.T) {
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachableURL := unreachable.URL
	unreachable.Close()

	tests := []struct {
		name      string
		raw       string
		wantClass string
		secrets   []string
	}{
		{
			name:      "unreachable credential-bearing URL",
			raw:       secretEndpoint(t, unreachableURL, "dial-unreachable"),
			wantClass: "chain-id request failed",
			secrets: []string{
				"rpc-user-dial-unreachable", "pw-dial-unreachable", "route-dial-unreachable", "query-dial-unreachable", "fragment-dial-unreachable",
			},
		},
		{
			name:      "malformed credential-bearing URL",
			raw:       "http://rpc-user-dial-malformed:pw-dial-malformed@%zz/route-dial-malformed?credential=query-dial-malformed#fragment-dial-malformed",
			wantClass: "invalid endpoint",
			secrets: []string{
				"rpc-user-dial-malformed", "pw-dial-malformed", "route-dial-malformed", "query-dial-malformed", "fragment-dial-malformed",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs strings.Builder
			_, err := Dial(t.Context(), []string{tt.raw}, "", testMulticallAddress, 1, captureLogger(&logs))
			if err == nil {
				t.Fatal("expected endpoint rejection")
			}
			if !strings.Contains(err.Error(), "rpc endpoint 1") || !strings.Contains(err.Error(), tt.wantClass) {
				t.Fatalf("error = %q, want safe endpoint ordinal and class %q", err, tt.wantClass)
			}
			assertEndpointSecretsRedacted(t, err, logs.String(), tt.secrets...)
		})
	}
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
	write := rpcRecorder(&writeMethods, func(method string) string {
		if method == "eth_chainId" {
			return `"0x7a69"`
		}
		return `"0x0000000000000000000000000000000000000000000000000000000000000001"`
	})
	defer write.Close()

	c, err := Dial(t.Context(), []string{read.URL}, write.URL, testMulticallAddress, 31337, logr.Discard())
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
	// Startup deliberately preflights eth_chainId on the write endpoint. Operational reads must not
	// be routed there.
	if slices.Contains(writeMethods, "eth_blockNumber") {
		t.Fatalf("reads leaked onto the write endpoint: %v", writeMethods)
	}
	if slices.Contains(readMethods, "eth_sendRawTransaction") {
		t.Fatalf("broadcast leaked onto the read endpoint: %v", readMethods)
	}
	if !slices.Contains(readMethods, "eth_blockNumber") {
		t.Fatalf("read endpoint did not receive the read, saw: %v", readMethods)
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

	c, err := Dial(t.Context(), []string{srv.URL}, "", testMulticallAddress, 31337, logr.Discard())
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
