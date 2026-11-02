package liquidlane

import (
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// QuoteMetrics records one coarse final HTTP result per request, including unproven pairs.
// PairMetrics separately counts the trusted supported-direction subset; amounts and identities stay in logs.
type QuoteMetrics struct {
	results     *prometheus.CounterVec
	boundReason func(string) string
}

// NewQuoteMetrics registers the common coarse transport counter.
func NewQuoteMetrics(reg prometheus.Registerer, solver string, boundReason func(string) string) (*QuoteMetrics, error) {
	m := &QuoteMetrics{
		boundReason: boundReason,
		results: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "solver_bot_quote_results_total",
			Help: "One final HTTP quote result; sum results for incoming requests. Supported directed pairs are counted separately; amounts and request identities remain in logs.",
		}, []string{"result", "reason"}),
	}
	if err := prometheus.WrapRegistererWith(prometheus.Labels{"solver": solver}, reg).Register(m.results); err != nil {
		return nil, errors.Errorf("register quote metrics: %w", err)
	}
	return m, nil
}

// Observe never inspects request tokens or amounts. The integration owns the fixed reason set.
func (m *QuoteMetrics) Observe(returned bool, reason string) {
	if m == nil {
		return
	}
	reason = m.boundReason(reason)
	result := "rejected"
	if returned {
		result, reason = "returned", "none"
	}
	m.results.WithLabelValues(result, reason).Inc()
}
