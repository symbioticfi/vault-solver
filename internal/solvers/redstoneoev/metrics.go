package redstoneoev

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	oevBidSubmitted      = "submitted"
	oevBidWon            = "won"
	oevBidSettledSuccess = "settled_success"
	oevBidSettledFailed  = "settled_failed"
)

// metrics are the OEV solver's collectors, registered on the shared Prometheus registry (served at
// the framework's /metrics). All methods are nil-safe so the solver runs unmetered when no registry
// is provided.
type metrics struct {
	auctions            prometheus.Counter
	bids                prometheus.Counter
	bidWei              *prometheus.CounterVec
	wins                prometheus.Counter
	failedLiq           prometheus.Counter
	settlements         *prometheus.CounterVec
	wonInflight         prometheus.GaugeFunc
	unresolvedWinsTotal prometheus.Counter
	skips               *prometheus.CounterVec
	hotPath             prometheus.Histogram
	deposit             prometheus.Gauge
	depositLow          prometheus.Gauge // 1 when the deposit is below the on-chain MIN_DEPOSIT floor
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
		auctions: prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_auctions_total", Help: "OEV auction frames seen."}),
		bids:     prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_bids_total", Help: "Bids sent (or would-bid in dry-run)."}),
		bidWei: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "oev_bid_wei_total",
			Help: "Bid wei at matched local auction lifecycle stages.",
		}, []string{"stage"}),
		wins: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "oev_wins_total",
			Help: "Wins matched to an active local bid reservation; late frames after reconciliation are excluded.",
		}),
		failedLiq: prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_failed_liquidations_total", Help: "Reverted settlements for our callback (from the WS liquidation-result frame)."}),
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
		skips: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "oev_skips_total", Help: "Auctions not bid on, by reason.",
		}, []string{"reason"}),
		hotPath: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "oev_hotpath_seconds", Help: "handleAuction wall-clock (the ~400ms budget).",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1},
		}),
		deposit:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_wei", Help: "Signer's Executor deposit (wei)."}),
		depositLow: prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_below_floor", Help: "1 when the Executor deposit is below the on-chain MIN_DEPOSIT floor."}),
	}
	for _, c := range []prometheus.Collector{
		m.auctions, m.bids, m.bidWei, m.wins, m.failedLiq, m.settlements, m.wonInflight,
		m.unresolvedWinsTotal, m.skips, m.hotPath, m.deposit, m.depositLow,
	} {
		if err := reg.Register(c); err != nil {
			return nil, errors.Errorf("redstoneoev: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) auction() {
	if m != nil {
		m.auctions.Inc()
	}
}

func (m *metrics) wouldBid() {
	if m != nil {
		m.bids.Inc()
	}
}

func (m *metrics) submittedBid(amount *big.Int) {
	if m == nil {
		return
	}
	m.bids.Inc()
	m.addBidWei(oevBidSubmitted, amount)
}

func (m *metrics) won(amount *big.Int) {
	if m == nil {
		return
	}
	m.wins.Inc()
	m.addBidWei(oevBidWon, amount)
}

func (m *metrics) failed() {
	if m != nil {
		m.failedLiq.Inc()
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

func (m *metrics) skip(reason string) {
	if m != nil {
		m.skips.WithLabelValues(reason).Inc()
	}
}

func (m *metrics) latency(d time.Duration) {
	if m != nil {
		m.hotPath.Observe(d.Seconds())
	}
}

func (m *metrics) depositWei(depositWei float64) {
	if m != nil {
		m.deposit.Set(depositWei)
	}
}

// depositBelowFloor sets the alarm gauge; "below" now means deposit < MIN_DEPOSIT.
func (m *metrics) depositBelowFloor(below bool) {
	if m != nil {
		v := 0.0
		if below {
			v = 1
		}
		m.depositLow.Set(v)
	}
}
