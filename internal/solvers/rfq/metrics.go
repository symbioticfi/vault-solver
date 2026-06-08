package rfq

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// httpMetrics is the RFQ quote-server's HTTP request instrumentation, registered on the shared
// Prometheus registry (served at the framework's /metrics). Names mirror the prior filler so existing
// dashboards/alerts keep working.
type httpMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newHTTPMetrics(reg prometheus.Registerer) (*httpMetrics, error) {
	m := &httpMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rfq_filler_http_requests_total",
			Help: "Total HTTP requests handled by the RFQ filler.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "rfq_filler_http_request_duration_seconds",
			Help:    "RFQ filler HTTP request duration in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"method", "route", "status"}),
	}
	if err := reg.Register(m.requests); err != nil {
		return nil, errors.Errorf("rfq: register http requests metric: %w", err)
	}
	if err := reg.Register(m.duration); err != nil {
		return nil, errors.Errorf("rfq: register http duration metric: %w", err)
	}
	return m, nil
}

// instrument wraps a handler to record per-request count + duration. The route label is drawn from a
// fixed allowlist so unmatched paths can't blow up label cardinality.
func (m *httpMetrics) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		labels := prometheus.Labels{
			"method": r.Method,
			"route":  routeLabel(r.URL.Path),
			"status": strconv.Itoa(rec.status),
		}
		m.requests.With(labels).Inc()
		m.duration.With(labels).Observe(time.Since(start).Seconds())
	})
}

// statusRecorder captures the response status code for the metrics labels.
type statusRecorder struct {
	http.ResponseWriter

	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// routeLabel maps a path to a bounded set of route labels (known routes, else "other").
func routeLabel(path string) string {
	switch path {
	case "/health", "/quote", "/openapi.json", "/openapi.yaml", "/docs":
		return path
	default:
		return "other"
	}
}
