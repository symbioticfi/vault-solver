package lifi

import (
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type lifiMetrics struct {
	activeQuotes prometheus.Gauge
	lastRefresh  prometheus.Gauge
	fillAmounts  *liquidlane.FillMetrics
}

func newLIFIMetrics(reg prometheus.Registerer) (*lifiMetrics, error) {
	fillAmounts, err := liquidlane.NewFillMetrics(reg, "lifi")
	if err != nil {
		return nil, err
	}
	m := &lifiMetrics{
		activeQuotes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_active_quotes",
			Help: "Standing LI.FI quotes in the last successfully reconciled state.",
		}),
		lastRefresh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_last_successful_refresh_timestamp",
			Help: "Unix timestamp of the last successful standing-quote reconciliation.",
		}),
		fillAmounts: fillAmounts,
	}
	for _, collector := range []prometheus.Collector{m.activeQuotes, m.lastRefresh} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("lifi: register metric: %w", err)
		}
	}
	return m, nil
}

func (s *Solver) observeQuoteRefresh(active int) {
	if s.metrics == nil {
		return
	}
	s.metrics.activeQuotes.Set(float64(active))
	s.metrics.lastRefresh.SetToCurrentTime()
}
