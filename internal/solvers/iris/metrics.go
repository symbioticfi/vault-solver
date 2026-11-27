package iris

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// metrics is nil-safe: a host without a registry records nothing.
type metrics struct {
	quotes   *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newMetrics(m *observability.Metrics) *metrics {
	if m == nil {
		return nil
	}
	out := &metrics{
		quotes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iris_quote_requests_total", Help: "IRIS quote requests by kind and outcome (quoted or the decline reason).",
		}, []string{"kind", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "iris_quote_duration_seconds", Help: "IRIS quote handler latency, including on-chain signing for firm quotes.",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 15, 20, 25, 30},
		}, []string{"kind"}),
	}
	m.Registerer().MustRegister(out.quotes, out.duration)
	return out
}

func (m *metrics) observe(kind, outcome string, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.quotes.WithLabelValues(kind, outcome).Inc()
	m.duration.WithLabelValues(kind).Observe(elapsed.Seconds())
}
