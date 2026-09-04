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

const orderPollOperation = "order_poll"

const (
	quoteDecisionQuoted           = "quoted"
	quoteDecisionLaneUnavailable  = "lane_unavailable"
	quoteDecisionNotQuotable      = "not_quotable"
	quoteDecisionBelowMinimum     = "below_minimum"
	quoteDecisionNoCandidates     = "no_candidates"
	quoteDecisionStrategyDeclined = "strategy_declined"
	quoteDecisionBadRequest       = "bad_request"
	quoteDecisionError            = "error"
)

var quoteDecisionOutcomes = []string{
	quoteDecisionQuoted, quoteDecisionLaneUnavailable, quoteDecisionNotQuotable,
	quoteDecisionBelowMinimum, quoteDecisionNoCandidates, quoteDecisionStrategyDeclined,
	quoteDecisionBadRequest, quoteDecisionError,
}

type rfqMetrics struct {
	workflow *observability.WorkflowMetrics
	duration *prometheus.HistogramVec
	active   prometheus.Gauge
	oldest   prometheus.Gauge
	fill     *liquidlane.FillMetrics
}

func newRFQMetrics(reg prometheus.Registerer, strategy string) (*rfqMetrics, error) {
	spec := liquidlane.FillWorkflowSpec()
	spec.Strategy = strategy
	spec.Operations = []string{orderPollOperation}
	spec.Events = append(spec.Events,
		observability.WorkflowEventSpec{Event: "fill", Outcomes: []string{liquidlane.FillOutcomeFailure, liquidlane.FillOutcomeNotAdmitted}},
		observability.WorkflowEventSpec{Event: "quote", Outcomes: quoteDecisionOutcomes},
		observability.WorkflowEventSpec{Event: "order", Outcomes: []string{"won"}},
		observability.WorkflowEventSpec{Event: "order_poll", Outcomes: []string{"success"}},
	)
	spec.Amounts = append(spec.Amounts, observability.WorkflowAmountSpec{Event: "quote", Kinds: []string{"input", "output"}})
	workflow, err := observability.NewWorkflowMetrics(reg, Name, spec)
	if err != nil {
		return nil, err
	}
	m := &rfqMetrics{
		workflow: workflow,
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "rfq_filler_http_request_duration_seconds", Help: "RFQ HTTP request duration.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"method", "route", "status"}),
		active: prometheus.NewGauge(prometheus.GaugeOpts{Name: "rfq_active_orders", Help: "Active RFQ orders."}),
		oldest: prometheus.NewGauge(prometheus.GaugeOpts{Name: "rfq_oldest_active_order_age_seconds", Help: "Age of the oldest active RFQ order."}),
		fill:   liquidlane.NewFillMetrics(workflow),
	}
	if err := observability.RegisterCollectors(reg, "rfq", m.duration, m.active, m.oldest); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *rfqMetrics) quote(outcome string, response *quoteResponse) {
	if m == nil {
		return
	}
	m.workflow.ObserveEvent("quote", outcome)
	if outcome == quoteDecisionQuoted && response != nil {
		amountIn, _ := new(big.Int).SetString(response.AmountIn, 10)
		amountOut, _ := new(big.Int).SetString(response.AmountOut, 10)
		m.workflow.AddAmount("quote", common.HexToAddress(response.TokenIn).Hex(), "input", amountIn)
		m.workflow.AddAmount("quote", common.HexToAddress(response.TokenOut).Hex(), "output", amountOut)
	}
}

func (m *rfqMetrics) operation() *observability.OperationObserver {
	if m == nil {
		return nil
	}
	return m.workflow.Operation(orderPollOperation)
}

func (m *rfqMetrics) pollSucceeded() {
	if m != nil {
		m.workflow.ObserveEvent("order_poll", "success")
	}
}

func (m *rfqMetrics) won() {
	if m != nil {
		m.workflow.ObserveEvent("order", "won")
	}
}

func (m *rfqMetrics) fillFailed(notAdmitted bool) {
	if m != nil {
		m.fill.ObserveFailure(notAdmitted)
	}
}

func (m *rfqMetrics) orders(records []*orderRecord, now time.Time) {
	if m == nil {
		return
	}
	m.active.Set(float64(len(records)))
	oldest := time.Duration(0)
	for _, record := range records {
		if age := now.Sub(record.UpdatedAt); age > oldest {
			oldest = age
		}
	}
	m.oldest.Set(oldest.Seconds())
}

func (m *rfqMetrics) instrument(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		labels := []string{methodLabel(r.Method), routeLabel(r.URL.Path), strconv.Itoa(recorder.status)}
		m.duration.WithLabelValues(labels...).Observe(time.Since(started).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter

	status int
	wrote  bool
}

func (recorder *statusRecorder) WriteHeader(code int) {
	if recorder.wrote {
		return
	}
	recorder.wrote, recorder.status = true, code
	recorder.ResponseWriter.WriteHeader(code)
}

func methodLabel(method string) string {
	if method == http.MethodGet || method == http.MethodPost {
		return method
	}
	return "other"
}

func routeLabel(path string) string {
	switch path {
	case "/health", "/quote", "/openapi.json", "/openapi.yaml", "/docs":
		return path
	default:
		return "other"
	}
}
