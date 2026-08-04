package rfq

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetrics_InstrumentRecordsRequest(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newHTTPMetrics(reg)
	if err != nil {
		t.Fatalf("newHTTPMetrics: %v", err)
	}

	h := m.instrument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	got := testutil.ToFloat64(m.requests.With(prometheus.Labels{
		"method": "POST", "route": "/quote", "status": "204",
	}))
	if got != 1 {
		t.Fatalf("rfq_filler_http_requests_total{POST,/quote,204} = %v, want 1", got)
	}
}

func TestRouteLabel_BoundsCardinality(t *testing.T) {
	for path, want := range map[string]string{
		"/quote":        "/quote",
		"/swap":         "/swap",
		"/health":       "/health",
		"/openapi.json": "/openapi.json",
		"/random/path":  "other",
		"/":             "other",
	} {
		if got := routeLabel(path); got != want {
			t.Fatalf("routeLabel(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestHTTPMetricsRecordsSwapRouteAndBoundedPhaseOutcome(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newHTTPMetrics(reg)
	if err != nil {
		t.Fatalf("newHTTPMetrics: %v", err)
	}
	srv := testServer()
	srv.metrics = m
	srv.swaps = &fakeSwapHandler{response: &swapResponse{
		Protocol: swapProtocolV2, Phase: swapPhaseDiscovery,
		RequestID: "33333333-3333-4333-8333-333333333333",
		QuoteID:   "44444444-4444-4444-8444-444444444444",
		ChainID:   1,
		Swapper:   "0x0000000000000000000000000000000000000099",
		TokenIn:   tIn.Hex(), TokenOut: tOut.Hex(),
		Points: &[]swapPointResponse{},
	}}

	if rr := do(t, srv.handler(), http.MethodPost, "/swap", testSecret, validSwapDiscoveryBody()); rr.Code != http.StatusOK {
		t.Fatalf("/swap = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	if got := testutil.ToFloat64(m.requests.With(prometheus.Labels{
		"method": "POST", "route": "/swap", "status": "200",
	})); got != 1 {
		t.Fatalf("swap HTTP requests = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.swaps.WithLabelValues("discovery", "success")); got != 1 {
		t.Fatalf("swap discovery successes = %v, want 1", got)
	}

	// Even if a caller passes unknown values internally, metrics must collapse them to the finite label set.
	m.observeSwap(swapPhase("UNKNOWN"), "unknown")
	if got := testutil.ToFloat64(m.swaps.WithLabelValues("invalid", "dependency_error")); got != 1 {
		t.Fatalf("normalized unknown outcome = %v, want 1", got)
	}
}
