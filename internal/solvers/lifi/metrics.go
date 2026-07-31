package lifi

import (
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type lifiMetrics struct {
	activeQuotes       prometheus.Gauge
	lastRefresh        prometheus.Gauge
	orderFeedConnected prometheus.GaugeFunc
	fillAmounts        *liquidlane.FillMetrics
}

func newLIFIMetrics(reg prometheus.Registerer, feed *orderFeed) (*lifiMetrics, error) {
	fillAmounts, err := liquidlane.NewFillMetrics(reg, "lifi")
	if err != nil {
		return nil, err
	}
	m := &lifiMetrics{
		activeQuotes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_active_quotes",
			Help: "Process-local standing quote count from the last successful reconciliation; quotes may expire remotely after their TTL.",
		}),
		lastRefresh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_last_successful_refresh_timestamp",
			Help: "Unix timestamp of the last successful standing-quote reconciliation.",
		}),
		orderFeedConnected: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "lifi_order_feed_connected",
			Help: "1 while the LI.FI WebSocket order feed owns an established connection; 0 otherwise.",
		}, func() float64 {
			if feed != nil && feed.connected.Load() {
				return 1
			}
			return 0
		}),
		fillAmounts: fillAmounts,
	}
	for _, collector := range []prometheus.Collector{m.activeQuotes, m.lastRefresh, m.orderFeedConnected} {
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
