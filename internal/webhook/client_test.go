package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func testYAMLNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	if len(node.Content) == 0 {
		return yaml.Node{}
	}
	return *node.Content[0]
}

func TestParseConfigKeepsHeaderEnvIndirect(t *testing.T) {
	cfg, err := ParseConfig(testYAMLNode(t, `
url: https://strategy.example
timeout: 250ms
maxRequestBytes: 2048
maxResponseBytes: 4096
headers:
  x-client:
    value: vault-solver
  authorization:
    env: STRATEGY_AUTH_HEADER
`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.URL != "https://strategy.example" || cfg.Timeout != 250*time.Millisecond {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.MaxRequestBytes != 2048 || cfg.MaxResponseBytes != 4096 {
		t.Fatalf("unexpected limits: request=%d response=%d", cfg.MaxRequestBytes, cfg.MaxResponseBytes)
	}
	if cfg.Headers["x-client"].Value != "vault-solver" || cfg.Headers["authorization"].Env != "STRATEGY_AUTH_HEADER" {
		t.Fatalf("unexpected headers: %+v", cfg.Headers)
	}
}

func TestParseConfigRejectsUnknownFields(t *testing.T) {
	_, err := ParseConfig(testYAMLNode(t, `
url: https://strategy.example
extra: true
`))
	if err == nil || !strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("ParseConfig error = %v, want unknown field", err)
	}
}

func TestParseConfigRejectsInvalidHeaders(t *testing.T) {
	cases := map[string]string{
		"empty name": `
url: https://strategy.example
headers:
  "":
    value: bad
`,
		"empty value": `
url: https://strategy.example
headers:
  x-client: {}
`,
		"value and env": `
url: https://strategy.example
headers:
  x-client:
    value: vault-solver
    env: STRATEGY_AUTH_HEADER
`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig(testYAMLNode(t, raw)); err == nil {
				t.Fatal("expected ParseConfig to reject header config")
			}
		})
	}
}

func TestParseConfigRejectsInvalidByteLimits(t *testing.T) {
	for name, raw := range map[string]string{
		"request":  "maxRequestBytes: -1",
		"response": "maxResponseBytes: -1",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseConfig(testYAMLNode(t, "url: https://strategy.example\n"+raw))
			if err == nil {
				t.Fatal("expected invalid byte limit to be rejected")
			}
		})
	}
}

func TestNewClientConfig(t *testing.T) {
	t.Setenv("STRATEGY_AUTH_HEADER", "Bearer test")
	_, err := NewClient(Config{URL: "http://strategy.example"})
	if err == nil {
		t.Fatal("expected non-loopback http url to be rejected")
	}

	client, err := NewClient(Config{
		URL:              "https://strategy.example",
		Timeout:          250 * time.Millisecond,
		MaxRequestBytes:  2048,
		MaxResponseBytes: 4096,
		Headers: map[string]HeaderValue{
			"x-client":      {Value: "vault-solver"},
			"authorization": {Env: "STRATEGY_AUTH_HEADER"},
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.client.Timeout != 250*time.Millisecond || client.maxRequestBytes != 2048 || client.maxResponseBytes != 4096 {
		t.Fatalf("unexpected client config: timeout=%v request=%d response=%d",
			client.client.Timeout, client.maxRequestBytes, client.maxResponseBytes)
	}
	if client.headers["x-client"] != "vault-solver" || client.headers["authorization"] != "Bearer test" {
		t.Fatalf("unexpected headers: %+v", client.headers)
	}
}

func TestNewClientRejectsEmptyHeaderEnv(t *testing.T) {
	t.Setenv("STRATEGY_AUTH_HEADER", "")
	_, err := NewClient(Config{
		URL: "https://strategy.example",
		Headers: map[string]HeaderValue{
			"authorization": {Env: "STRATEGY_AUTH_HEADER"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "env \"STRATEGY_AUTH_HEADER\" is empty") {
		t.Fatalf("NewClient error = %v, want empty env", err)
	}
}

func TestNewClientRejectsInvalidByteLimits(t *testing.T) {
	for _, tc := range []Config{
		{URL: "https://strategy.example", MaxRequestBytes: -1},
		{URL: "https://strategy.example", MaxResponseBytes: -1},
	} {
		if _, err := NewClient(tc); err == nil {
			t.Fatalf("expected invalid byte limit to be rejected: %+v", tc)
		}
	}
}

func TestNewClientAllowsLoopbackHTTP(t *testing.T) {
	for _, rawURL := range []string{
		"http://localhost:8080/strategy",
		"http://127.0.0.1:8080/strategy",
		"http://[::1]:8080/strategy",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := NewClient(Config{URL: rawURL}); err != nil {
				t.Fatalf("NewClient: %v", err)
			}
		})
	}
}

func TestWebhookClientDoJSONPostRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/strategy/quote" {
			t.Fatalf("path = %q, want /strategy/quote", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		if got := r.Header.Get("x-client"); got != "vault-solver" {
			t.Fatalf("x-client = %q, want vault-solver", got)
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.ID != "q1" {
			t.Fatalf("request id = %q, want q1", req.ID)
		}
		_, _ = w.Write([]byte(`{"decision":"quote"}`))
	}))
	defer srv.Close()

	client, err := NewClient(Config{
		URL:     srv.URL + "/strategy",
		Timeout: time.Second,
		Headers: map[string]HeaderValue{
			"x-client": {Value: "vault-solver"},
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var resp struct {
		Decision string `json:"decision"`
	}
	if err := client.DoJSON(t.Context(), http.MethodPost, "/quote", struct {
		ID string `json:"id"`
	}{ID: "q1"}, &resp); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if resp.Decision != "quote" {
		t.Fatalf("decision = %q, want quote", resp.Decision)
	}
}

func TestWebhookClientGetJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/strategy/callbacks" {
			t.Fatalf("path = %q, want /strategy/callbacks", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Fatalf("content-type = %q, want unset", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("accept = %q, want application/json", got)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	client, err := NewClient(Config{URL: srv.URL + "/strategy", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := client.GetJSON(t.Context(), "callbacks", &resp); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want ok", resp.Status)
	}
}

func TestWebhookClientRejectsAbsoluteRoute(t *testing.T) {
	client, err := NewClient(Config{URL: "https://strategy.example/base", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var resp struct{}
	err = client.GetJSON(t.Context(), "https://other.example/callbacks", &resp)
	if err == nil || !strings.Contains(err.Error(), "route must be relative") {
		t.Fatalf("GetJSON error = %v, want route must be relative", err)
	}
}

func TestWebhookClientPostJSONFailures(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"non-2xx":       {status: http.StatusInternalServerError, body: "boom", want: "status 500"},
		"empty body":    {status: http.StatusOK, body: " \n", want: "empty response"},
		"unknown field": {status: http.StatusOK, body: `{"decision":"quote","extra":1}`, want: "unknown field"},
		"trailing json": {status: http.StatusOK, body: `{"decision":"quote"}{"decision":"decline"}`, want: "multiple JSON values"},
		"too large": {
			status: http.StatusOK,
			body:   strings.Repeat("x", defaultWebhookMaxBodyBytes+1),
			want:   "response body exceeds",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client, err := NewClient(Config{URL: srv.URL, Timeout: time.Second})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			var resp struct {
				Decision string `json:"decision"`
			}
			err = client.PostJSON(t.Context(), struct{}{}, &resp)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("PostJSON error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestWebhookClientRejectsOversizedRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"decision":"quote"}`))
	}))
	defer srv.Close()

	client, err := NewClient(Config{
		URL:             srv.URL,
		Timeout:         time.Second,
		MaxRequestBytes: 8,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var resp struct {
		Decision string `json:"decision"`
	}
	err = client.PostJSON(t.Context(), struct {
		Payload string `json:"payload"`
	}{Payload: "larger-than-limit"}, &resp)
	if err == nil || !strings.Contains(err.Error(), "request body exceeds") {
		t.Fatalf("PostJSON error = %v, want request body exceeds", err)
	}
	if called {
		t.Fatal("server was called for an oversized request")
	}
}

func TestWebhookClientRejectsNilResponseTarget(t *testing.T) {
	client, err := NewClient(Config{URL: "https://strategy.example", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = client.GetJSON(t.Context(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "response target is nil") {
		t.Fatalf("GetJSON error = %v, want response target is nil", err)
	}
}
