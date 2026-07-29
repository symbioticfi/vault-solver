package rfq

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type rfqMetrics struct {
	requests     *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	wins         prometheus.Counter
	activeOrders prometheus.GaugeFunc
	oldestActive prometheus.GaugeFunc
	fillAmounts  *liquidlane.FillMetrics
}

func newRFQMetrics(reg prometheus.Registerer, st *store) (*rfqMetrics, error) {
	fillAmounts, err := liquidlane.NewFillMetrics(reg, "rfq")
	if err != nil {
		return nil, err
	}
	m := &rfqMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rfq_filler_http_requests_total",
			Help: "Total HTTP requests handled by the RFQ filler.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "rfq_filler_http_request_duration_seconds",
			Help:    "RFQ filler HTTP request duration in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"method", "route", "status"}),
		wins: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rfq_wins_total",
			Help: "RFQ orders first observed assigned to this filler.",
		}),
		activeOrders: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "rfq_active_orders",
			Help: "RFQ orders currently queued, submitting, or awaiting backend settlement.",
		}, func() float64 {
			count, _ := st.activeOrderMetrics()
			return float64(count)
		}),
		oldestActive: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "rfq_oldest_active_order_age_seconds",
			Help: "Age of the oldest active RFQ order; zero when none.",
		}, func() float64 {
			_, age := st.activeOrderMetrics()
			return age.Seconds()
		}),
		fillAmounts: fillAmounts,
	}
	for _, collector := range []prometheus.Collector{
		m.requests, m.duration, m.wins, m.activeOrders, m.oldestActive,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("rfq: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *rfqMetrics) observeWin() {
	if m != nil {
		m.wins.Inc()
	}
}

// instrument wraps a handler to record per-request count + duration. The route label is drawn from a
// fixed allowlist so unmatched paths can't blow up label cardinality.
func (m *rfqMetrics) instrument(next http.Handler) http.Handler {
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
