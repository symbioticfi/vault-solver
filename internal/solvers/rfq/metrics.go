package rfq

import (
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type rfqMetrics struct {
	workflow          *observability.WorkflowMetrics
	orderPollObserver *observability.OperationObserver
	requests          *prometheus.CounterVec
	duration          *prometheus.HistogramVec
	activeOrders      prometheus.GaugeFunc
	oldestActive      prometheus.GaugeFunc
	fillAmounts       *liquidlane.FillMetrics
	now               func() time.Time
}

func newRFQMetrics(
	reg prometheus.Registerer,
	st *store,
	strategyName string,
) (*rfqMetrics, error) {
	spec := liquidlane.FillWorkflowSpec()
	spec.Strategy = strategyName
	spec.Operations = []string{orderPollOperation}
	spec.Events = append(spec.Events, observability.WorkflowEventSpec{
		Event: "fill", Outcomes: []string{liquidlane.FillOutcomeFailure, liquidlane.FillOutcomeNotAdmitted},
	})
	for _, outcome := range quoteDecisionOutcomes {
		spec.Events = append(spec.Events, observability.WorkflowEventSpec{
			Event: "quote", Outcomes: []string{string(outcome)},
		})
	}
	spec.Events = append(spec.Events,
		observability.WorkflowEventSpec{Event: "order", Outcomes: []string{"won"}},
		observability.WorkflowEventSpec{Event: "order_poll", Outcomes: []string{"success"}},
	)
	spec.Amounts = append(spec.Amounts, observability.WorkflowAmountSpec{
		Event: "quote", Kinds: []string{"input", "output"},
	})
	group := observability.NewMetricGroup("")
	workflow, err := observability.NewWorkflowMetrics(group, Name, spec)
	if err != nil {
		return nil, err
	}
	m := &rfqMetrics{
		workflow:          workflow,
		orderPollObserver: workflow.Operation(orderPollOperation),
		requests:          group.Counter("rfq_filler_http_requests_total", "Deprecated compatibility counter for total RFQ filler HTTP requests; use rfq_filler_http_request_duration_seconds_count.", "method", "route", "status"),
		duration:          group.Histogram("rfq_filler_http_request_duration_seconds", "RFQ filler HTTP request count and duration in seconds.", []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}, "method", "route", "status"),
		activeOrders: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "rfq_active_orders",
			Help: "RFQ orders currently queued, submitting, or awaiting backend settlement.",
		}, func() float64 {
			count, _ := st.activeOrderMetrics()
			return float64(count)
		}),
		oldestActive: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "rfq_oldest_active_order_age_seconds",
			Help: "Age of the oldest active RFQ order; zero when none.",
		}, func() float64 {
			_, age := st.activeOrderMetrics()
			return age.Seconds()
		}),
		fillAmounts: liquidlane.NewFillMetrics(workflow),
		now:         time.Now,
	}
	group.Add(m.activeOrders, m.oldestActive)
	if err := group.Publish(reg); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *rfqMetrics) observeQuoteDecision(outcome quoteDecisionOutcome) {
	if m != nil {
		m.workflow.ObserveEventAt("quote", boundedQuoteDecisionOutcome(outcome), 1, m.now())
	}
}

// boundedQuoteDecisionOutcome fails closed to the fixed error bucket if a future decision path
// forgets to use one of the declared enum values. It must never pass request-derived text to labels.
func boundedQuoteDecisionOutcome(outcome quoteDecisionOutcome) string {
	for _, declared := range quoteDecisionOutcomes {
		if outcome == declared {
			return string(outcome)
		}
	}
	return string(quoteDecisionError)
}

func (m *rfqMetrics) observeQuotedAmounts(observation *quoteObservation) {
	if m == nil || observation == nil {
		return
	}
	m.addQuotedAmount(observation.tokenIn, "input", observation.amountIn)
	m.addQuotedAmount(observation.tokenOut, "output", observation.amountOut)
}

func (m *rfqMetrics) addQuotedAmount(token common.Address, side string, amount *big.Int) {
	if token == (common.Address{}) || amount == nil || amount.Sign() <= 0 {
		return
	}
	m.workflow.AddAmount("quote", token.Hex(), side, amount)
}

func (m *rfqMetrics) observeWin() {
	if m != nil {
		m.workflow.ObserveEventAt("order", "won", 1, m.now())
	}
}

func (m *rfqMetrics) observeOrderPoll(at time.Time) {
	if m != nil {
		m.workflow.ObserveEventAt("order_poll", "success", 1, at)
	}
}

func (m *rfqMetrics) observeHTTP(method, path string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	labels := prometheus.Labels{
		"method": methodLabel(method), "route": routeLabel(path), "status": strconv.Itoa(status),
	}
	m.requests.With(labels).Inc()
	m.duration.With(labels).Observe(duration.Seconds())
}

// methodLabel bounds arbitrary HTTP methods to the methods served by this process.
func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost:
		return method
	default:
		return "other"
	}
}

// routeLabel maps a path to a bounded set of route labels (known routes, else "other").
func routeLabel(path string) string {
	switch path {
	case "/health", "/quote", "/openapi.json", "/openapi.yaml", "/docs":
		return path
	default:
		return "other"
	}
}
