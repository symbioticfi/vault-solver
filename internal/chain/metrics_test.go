package chain

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestRPCMetricsClassifyFallbackAndRPCError(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), rpcMethodGetTransactionReceipt) {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":null}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"id":2`) {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"reverted"}}`)
			return
		}
		if strings.Contains(string(body), `"id":3`) {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":{"blockNumber":"0x1"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`)
	}))
	defer fallback.Close()

	metrics, err := NewRPCMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	endpoints := mustEndpoints(t, primary.URL, fallback.URL)
	metrics.bindTransport(rpcRoleRead, len(endpoints))
	transport := &fallbackTransport{
		endpoints: endpoints, base: http.DefaultTransport, metrics: metrics, role: rpcRoleRead, log: logr.Discard(),
	}
	for _, payload := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"eth_call","params":[]}`,
		`{"jsonrpc":"2.0","id":2,"method":"eth_call","params":[]}`,
		`{"jsonrpc":"2.0","id":3,"method":"eth_getTransactionReceipt","params":[]}`,
	} {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, primary.URL, strings.NewReader(payload))
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		resp, roundTripErr := transport.RoundTrip(req)
		if roundTripErr != nil {
			t.Fatalf("round trip: %v", roundTripErr)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	metricstest.RequireValue(t, metrics.requests.WithLabelValues(rpcRoleRead, "eth_call", string(rpcOutcomeSuccess)), 1)
	metricstest.RequireValue(t, metrics.requests.WithLabelValues(rpcRoleRead, "eth_call", string(rpcOutcomeRPCError)), 1)
	metricstest.RequireValue(t, metrics.attempts.WithLabelValues(
		rpcRoleRead, "0", "eth_call", string(rpcOutcomeHTTP5xx),
	), 2)
	metricstest.RequireValue(t, metrics.attempts.WithLabelValues(
		rpcRoleRead, "1", "eth_call", string(rpcOutcomeSuccess),
	), 1)
	metricstest.RequireValue(t, metrics.attempts.WithLabelValues(
		rpcRoleRead, "1", "eth_call", string(rpcOutcomeRPCError),
	), 1)
	metricstest.RequireValue(t, metrics.attempts.WithLabelValues(
		rpcRoleRead, "0", rpcMethodGetTransactionReceipt, string(rpcOutcomeNullResult),
	), 1)
	metricstest.RequireValue(t, metrics.requests.WithLabelValues(
		rpcRoleRead, rpcMethodGetTransactionReceipt, string(rpcOutcomeSuccess),
	), 1)
	metricstest.RequireValue(t, metrics.inflight.WithLabelValues(rpcRoleRead), 0)
	if got := testutil.ToFloat64(metrics.lastSuccessfulAttempt.WithLabelValues(rpcRoleRead, "1")); got <= 0 {
		t.Fatalf("last successful attempt = %v, want timestamp", got)
	}
}

func TestBoundedRPCMethod(t *testing.T) {
	for _, test := range []struct {
		body string
		want string
	}{
		{`{"jsonrpc":"2.0","method":"eth_getBalance"}`, "eth_getBalance"},
		{`{"jsonrpc":"2.0","method":"custom_user_value"}`, "other"},
		{`[{"jsonrpc":"2.0","method":"eth_call"}]`, "batch"},
		{`not-json`, "unknown"},
	} {
		if got := boundedRPCMethod([]byte(test.body)); got != test.want {
			t.Errorf("boundedRPCMethod(%q) = %q, want %q", test.body, got, test.want)
		}
	}
}
