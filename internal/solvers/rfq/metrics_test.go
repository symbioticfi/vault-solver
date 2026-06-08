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
