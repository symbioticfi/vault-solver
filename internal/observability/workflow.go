package observability

import (
	"math/big"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const unspecifiedWorkflowStrategy = "unspecified"

const (
	workflowDropUnknownEvent  = "unknown_event"
	workflowDropUnknownAmount = "unknown_amount"
	workflowDropUnknownState  = "unknown_state"
)

type workflowEventKey struct{ event, outcome string }
type workflowAmountKey struct{ event, kind string }

type WorkflowEventSpec struct {
	Event    string
	Outcomes []string
}

type WorkflowAmountSpec struct {
	Event  string
	Kinds  []string
	Assets []string
}

type WorkflowSpec struct {
	Strategy   string
	Operations []string
	Events     []WorkflowEventSpec
	Amounts    []WorkflowAmountSpec
	States     []string
}

type workflowEventMetrics struct {
	count prometheus.Counter
	last  prometheus.Gauge
}

type workflowStateMetrics struct {
	value prometheus.Gauge
	last  prometheus.Gauge
}

// WorkflowMetrics binds every non-asset label at construction, keeping runtime observations bounded.
type WorkflowMetrics struct {
	operations map[string]*OperationObserver
	events     map[workflowEventKey]workflowEventMetrics
	amounts    map[workflowAmountKey]*prometheus.CounterVec
	states     map[string]workflowStateMetrics
	dropped    *prometheus.CounterVec
}

func NewWorkflowMetrics(reg prometheus.Registerer, solver string, spec WorkflowSpec) (*WorkflowMetrics, error) {
	if reg == nil {
		return nil, errors.New("observability: workflow metrics registerer is required")
	}
	if solver == "" {
		return nil, errors.New("observability: workflow solver is required")
	}
	strategy := spec.Strategy
	if strategy == "" {
		strategy = unspecifiedWorkflowStrategy
	}
	wrapped := prometheus.WrapRegistererWith(prometheus.Labels{"solver": solver, "strategy": strategy}, reg)
	events := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "solver_bot", Name: "workflow_events_total",
		Help: "Bounded solver workflow events by integration-owned event and outcome.",
	}, []string{"event", "outcome"})
	lastEvents := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot", Name: "workflow_last_event_timestamp",
		Help: "Unix timestamp of the last bounded solver workflow event by event and outcome.",
	}, []string{"event", "outcome"})
	amounts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "solver_bot", Name: "workflow_amount_atomic_units_total",
		Help: "Solver workflow amounts in asset atomic units; assets and kinds must not be aggregated across unlike units.",
	}, []string{"event", "asset", "kind"})
	states := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot", Name: "workflow_observed_items",
		Help: "Items in the last complete solver workflow observation by integration-owned state view.",
	}, []string{"view"})
	lastStates := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot", Name: "workflow_last_observation_timestamp",
		Help: "Unix timestamp of the last complete solver workflow observation by state view.",
	}, []string{"view"})
	operations := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "solver_bot", Name: "external_operation_duration_seconds",
		Help:    "External operation duration by solver, allowlisted operation, and bounded outcome.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"operation", "outcome"})
	dropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "solver_bot", Name: "workflow_dropped_observations_total",
		Help: "Workflow observations rejected because their event, amount, or state dimension was not declared.",
	}, []string{"reason"})
	metrics := &WorkflowMetrics{
		operations: make(map[string]*OperationObserver, len(spec.Operations)),
		events:     make(map[workflowEventKey]workflowEventMetrics),
		amounts:    make(map[workflowAmountKey]*prometheus.CounterVec),
		states:     make(map[string]workflowStateMetrics, len(spec.States)),
		dropped:    dropped,
	}

	if err := metrics.bindOperations(operations, spec.Operations); err != nil {
		return nil, err
	}
	if err := metrics.bindEvents(events, lastEvents, spec.Events); err != nil {
		return nil, err
	}
	if err := metrics.bindAmounts(amounts, spec.Amounts); err != nil {
		return nil, err
	}
	if err := metrics.bindStates(states, lastStates, spec.States); err != nil {
		return nil, err
	}
	for _, reason := range []string{workflowDropUnknownEvent, workflowDropUnknownAmount, workflowDropUnknownState} {
		dropped.WithLabelValues(reason)
	}
	if err := RegisterCollectors(wrapped, "observability: workflow",
		events, lastEvents, amounts, states, lastStates, operations, dropped,
	); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (m *WorkflowMetrics) bindOperations(metric *prometheus.HistogramVec, operations []string) error {
	for _, operation := range operations {
		if operation == "" {
			return errors.New("observability: workflow operation label is required")
		}
		if _, exists := m.operations[operation]; exists {
			return errors.Errorf("observability: duplicate workflow operation %q", operation)
		}
		observer := &OperationObserver{}
		for outcome := ExternalOperationSuccess; outcome <= ExternalOperationError; outcome++ {
			observer.observers[outcome] = metric.WithLabelValues(operation, externalOperationOutcomeLabels[outcome])
		}
		m.operations[operation] = observer
	}
	return nil
}

func (m *WorkflowMetrics) bindEvents(counts *prometheus.CounterVec, last *prometheus.GaugeVec, events []WorkflowEventSpec) error {
	for _, event := range events {
		if event.Event == "" || len(event.Outcomes) == 0 {
			return errors.New("observability: workflow event and outcomes are required")
		}
		for _, outcome := range event.Outcomes {
			key := workflowEventKey{event.Event, outcome}
			if outcome == "" {
				return errors.New("observability: workflow event outcome is required")
			}
			if _, exists := m.events[key]; exists {
				return errors.Errorf("observability: duplicate workflow event %q", event.Event+"/"+outcome)
			}
			m.events[key] = workflowEventMetrics{
				count: counts.WithLabelValues(event.Event, outcome),
				last:  last.WithLabelValues(event.Event, outcome),
			}
		}
	}
	return nil
}

func (m *WorkflowMetrics) bindAmounts(metric *prometheus.CounterVec, amounts []WorkflowAmountSpec) error {
	for _, amount := range amounts {
		if amount.Event == "" || len(amount.Kinds) == 0 {
			return errors.New("observability: workflow amount event and kinds are required")
		}
		for _, kind := range amount.Kinds {
			key := workflowAmountKey{amount.Event, kind}
			if kind == "" {
				return errors.New("observability: workflow amount kind is required")
			}
			if _, exists := m.amounts[key]; exists {
				return errors.Errorf("observability: duplicate workflow amount %q", amount.Event+"/"+kind)
			}
			bound := metric.MustCurryWith(prometheus.Labels{"event": amount.Event, "kind": kind})
			m.amounts[key] = bound
			assets := make(map[string]struct{}, len(amount.Assets))
			for _, asset := range amount.Assets {
				asset = strings.ToLower(asset)
				if asset == "" {
					return errors.New("observability: workflow amount asset is required")
				}
				if _, exists := assets[asset]; exists {
					return errors.Errorf("observability: duplicate workflow amount asset %q", asset)
				}
				assets[asset] = struct{}{}
				bound.WithLabelValues(asset)
			}
		}
	}
	return nil
}

func (m *WorkflowMetrics) bindStates(values, last *prometheus.GaugeVec, states []string) error {
	for _, state := range states {
		if state == "" {
			return errors.New("observability: workflow state label is required")
		}
		if _, exists := m.states[state]; exists {
			return errors.Errorf("observability: duplicate workflow state %q", state)
		}
		m.states[state] = workflowStateMetrics{
			value: values.WithLabelValues(state),
			last:  last.WithLabelValues(state),
		}
	}
	return nil
}

func (m *WorkflowMetrics) Operation(name string) *OperationObserver {
	if m == nil {
		return nil
	}
	return m.operations[name]
}

func (m *WorkflowMetrics) ObserveEventAt(event, outcome string, count float64, at time.Time) {
	if m == nil || count <= 0 {
		return
	}
	metric, ok := m.events[workflowEventKey{event, outcome}]
	if !ok {
		m.dropped.WithLabelValues(workflowDropUnknownEvent).Inc()
		return
	}
	metric.count.Add(count)
	metric.last.Set(float64(at.Unix()))
}

func (m *WorkflowMetrics) ObserveEvent(event, outcome string) {
	m.ObserveEventAt(event, outcome, 1, time.Now())
}

func (m *WorkflowMetrics) AddAmount(event, asset, kind string, amount *big.Int) {
	if m == nil || asset == "" || amount == nil || amount.Sign() <= 0 {
		return
	}
	metric, ok := m.amounts[workflowAmountKey{event, kind}]
	if !ok {
		m.dropped.WithLabelValues(workflowDropUnknownAmount).Inc()
		return
	}
	value, _ := new(big.Float).SetInt(amount).Float64()
	metric.WithLabelValues(strings.ToLower(asset)).Add(value)
}

func (m *WorkflowMetrics) ObserveStateAt(view string, count int, at time.Time) {
	if m == nil {
		return
	}
	metric, ok := m.states[view]
	if !ok {
		m.dropped.WithLabelValues(workflowDropUnknownState).Inc()
		return
	}
	metric.value.Set(float64(count))
	metric.last.Set(float64(at.Unix()))
}
