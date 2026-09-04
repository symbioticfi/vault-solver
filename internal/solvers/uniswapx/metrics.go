package uniswapx

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	quoteRefreshOperation       = "quote_refresh"
	exclusiveOrderPollOperation = "exclusive_order_poll"
	publicOrderPollOperation    = "public_order_poll"
)

var quoteMetricOutcomes = []string{
	"invalid", "breaker-notification", "error", "quoted",
	"declined_blocked", "declined_invalid_request", "declined_pair_out_of_scope",
	"declined_invalid_amount", "declined_quote_state_unavailable", "declined_strategy",
	"declined_state_changed", "declined_unknown",
}

type uniswapXMetrics struct {
	workflow      *observability.WorkflowMetrics
	quoteTime     prometheus.Histogram
	blockUntil    prometheus.Gauge
	ready         prometheus.GaugeFunc
	quoteRefresh  prometheus.Gauge
	exclusivePoll prometheus.Gauge
	pendingFills  prometheus.Gauge
	outstanding   prometheus.Gauge
	deadline      prometheus.Gauge
	fill          *liquidlane.FillMetrics
}

func newUniswapXMetrics(reg prometheus.Registerer, ready func() bool, strategy string) (*uniswapXMetrics, error) {
	spec := liquidlane.FillWorkflowSpec()
	spec.Strategy = strategy
	spec.Operations = []string{quoteRefreshOperation, exclusiveOrderPollOperation, publicOrderPollOperation}
	spec.Events = append(spec.Events,
		observability.WorkflowEventSpec{Event: "fill", Outcomes: []string{liquidlane.FillOutcomeFailure, liquidlane.FillOutcomeNotAdmitted}},
		observability.WorkflowEventSpec{Event: "quote", Outcomes: quoteMetricOutcomes},
		observability.WorkflowEventSpec{Event: "exclusive_order_poll", Outcomes: []string{"ok", "failed"}},
		observability.WorkflowEventSpec{Event: "public_order_poll", Outcomes: []string{"ok", "failed"}},
		observability.WorkflowEventSpec{Event: "exclusive_obligation", Outcomes: []string{"won", "settled_in_time", "missed"}},
	)
	spec.Amounts = append(spec.Amounts, observability.WorkflowAmountSpec{Event: "quote", Kinds: []string{"input", "output"}})
	workflow, err := observability.NewWorkflowMetrics(reg, Name, spec)
	if err != nil {
		return nil, err
	}
	m := &uniswapXMetrics{
		workflow:   workflow,
		quoteTime:  prometheus.NewHistogram(prometheus.HistogramOpts{Name: "uniswapx_quote_duration_seconds", Help: "Quote handler latency.", Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5}}),
		blockUntil: prometheus.NewGauge(prometheus.GaugeOpts{Name: "uniswapx_block_until_timestamp", Help: "Current quote blocker deadline."}),
		ready: prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "uniswapx_ready", Help: "Whether quoting is ready."}, func() float64 {
			if ready != nil && ready() {
				return 1
			}
			return 0
		}),
		quoteRefresh:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "uniswapx_last_quote_refresh_timestamp", Help: "Last quote-state publication."}),
		exclusivePoll: prometheus.NewGauge(prometheus.GaugeOpts{Name: "uniswapx_last_exclusive_poll_timestamp", Help: "Last successful exclusive poll."}),
		pendingFills:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "uniswapx_pending_fills", Help: "Admitted fills awaiting a result."}),
		outstanding:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "uniswapx_exclusive_obligations_outstanding", Help: "Outstanding exclusive obligations."}),
		deadline:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "uniswapx_exclusive_nearest_deadline_timestamp", Help: "Nearest exclusive deadline."}),
		fill:          liquidlane.NewFillMetrics(workflow),
	}
	if err := observability.RegisterCollectors(
		reg, "uniswapx", m.quoteTime, m.blockUntil, m.ready, m.quoteRefresh,
		m.exclusivePoll, m.pendingFills, m.outstanding, m.deadline,
	); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *uniswapXMetrics) operation(name string) *observability.OperationObserver {
	if m == nil {
		return nil
	}
	return m.workflow.Operation(name)
}

func (m *uniswapXMetrics) observeQuote(outcome string) {
	if m != nil {
		m.workflow.ObserveEvent("quote", outcome)
	}
}

func (m *uniswapXMetrics) observeQuotedAmounts(response quoteResponse) {
	if m == nil || !response.quotedPairBounded {
		return
	}
	in, inOK := new(big.Int).SetString(response.AmountIn, 10)
	out, outOK := new(big.Int).SetString(response.AmountOut, 10)
	if inOK && outOK && common.IsHexAddress(response.TokenIn) && common.IsHexAddress(response.TokenOut) {
		m.workflow.AddAmount("quote", common.HexToAddress(response.TokenIn).Hex(), "input", in)
		m.workflow.AddAmount("quote", common.HexToAddress(response.TokenOut).Hex(), "output", out)
	}
}

func quoteMetricOutcome(reason string) string {
	switch reason {
	case "blocked":
		return "declined_blocked"
	case "invalid-request":
		return "declined_invalid_request"
	case "pair-out-of-scope":
		return "declined_pair_out_of_scope"
	case "invalid-amount":
		return "declined_invalid_amount"
	case "quote-state-unavailable":
		return "declined_quote_state_unavailable"
	case "strategy-declined":
		return "declined_strategy"
	case "state-changed":
		return "declined_state_changed"
	default:
		return "declined_unknown"
	}
}

func quotePairIsBounded(state *quoteState, tokenIn, tokenOut common.Address) bool {
	for _, inventory := range state.inventory {
		if inventory.TokenIn == tokenIn && inventory.TokenOut == tokenOut {
			return true
		}
	}
	return false
}

func (m *uniswapXMetrics) observeQuoteLatency(elapsed time.Duration) {
	if m != nil {
		m.quoteTime.Observe(elapsed.Seconds())
	}
}

func (m *uniswapXMetrics) observePoll(source, outcome string) {
	if m == nil {
		return
	}
	event := "public_order_poll"
	if source == string(orderSourceExclusiveV2) {
		event = "exclusive_order_poll"
	}
	m.workflow.ObserveEvent(event, outcome)
}

func (m *uniswapXMetrics) fillFailed(notAdmitted bool) {
	if m != nil {
		m.fill.ObserveFailure(notAdmitted)
	}
}

func (m *uniswapXMetrics) observeExclusive(outcome string) {
	if m != nil {
		m.workflow.ObserveEvent("exclusive_obligation", outcome)
	}
}

func (m *uniswapXMetrics) successfulFill(
	receipt *types.Receipt,
	tokenIn common.Address,
	amountIn *big.Int,
	tokenOut common.Address,
	amountOut *big.Int,
) {
	if m != nil {
		m.fill.Observe(receipt, tokenIn, amountIn, tokenOut, amountOut, new(big.Int))
	}
}

func (m *uniswapXMetrics) observeExclusiveWon() {
	if m != nil {
		m.workflow.ObserveEvent("exclusive_obligation", "won")
	}
}

func (m *uniswapXMetrics) quoteRefreshed(at time.Time) {
	if m != nil {
		m.quoteRefresh.Set(float64(at.Unix()))
	}
}

func (m *uniswapXMetrics) exclusivePollSucceeded(at time.Time) {
	if m != nil {
		m.exclusivePoll.Set(float64(at.Unix()))
	}
}

func (m *uniswapXMetrics) setPendingFills(count int) {
	if m != nil {
		m.pendingFills.Set(float64(count))
	}
}

func (s *Solver) observeExclusiveState() {
	if s.metrics == nil {
		return
	}
	count, deadline := s.ledger.exclusiveMetrics()
	s.metrics.outstanding.Set(float64(count))
	s.metrics.deadline.Set(float64(deadline))
}
