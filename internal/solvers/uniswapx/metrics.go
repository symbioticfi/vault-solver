package uniswapx

import (
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

type uniswapXMetrics struct {
	quotes        *prometheus.CounterVec
	quoteTime     prometheus.Histogram
	polls         *prometheus.CounterVec
	fills         *prometheus.CounterVec
	blockUntil    prometheus.Gauge
	ready         prometheus.GaugeFunc
	quoteRefresh  prometheus.Gauge
	exclusivePoll prometheus.Gauge
	pendingFills  prometheus.Gauge
}

func newUniswapXMetrics(reg prometheus.Registerer, ready func() bool) (*uniswapXMetrics, error) {
	m := &uniswapXMetrics{
		quotes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "uniswapx_quote_requests_total", Help: "UniswapX quote requests by bounded outcome.",
		}, []string{"outcome"}),
		quoteTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "uniswapx_quote_duration_seconds", Help: "UniswapX quote handler latency.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		}),
		polls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "uniswapx_order_polls_total", Help: "UniswapX order polls by source and outcome.",
		}, []string{"source", "outcome"}),
		fills: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "uniswapx_fills_total", Help: "UniswapX fill attempts by outcome.",
		}, []string{"outcome"}),
		blockUntil: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_block_until_timestamp", Help: "Unix timestamp until which UniswapX quoting is blocked.",
		}),
		ready: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_ready", Help: "1 when quote state, exclusive delivery, and the transaction nonce lane are healthy.",
		}, func() float64 {
			if ready() {
				return 1
			}
			return 0
		}),
		quoteRefresh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_last_quote_refresh_timestamp", Help: "Unix timestamp of the last successful quote refresh.",
		}),
		exclusivePoll: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_last_exclusive_poll_timestamp", Help: "Unix timestamp of the last successful exclusive order poll.",
		}),
		pendingFills: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_pending_fills", Help: "Current asynchronous UniswapX fills awaiting a transaction result.",
		}),
	}
	collectors := []prometheus.Collector{
		m.quotes, m.quoteTime, m.polls, m.fills, m.blockUntil, m.ready,
		m.quoteRefresh, m.exclusivePoll, m.pendingFills,
	}
	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("uniswapx: register metric: %w", err)
		}
	}
	return m, nil
}

func (s *Solver) observeQuote(outcome string) {
	if s.metrics != nil {
		s.metrics.quotes.WithLabelValues(outcome).Inc()
	}
}

func (m *uniswapXMetrics) observeQuoteLatency(elapsed time.Duration) {
	if m != nil {
		m.quoteTime.Observe(elapsed.Seconds())
	}
}

func (s *Solver) observePoll(source, outcome string) {
	if s.metrics != nil {
		s.metrics.polls.WithLabelValues(source, outcome).Inc()
	}
}

func (s *Solver) observeFill(outcome string) {
	if s.metrics != nil {
		s.metrics.fills.WithLabelValues(outcome).Inc()
	}
}
