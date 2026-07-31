package redstoneoev

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	oevBidEnqueued       = "enqueued"
	oevBidWon            = "won"
	oevBidSettledSuccess = "settled_success"
	oevBidSettledFailed  = "settled_failed"
)

// metrics are the OEV solver's collectors, registered on the shared Prometheus registry (served at
// the framework's /metrics). All methods are nil-safe so the solver runs unmetered when no registry
// is provided.
type metrics struct {
	decisions           *prometheus.CounterVec
	bidWei              *prometheus.CounterVec
	wins                prometheus.Counter
	breakerFailures     prometheus.Counter
	settlements         *prometheus.CounterVec
	wonInflight         prometheus.GaugeFunc
	unresolvedWinsTotal prometheus.Counter
	hotPath             prometheus.Histogram
	deposit             prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer, strategyName string, wonCount func() int) (*metrics, error) {
	if strategyName == "" {
		strategyName = defaultStrategyName
	}
	if wonCount == nil {
		wonCount = func() int { return 0 }
	}
	reg = prometheus.WrapRegistererWith(prometheus.Labels{"strategy": strategyName}, reg)
	m := &metrics{
		decisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "oev_auction_decisions_total",
			Help: "Successfully parsed OEV auction frames by terminal local outcome.",
		}, []string{"outcome"}),
		bidWei: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "oev_bid_wei_total",
			Help: "Bid wei at matched local auction lifecycle stages.",
		}, []string{"stage"}),
		wins: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "oev_wins_total",
			Help: "Wins matched to an active local bid reservation; late frames after reconciliation are excluded.",
		}),
		breakerFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "oev_breaker_failures_total",
			Help: "Failed liquidation-result frames for our callback accepted by the rolling breaker; identifiable replays within its window are excluded.",
		}),
		settlements: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "oev_settlements_total",
			Help: "Settlement results matched to active local bid reservations.",
		}, []string{"result"}),
		wonInflight: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "oev_won_inflight",
			Help: "Locally observed winning bids awaiting settlement.",
		}, func() float64 { return float64(wonCount()) }),
		unresolvedWinsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "oev_unresolved_wins_total",
			Help: "Winning bids whose local reservation expired without settlement or nonce proof.",
		}),
		hotPath: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "oev_hotpath_seconds", Help: "Wall-clock time from a parsed auction frame to its terminal local outcome.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1},
		}),
		deposit: prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_wei", Help: "Signer's Executor deposit (wei)."}),
	}
	for _, c := range []prometheus.Collector{
		m.decisions, m.bidWei, m.wins, m.breakerFailures, m.settlements, m.wonInflight,
		m.unresolvedWinsTotal, m.hotPath, m.deposit,
	} {
		if err := reg.Register(c); err != nil {
			return nil, errors.Errorf("redstoneoev: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) auctionDecision(outcome string, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.decisions.WithLabelValues(outcome).Inc()
	m.hotPath.Observe(elapsed.Seconds())
}

func (m *metrics) enqueuedBid(amount *big.Int) {
	if m != nil {
		m.addBidWei(oevBidEnqueued, amount)
	}
}

func (m *metrics) won(amount *big.Int) {
	if m == nil {
		return
	}
	m.wins.Inc()
	m.addBidWei(oevBidWon, amount)
}

func (m *metrics) breakerFailure() {
	if m != nil {
		m.breakerFailures.Inc()
	}
}

func (m *metrics) settlement(success bool, amount *big.Int) {
	if m == nil {
		return
	}
	result := "failed"
	stage := oevBidSettledFailed
	if success {
		result = "success"
		stage = oevBidSettledSuccess
	}
	m.settlements.WithLabelValues(result).Inc()
	m.addBidWei(stage, amount)
}

func (m *metrics) addBidWei(stage string, amount *big.Int) {
	if amount != nil && amount.Sign() > 0 {
		m.bidWei.WithLabelValues(stage).Add(weiFloat(amount))
	}
}

func (m *metrics) unresolvedWins(count int) {
	if m != nil && count > 0 {
		m.unresolvedWinsTotal.Add(float64(count))
	}
}

func (m *metrics) depositWei(depositWei float64) {
	if m != nil {
		m.deposit.Set(depositWei)
	}
}
