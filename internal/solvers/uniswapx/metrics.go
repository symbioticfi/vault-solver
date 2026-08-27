package uniswapx

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type quoteMetricOutcome string

const (
	quoteOutcomeInvalid                quoteMetricOutcome = "invalid"
	quoteOutcomeBreakerNotification    quoteMetricOutcome = "breaker-notification"
	quoteOutcomeError                  quoteMetricOutcome = "error"
	quoteOutcomeQuoted                 quoteMetricOutcome = "quoted"
	quoteOutcomeDeclinedBlocked        quoteMetricOutcome = "declined_blocked"
	quoteOutcomeDeclinedInvalidRequest quoteMetricOutcome = "declined_invalid_request"
	quoteOutcomeDeclinedPairOutOfScope quoteMetricOutcome = "declined_pair_out_of_scope"
	quoteOutcomeDeclinedInvalidAmount  quoteMetricOutcome = "declined_invalid_amount"
	quoteOutcomeDeclinedQuoteState     quoteMetricOutcome = "declined_quote_state_unavailable"
	quoteOutcomeDeclinedStrategy       quoteMetricOutcome = "declined_strategy"
	quoteOutcomeDeclinedStateChanged   quoteMetricOutcome = "declined_state_changed"
	quoteOutcomeDeclinedUnknown        quoteMetricOutcome = "declined_unknown"
)

const (
	quoteRefreshOperation       = "quote_refresh"
	exclusiveOrderPollOperation = "exclusive_order_poll"
	publicOrderPollOperation    = "public_order_poll"
)

var quoteMetricOutcomes = [...]quoteMetricOutcome{
	quoteOutcomeInvalid, quoteOutcomeBreakerNotification, quoteOutcomeError, quoteOutcomeQuoted,
	quoteOutcomeDeclinedBlocked, quoteOutcomeDeclinedInvalidRequest, quoteOutcomeDeclinedPairOutOfScope,
	quoteOutcomeDeclinedInvalidAmount, quoteOutcomeDeclinedQuoteState, quoteOutcomeDeclinedStrategy,
	quoteOutcomeDeclinedStateChanged, quoteOutcomeDeclinedUnknown,
}

type uniswapXOperationObservers struct {
	quoteRefresh       *observability.OperationObserver
	exclusiveOrderPoll *observability.OperationObserver
	publicOrderPoll    *observability.OperationObserver
}

type uniswapXMetrics struct {
	workflow             *observability.WorkflowMetrics
	operations           uniswapXOperationObservers
	quoteTime            prometheus.Histogram
	exclusiveOutstanding prometheus.GaugeFunc
	exclusiveDeadline    prometheus.GaugeFunc
	blockUntil           prometheus.GaugeFunc
	ready                prometheus.GaugeFunc
	quoteRefresh         prometheus.Gauge
	exclusivePoll        prometheus.GaugeFunc
	pendingFills         prometheus.GaugeFunc
	fillAmounts          *liquidlane.FillMetrics
	now                  func() time.Time
}

func newUniswapXMetrics(
	reg prometheus.Registerer,
	solver *Solver,
	strategyName string,
) (*uniswapXMetrics, error) {
	spec := liquidlane.FillWorkflowSpec()
	spec.Events = append(spec.Events, observability.WorkflowEventSpec{
		Event: "fill", Outcomes: []string{liquidlane.FillOutcomeFailure, liquidlane.FillOutcomeNotAdmitted},
	})
	spec.Strategy = strategyName
	spec.Operations = []string{
		quoteRefreshOperation, exclusiveOrderPollOperation, publicOrderPollOperation,
	}
	for _, outcome := range quoteMetricOutcomes {
		spec.Events = append(spec.Events, observability.WorkflowEventSpec{
			Event: "quote", Outcomes: []string{string(outcome)},
		})
	}
	spec.Events = append(spec.Events,
		observability.WorkflowEventSpec{Event: "exclusive_order_poll", Outcomes: []string{"ok", "failed"}},
		observability.WorkflowEventSpec{Event: "public_order_poll", Outcomes: []string{"ok", "failed"}},
		observability.WorkflowEventSpec{
			Event:    "exclusive_obligation",
			Outcomes: []string{"won", exclusiveOutcomeSettledInTime, exclusiveOutcomeMissed},
		},
	)
	spec.Amounts = append(spec.Amounts, observability.WorkflowAmountSpec{
		Event: "quote", Kinds: []string{"input", "output"},
	})
	workflow, err := observability.NewWorkflowMetrics(reg, Name, spec)
	if err != nil {
		return nil, err
	}
	m := &uniswapXMetrics{
		workflow: workflow,
		operations: uniswapXOperationObservers{
			quoteRefresh:       workflow.Operation(quoteRefreshOperation),
			exclusiveOrderPoll: workflow.Operation(exclusiveOrderPollOperation),
			publicOrderPoll:    workflow.Operation(publicOrderPollOperation),
		},
		quoteTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "uniswapx_quote_duration_seconds", Help: "UniswapX quote webhook handler latency.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		}),
		exclusiveOutstanding: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_exclusive_obligations_outstanding",
			Help: "Live-observed or recovered exclusive delivery obligations awaiting terminal classification.",
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
		blockUntil: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_block_until_timestamp",
			Help: "Maximum Unix deadline among remote, local-fill, exclusive-fade, and startup-warmup quote blockers.",
		}, func() float64 {
			return float64(solver.timeBasedBlockUntil())
		}),
		ready: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_ready",
			Help: "1 when current quote cache, breaker state, exclusive delivery, and the transaction nonce lane permit quoting.",
		}, func() float64 {
			if solver.ready() {
				return 1
			}
			return 0
		}),
		quoteRefresh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "uniswapx_last_quote_refresh_timestamp",
			Help: "Unix timestamp when quote state was last atomically published; inventory may be empty.",
		}),
		exclusivePoll: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_last_exclusive_poll_timestamp",
			Help: "Unix timestamp of the last successful exclusive poll and obligation reconciliation.",
		}, func() float64 {
			return float64(solver.lastExclusivePoll.Load())
		}),
		pendingFills: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "uniswapx_pending_fills",
			Help: "Current admitted UniswapX fills holding capacity while awaiting a txmanager terminal result.",
		}, func() float64 {
			return float64(solver.capacity.Len())
		}),
		fillAmounts: liquidlane.NewFillMetrics(workflow),
		now:         time.Now,
	}
	collectors := []prometheus.Collector{
		m.quoteTime, m.exclusiveOutstanding, m.exclusiveDeadline, m.blockUntil, m.ready,
		m.quoteRefresh, m.exclusivePoll, m.pendingFills,
	}
	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("uniswapx: register metric: %w", err)
		}
	}
	return m, nil
}

func (s *Solver) observeFillOutcome(outcome string) {
	if s.metrics != nil {
		s.metrics.fillAmounts.ObserveOutcome(outcome)
	}
}

func (s *Solver) observeQuote(outcome quoteMetricOutcome) {
	if s.metrics != nil {
		s.metrics.workflow.ObserveEventAt("quote", string(outcome), 1, s.metrics.now())
	}
}

func (s *Solver) observeQuoteDecline(reason quoteDeclineReason) {
	s.observeQuote(metricOutcomeForDecline(reason))
}

func (s *Solver) observeQuotedAmounts(response quoteResponse) {
	if s.metrics == nil || !common.IsHexAddress(response.TokenIn) || !common.IsHexAddress(response.TokenOut) {
		return
	}
	tokenIn := common.HexToAddress(response.TokenIn)
	tokenOut := common.HexToAddress(response.TokenOut)
	if !s.quotedPairIsBounded(tokenIn, tokenOut) {
		return
	}
	amountIn, inputOK := new(big.Int).SetString(response.AmountIn, 10)
	amountOut, outputOK := new(big.Int).SetString(response.AmountOut, 10)
	if !inputOK || !outputOK || amountIn.Sign() <= 0 || amountOut.Sign() <= 0 {
		return
	}
	s.metrics.addQuotedAmount(tokenIn, "input", amountIn)
	s.metrics.addQuotedAmount(tokenOut, "output", amountOut)
}

func (s *Solver) quotedPairIsBounded(tokenIn, tokenOut common.Address) bool {
	state := s.quoteState.Load()
	if state == nil {
		return false
	}
	for _, inventory := range state.inventory {
		if inventory.TokenIn == tokenIn && inventory.TokenOut == tokenOut {
			return true
		}
	}
	return false
}

func (m *uniswapXMetrics) addQuotedAmount(token common.Address, side string, amount *big.Int) {
	m.workflow.AddAmount("quote", token.Hex(), side, amount)
}

// metricOutcomeForDecline deliberately maps every domain reason to a fixed label. The fallback is
// bounded too, so a future missed mapping cannot leak arbitrary text into Prometheus cardinality.
func metricOutcomeForDecline(reason quoteDeclineReason) quoteMetricOutcome {
	switch reason {
	case quoteDeclineBlocked:
		return quoteOutcomeDeclinedBlocked
	case quoteDeclineInvalidRequest:
		return quoteOutcomeDeclinedInvalidRequest
	case quoteDeclinePairOutOfScope:
		return quoteOutcomeDeclinedPairOutOfScope
	case quoteDeclineInvalidAmount:
		return quoteOutcomeDeclinedInvalidAmount
	case quoteDeclineQuoteStateUnavailable:
		return quoteOutcomeDeclinedQuoteState
	case quoteDeclineStrategy:
		return quoteOutcomeDeclinedStrategy
	case quoteDeclineStateChanged:
		return quoteOutcomeDeclinedStateChanged
	default:
		return quoteOutcomeDeclinedUnknown
	}
}

func (m *uniswapXMetrics) observeQuoteLatency(elapsed time.Duration) {
	if m != nil {
		m.quoteTime.Observe(elapsed.Seconds())
	}
}

func (s *Solver) observePoll(source, outcome string) {
	if s.metrics == nil {
		return
	}
	event := ""
	switch source {
	case string(orderSourceExclusiveV2):
		event = "exclusive_order_poll"
	case string(orderSourcePublicV2):
		event = "public_order_poll"
	}
	s.metrics.workflow.ObserveEventAt(event, outcome, 1, s.metrics.now())
}

func (s *Solver) observeExclusiveWin() {
	if s.metrics != nil {
		s.metrics.workflow.ObserveEventAt("exclusive_obligation", "won", 1, s.metrics.now())
	}
}

func (s *Solver) observeExclusiveOutcome(outcome string) {
	if s.metrics != nil {
		s.metrics.workflow.ObserveEventAt("exclusive_obligation", outcome, 1, s.metrics.now())
	}
}
