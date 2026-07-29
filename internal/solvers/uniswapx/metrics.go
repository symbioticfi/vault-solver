package uniswapx

import (
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type uniswapXMetrics struct {
	quotes               *prometheus.CounterVec
	quoteTime            prometheus.Histogram
	polls                *prometheus.CounterVec
	fills                *prometheus.CounterVec
	exclusiveWins        prometheus.Counter
	exclusiveOutstanding prometheus.GaugeFunc
	exclusiveDeadline    prometheus.GaugeFunc
	exclusiveOutcomes    *prometheus.CounterVec
	blockUntil           prometheus.Gauge
	ready                prometheus.Gauge
	quoteRefresh         prometheus.Gauge
	exclusivePoll        prometheus.Gauge
	pendingFills         prometheus.Gauge
	fillAmounts          *liquidlane.FillMetrics
}

func newUniswapXMetrics(reg prometheus.Registerer, solver *Solver) (*uniswapXMetrics, error) {
	fillAmounts, err := liquidlane.NewFillMetrics(reg, "uniswapx")
	if err != nil {
		return nil, err
	}
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
		exclusiveWins: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "uniswapx_exclusive_wins_total",
			Help: "Exclusive obligations first observed during live polling.",
		}),
		exclusiveOutstanding: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_exclusive_obligations_outstanding",
			Help: "Exclusive obligations awaiting terminal classification.",
		}, func() float64 {
			outstanding, _ := solver.exclusiveObligationMetrics()
			return float64(outstanding)
		}),
		exclusiveDeadline: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_exclusive_nearest_deadline_timestamp",
			Help: "Unix timestamp of the nearest outstanding exclusive deadline; zero when none.",
		}, func() float64 {
			_, deadline := solver.exclusiveObligationMetrics()
			return float64(deadline)
		}),
		exclusiveOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "uniswapx_exclusive_obligation_outcomes_total",
			Help: "Live-observed exclusive obligations by terminal outcome.",
		}, []string{"outcome"}),
		blockUntil: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_block_until_timestamp", Help: "Unix timestamp until which UniswapX quoting is blocked.",
		}),
		ready: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_ready", Help: "1 when quote cache and exclusive order delivery are healthy.",
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
		fillAmounts: fillAmounts,
	}
	collectors := []prometheus.Collector{
		m.quotes, m.quoteTime, m.polls, m.fills,
		m.exclusiveWins, m.exclusiveOutstanding, m.exclusiveDeadline, m.exclusiveOutcomes,
		m.blockUntil, m.ready,
		m.quoteRefresh, m.exclusivePoll, m.pendingFills,
	}
	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("uniswapx: register metric: %w", err)
		}
	}
	m.exclusiveOutcomes.WithLabelValues(exclusiveOutcomeSettledInTime).Add(0)
	m.exclusiveOutcomes.WithLabelValues(exclusiveOutcomeMissed).Add(0)
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

func (s *Solver) observeExclusiveWin() {
	if s.metrics != nil {
		s.metrics.exclusiveWins.Inc()
	}
}

func (s *Solver) observeExclusiveOutcome(outcome string) {
	if s.metrics != nil {
		s.metrics.exclusiveOutcomes.WithLabelValues(outcome).Inc()
	}
}
