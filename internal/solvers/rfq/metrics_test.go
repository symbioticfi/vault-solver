package rfq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetrics_InstrumentRecordsRequest(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newRFQMetrics(reg, newStore(time.Now))
	if err != nil {
		t.Fatalf("newRFQMetrics: %v", err)
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

func TestRFQMetricsTrackUniqueWinsAndActiveOrders(t *testing.T) {
	now := time.Unix(1_000, 0)
	st := newStore(func() time.Time { return now })
	metrics, err := newRFQMetrics(prometheus.NewRegistry(), st)
	if err != nil {
		t.Fatal(err)
	}
	exec := &executionService{
		executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		backend:  &fakeBackend{open: []backendOrder{{OrderID: "order-1"}}},
		store:    st,
		metrics:  metrics,
		log:      logr.Discard(),
	}

	if err := exec.pollOpenOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := exec.pollOpenOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)

	if got := testutil.ToFloat64(metrics.wins); got != 1 {
		t.Fatalf("wins = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.activeOrders); got != 1 {
		t.Fatalf("active orders = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.oldestActive); got != 10 {
		t.Fatalf("oldest active age = %v, want 10", got)
	}
}
