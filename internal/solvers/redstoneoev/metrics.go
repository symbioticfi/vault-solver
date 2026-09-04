package redstoneoev

import (
	"math/big"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

var auctionOutcomes = []string{
	"breaker", "context_canceled", "duplicate", "enqueued", "feed_ignored", "send_dropped",
	"sign_error", "signer_locked", "strategy_error", "strategy_invalid", "too_late", "would_bid",
	"bid_cap", "bid_lane_busy", "deposit_low", "empty_auction_id", "executor_state_stale",
	"no_legs", "gas_unprofitable", "stale_epoch", "stale_state", "in_flight", "callback_balance",
	"strategy_skip", "other",
}

// metrics are the OEV solver's collectors, registered on the shared Prometheus registry (served at
// the framework's /metrics). All methods are nil-safe so the solver runs unmetered when no registry
// is provided.
type metrics struct {
	workflow   *observability.WorkflowMetrics
	hotPath    prometheus.Histogram
	deposit    prometheus.Gauge
	depositLow prometheus.Gauge // 1 when the deposit is below the on-chain MIN_DEPOSIT floor
	feed       prometheus.Gauge
	winflight  prometheus.Gauge
	oldestWin  prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer, strategyName string) (*metrics, error) {
	workflow, err := observability.NewWorkflowMetrics(reg, Name, observability.WorkflowSpec{
		Strategy: strategyName, Operations: []string{"state_refresh"},
		Events: []observability.WorkflowEventSpec{
			{Event: "auction", Outcomes: auctionOutcomes},
			{Event: "bid", Outcomes: []string{"enqueued", "won", "settled_success", "settled_failed", "would_bid", "unresolved"}},
			{Event: "breaker", Outcomes: []string{"failure"}},
			{Event: "state_refresh", Outcomes: []string{"success"}},
		},
		Amounts: []observability.WorkflowAmountSpec{{Event: "bid", Kinds: []string{"enqueued", "won", "settled_success", "settled_failed", "would_bid"}, Assets: []string{"native"}}},
	})
	if err != nil {
		return nil, err
	}
	reg = prometheus.WrapRegistererWith(prometheus.Labels{"strategy": strategyName}, reg)
	m := &metrics{
		workflow: workflow,
		hotPath: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "oev_hotpath_seconds", Help: "handleAuction wall-clock (the ~400ms budget).",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1},
		}),
		deposit:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_wei", Help: "Signer's Executor deposit (wei)."}),
		depositLow: prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_below_floor", Help: "1 when the Executor deposit is below the on-chain MIN_DEPOSIT floor."}),
		feed:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_feed_connected", Help: "Whether the subscribed OEV feed is connected."}),
		winflight:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_won_inflight", Help: "Winning bids awaiting settlement."}),
		oldestWin:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_oldest_won_inflight_age_seconds", Help: "Age of the oldest unsettled win."}),
	}
	if err := observability.RegisterCollectors(
		reg, "redstoneoev", m.hotPath, m.deposit, m.depositLow,
		m.feed, m.winflight, m.oldestWin,
	); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *metrics) bid(amount *big.Int) {
	if m != nil {
		m.workflow.ObserveEvent("auction", "enqueued")
		m.observeBid("enqueued", amount)
	}
}

func (m *metrics) wouldBid(amount *big.Int) {
	if m != nil {
		m.workflow.ObserveEvent("auction", "would_bid")
		m.observeBid("would_bid", amount)
	}
}

func (m *metrics) settled(amount *big.Int) {
	if m != nil {
		m.observeBid("settled_success", amount)
	}
}

func (m *metrics) won(amount *big.Int) {
	if m != nil {
		m.observeBid("won", amount)
	}
}

func (m *metrics) failed(amount *big.Int) {
	if m != nil {
		m.observeBid("settled_failed", amount)
		m.workflow.ObserveEvent("breaker", "failure")
	}
}

func (m *metrics) observeBid(outcome string, amount *big.Int) {
	m.workflow.ObserveEvent("bid", outcome)
	m.workflow.AddAmount("bid", "native", outcome, amount)
}

func (m *metrics) skip(reason string) {
	if m != nil {
		if !slices.Contains(auctionOutcomes, reason) {
			reason = "other"
		}
		m.workflow.ObserveEvent("auction", reason)
	}
}

func (m *metrics) operation() *observability.OperationObserver {
	if m == nil {
		return nil
	}
	return m.workflow.Operation("state_refresh")
}

func (m *metrics) stateRefreshed() {
	if m != nil {
		m.workflow.ObserveEvent("state_refresh", "success")
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
	m.feed.Set(value)
}

func (m *metrics) updateWins(count int, oldest time.Duration) {
	if m != nil {
		m.winflight.Set(float64(count))
		m.oldestWin.Set(oldest.Seconds())
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
