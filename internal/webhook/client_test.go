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

func testYAMLNode(t *testing.T, body string) yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	return *doc.Content[0]
}

func TestParseConfigAndResolveHeaders(t *testing.T) {
	t.Setenv("TEST_AUTH_HEADER", "Bearer test")
	_, err := ParseConfig(testYAMLNode(t, `
url: http://strategy.example
timeout: 250ms
headers:
  x-client:
    value: vault-solver
  authorization:
    env: TEST_AUTH_HEADER
`))
	if err == nil {
		t.Fatal("expected non-loopback http url to be rejected")
	}
	cfg, err := ParseConfig(testYAMLNode(t, `
url: https://strategy.example
timeout: 250ms
maxRequestBytes: 2048
maxResponseBytes: 4096
headers:
  x-client:
    value: vault-solver
  authorization:
    env: TEST_AUTH_HEADER
`))
	if err != nil {
		t.Fatalf("ParseConfig https: %v", err)
	}
	if cfg.URL != "https://strategy.example" || cfg.Timeout != 250*time.Millisecond {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.MaxRequestBytes != 2048 || cfg.MaxResponseBytes != 4096 {
		t.Fatalf("unexpected byte limits: %+v", cfg)
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.headers["x-client"] != "vault-solver" || client.headers["authorization"] != "Bearer test" {
		t.Fatalf("unexpected headers: %+v", client.headers)
	}
}

func TestParseConfigRejectsUnknownFields(t *testing.T) {
	_, err := ParseConfig(testYAMLNode(t, `
url: https://strategy.example
retries: 3
`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestParseConfigRejectsInvalidByteLimits(t *testing.T) {
	for _, field := range []string{"maxRequestBytes", "maxResponseBytes"} {
		t.Run(field, func(t *testing.T) {
			_, err := ParseConfig(testYAMLNode(t, "url: https://strategy.example\n"+field+": -1\n"))
			if err == nil {
				t.Fatalf("expected %s to be rejected", field)
			}
		})
	}
}

func TestParseConfigAllowsLoopbackHTTP(t *testing.T) {
	for _, rawURL := range []string{
		"http://localhost:8080/strategy",
		"http://127.0.0.1:8080/strategy",
		"http://[::1]:8080/strategy",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := ParseConfig(testYAMLNode(t, "url: "+rawURL+"\n"))
			if err != nil {
				t.Fatalf("ParseConfig: %v", err)
			}
		})
	}
}

func TestWebhookClientPostJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
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
		URL:     srv.URL,
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
	if err := client.PostJSON(t.Context(), struct {
		ID string `json:"id"`
	}{ID: "q1"}, &resp); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if resp.Decision != "quote" {
		t.Fatalf("decision = %q, want quote", resp.Decision)
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
