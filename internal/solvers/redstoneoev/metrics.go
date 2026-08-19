package redstoneoev

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	oevBidEnqueued       = "enqueued"
	oevBidWon            = "won"
	oevBidSettledSuccess = "settled_success"
	oevBidSettledFailed  = "settled_failed"
	oevBidUnresolved     = "unresolved"
)

var (
	auctionDecisionOutcomes = [...]string{
		"breaker", "context_canceled", "duplicate", "enqueued", "feed_ignored", "send_dropped",
		"sign_error", "signer_locked", "state_unknown", "strategy_error", "strategy_invalid", "too_late",
		"would_bid", "bid_cap", "deposit_low", "empty_auction_id", "executor_state_stale",
		"no_legs", "gas_unprofitable", "stale_epoch", "stale_state", "in_flight", "callback_balance", "strategy_skip",
	}
	bidLifecycleStages = [...]string{oevBidEnqueued, oevBidWon, oevBidSettledSuccess, oevBidSettledFailed}
)

// metrics are the OEV solver's collectors, registered on the shared Prometheus registry (served at
// the framework's /metrics). All methods are nil-safe so the solver runs unmetered when no registry
// is provided.
type metrics struct {
	workflow          *observability.WorkflowMetrics
	wonInflight       prometheus.GaugeFunc
	oldestWonInflight prometheus.GaugeFunc
	hotPath           prometheus.Histogram
	deposit           prometheus.Gauge
	feedConnected     prometheus.Gauge
	now               func() time.Time
}

func newMetrics(
	reg prometheus.Registerer,
	strategyName string,
	wonMetrics func() (count int, oldestAge time.Duration),
) (*metrics, error) {
	if strategyName == "" {
		strategyName = defaultStrategyName
	}
	if wonMetrics == nil {
		wonMetrics = func() (int, time.Duration) { return 0, 0 }
	}
	workflow, err := observability.NewWorkflowMetrics(reg, Name, observability.WorkflowSpec{
		Strategy:   strategyName,
		Operations: []string{stateRefreshOperation},
		Events: []observability.WorkflowEventSpec{
			{Event: "auction", Outcomes: auctionDecisionOutcomes[:]},
			{Event: "bid", Outcomes: append(bidLifecycleStages[:], oevBidUnresolved)},
			{Event: "breaker", Outcomes: []string{"failure"}},
			{Event: "state_refresh", Outcomes: []string{"success"}},
		},
		Amounts: []observability.WorkflowAmountSpec{{
			Event: "bid", Kinds: bidLifecycleStages[:], Assets: []string{"native"},
		}},
	})
	if err != nil {
		return nil, err
	}
	reg = prometheus.WrapRegistererWith(prometheus.Labels{"strategy": strategyName}, reg)
	m := &metrics{
		workflow: workflow,
		wonInflight: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "oev_won_inflight",
			Help: "Locally observed winning bids awaiting settlement.",
		}, func() float64 {
			count, _ := wonMetrics()
			return float64(count)
		}),
		oldestWonInflight: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "oev_oldest_won_inflight_age_seconds",
			Help: "Age of the oldest inflight reservation since its win was locally observed; zero when none.",
		}, func() float64 {
			_, oldestAge := wonMetrics()
			return oldestAge.Seconds()
		}),
		hotPath: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "oev_hotpath_seconds", Help: "Wall-clock time from a parsed auction frame to its terminal local outcome.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1},
		}),
		deposit: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "oev_deposit_wei", Help: "Signer's Executor deposit (wei).",
		}),
		feedConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "oev_feed_connected",
			Help: "Whether the OEV WebSocket is connected with all configured topic subscriptions sent (1 or 0).",
		}),
		now: time.Now,
	}
	for _, collector := range []prometheus.Collector{
		m.wonInflight, m.oldestWonInflight, m.hotPath, m.deposit, m.feedConnected,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("redstoneoev: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) auctionDecision(outcome string, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.workflow.ObserveEventAt("auction", outcome, 1, m.now())
	m.hotPath.Observe(elapsed.Seconds())
}

func (m *metrics) enqueuedBid(amount *big.Int) {
	if m != nil {
		m.observeBid(oevBidEnqueued, 1, amount)
	}
}

func (m *metrics) won(amount *big.Int) {
	if m != nil {
		m.observeBid(oevBidWon, 1, amount)
	}
}

func (m *metrics) breakerFailure() {
	if m != nil {
		m.workflow.ObserveEventAt("breaker", "failure", 1, m.now())
	}
}

func (m *metrics) settlement(success bool, amount *big.Int) {
	outcome := oevBidSettledFailed
	if success {
		outcome = oevBidSettledSuccess
	}
	if m != nil {
		m.observeBid(outcome, 1, amount)
	}
}

func (m *metrics) observeBid(outcome string, count float64, amount *big.Int) {
	m.workflow.ObserveEventAt("bid", outcome, count, m.now())
	m.workflow.AddAmount("bid", "native", outcome, amount)
}

func (m *metrics) unresolvedWins(count int) {
	if m != nil && count > 0 {
		m.workflow.ObserveEventAt("bid", oevBidUnresolved, float64(count), m.now())
	}
}

func (m *metrics) depositWei(depositWei float64) {
	if m != nil {
		m.deposit.Set(depositWei)
	}
}

func (m *metrics) setFeedConnected(connected bool) {
	if m == nil {
		return
	}
	value := float64(0)
	if connected {
		value = 1
	}
	m.feedConnected.Set(value)
}

func (m *metrics) stateRefreshed() {
	if m != nil {
		m.workflow.ObserveEventAt("state_refresh", "success", 1, m.now())
	}
}
