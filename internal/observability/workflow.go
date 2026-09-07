package observability

import (
	"math"
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

// WorkflowSpec is the complete label vocabulary of one integration. Observations
// select declared series; only validated asset addresses are added dynamically.
type WorkflowSpec struct {
	Strategy   string
	Operations []string
	Events     []WorkflowEventSpec
	Amounts    []WorkflowAmountSpec
	States     []string
}

type WorkflowEventSpec struct {
	Event    string
	Outcomes []string
}

type WorkflowAmountSpec struct {
	Event  string
	Kinds  []string
	Assets []string
}

type workflowEventKey struct{ event, outcome string }
type workflowAmountKey struct{ event, kind string }

// The vocabulary is immutable after construction. Prometheus owns synchronization
// of counters/gauges; callers never mutate a registry or grow outcome dimensions.
type WorkflowMetrics struct {
	operations  map[string]*OperationObserver
	events      map[workflowEventKey]struct{}
	amounts     map[workflowAmountKey]struct{}
	states      map[string]struct{}
	eventCount  *prometheus.CounterVec
	eventTime   *prometheus.GaugeVec
	amountCount *prometheus.CounterVec
	stateCount  *prometheus.GaugeVec
	stateTime   *prometheus.GaugeVec
	dropped     *prometheus.CounterVec
}

func NewWorkflowMetrics(reg prometheus.Registerer, solver string, spec WorkflowSpec) (*WorkflowMetrics, error) {
	if reg == nil || solver == "" {
		return nil, errors.New("observability: workflow registerer and solver are required")
	}
	group := NewMetricGroup("solver_bot_")
	m := &WorkflowMetrics{
		operations: make(map[string]*OperationObserver),
		events:     make(map[workflowEventKey]struct{}), amounts: make(map[workflowAmountKey]struct{}),
		states:      make(map[string]struct{}),
		eventCount:  group.Counter("workflow_events_total", "Bounded solver workflow events by integration-owned event and outcome.", "event", "outcome"),
		eventTime:   group.Gauge("workflow_last_event_timestamp", "Unix timestamp of the last bounded solver workflow event by event and outcome.", "event", "outcome"),
		amountCount: group.Counter("workflow_amount_atomic_units_total", "Solver workflow amounts in asset atomic units; assets and kinds must not be aggregated across unlike units.", "event", "asset", "kind"),
		stateCount:  group.Gauge("workflow_observed_items", "Items in the last complete solver workflow observation by integration-owned state view.", "view"),
		stateTime:   group.Gauge("workflow_last_observation_timestamp", "Unix timestamp of the last complete solver workflow observation by state view.", "view"),
		dropped:     group.Counter("workflow_dropped_observations_total", "Workflow observations rejected because their event, amount, or state dimension was not declared.", "reason"),
	}
	operations := group.Histogram("external_operation_duration_seconds",
		"External operation duration by solver, allowlisted operation, and bounded outcome.",
		[]float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}, "operation", "outcome")
	if err := m.bind(spec, operations); err != nil {
		return nil, err
	}
	for _, reason := range []string{workflowDropUnknownEvent, workflowDropUnknownAmount, workflowDropUnknownState} {
		m.dropped.WithLabelValues(reason)
	}
	strategy := spec.Strategy
	if strategy == "" {
		strategy = unspecifiedWorkflowStrategy
	}
	if err := group.Publish(prometheus.WrapRegistererWith(prometheus.Labels{"solver": solver, "strategy": strategy}, reg)); err != nil {
		return nil, err
	}
	return m, nil
}

// Bind validates and instantiates the vocabulary in the same pass. No caller can
// observe an incompletely bound component because publication happens afterwards.
func (m *WorkflowMetrics) bind(spec WorkflowSpec, durations *prometheus.HistogramVec) error {
	for _, name := range spec.Operations {
		if name == "" || m.operations[name] != nil {
			return errors.Errorf("observability: empty or duplicate workflow operation %q", name)
		}
		observer := &OperationObserver{}
		for outcome := ExternalOperationSuccess; outcome <= ExternalOperationError; outcome++ {
			observer.observers[outcome] = durations.WithLabelValues(name, externalOperationOutcomeLabels[outcome])
		}
		m.operations[name] = observer
	}
	for _, event := range spec.Events {
		if event.Event == "" || len(event.Outcomes) == 0 {
			return errors.New("observability: workflow event and outcomes are required")
		}
		for _, outcome := range event.Outcomes {
			key := workflowEventKey{event.Event, outcome}
			if _, exists := m.events[key]; outcome == "" || exists {
				return errors.Errorf("observability: empty or duplicate workflow event %q/%q", event.Event, outcome)
			}
			m.events[key] = struct{}{}
			m.eventCount.WithLabelValues(event.Event, outcome)
			m.eventTime.WithLabelValues(event.Event, outcome)
		}
	}
	for _, amount := range spec.Amounts {
		if amount.Event == "" || len(amount.Kinds) == 0 {
			return errors.New("observability: workflow amount event and kinds are required")
		}
		for _, kind := range amount.Kinds {
			key := workflowAmountKey{amount.Event, kind}
			if _, exists := m.amounts[key]; kind == "" || exists {
				return errors.Errorf("observability: empty or duplicate workflow amount %q/%q", amount.Event, kind)
			}
			m.amounts[key] = struct{}{}
			assets := make(map[string]struct{}, len(amount.Assets))
			for _, raw := range amount.Assets {
				asset := strings.ToLower(raw)
				if _, exists := assets[asset]; asset == "" || exists {
					return errors.Errorf("observability: empty or duplicate workflow amount asset %q", raw)
				}
				assets[asset] = struct{}{}
				m.amountCount.WithLabelValues(amount.Event, asset, kind)
			}
		}
	}
	for _, view := range spec.States {
		if _, exists := m.states[view]; view == "" || exists {
			return errors.Errorf("observability: empty or duplicate workflow state %q", view)
		}
		m.states[view] = struct{}{}
		m.stateCount.WithLabelValues(view)
		m.stateTime.WithLabelValues(view)
	}
	return nil
}

func (m *WorkflowMetrics) Operation(name string) *OperationObserver {
	var observer *OperationObserver
	if m != nil {
		observer = m.operations[name]
	}
	return observer
}

func (m *WorkflowMetrics) ObserveEvent(event, outcome string) {
	m.ObserveEventAt(event, outcome, 1, time.Now())
}

func (m *WorkflowMetrics) ObserveEventAt(event, outcome string, count float64, at time.Time) {
	if m == nil || !(count > 0) || math.IsInf(count, 0) {
		return
	}
	if _, allowed := m.events[workflowEventKey{event, outcome}]; !allowed {
		m.dropped.WithLabelValues(workflowDropUnknownEvent).Inc()
		return
	}
	m.eventCount.WithLabelValues(event, outcome).Add(count)
	m.eventTime.WithLabelValues(event, outcome).Set(float64(at.Unix()))
}

func (m *WorkflowMetrics) AddAmount(event, asset, kind string, amount *big.Int) {
	if m == nil || asset == "" || amount == nil || amount.Sign() <= 0 {
		return
	}
	if _, allowed := m.amounts[workflowAmountKey{event, kind}]; !allowed {
		m.dropped.WithLabelValues(workflowDropUnknownAmount).Inc()
		return
	}
	value, _ := new(big.Float).SetInt(amount).Float64()
	m.amountCount.WithLabelValues(event, strings.ToLower(asset), kind).Add(value)
}

func (m *WorkflowMetrics) ObserveStateAt(view string, count int, at time.Time) {
	if m == nil {
		return
	}
	if _, allowed := m.states[view]; !allowed {
		m.dropped.WithLabelValues(workflowDropUnknownState).Inc()
		return
	}
	m.stateCount.WithLabelValues(view).Set(float64(count))
	m.stateTime.WithLabelValues(view).Set(float64(at.Unix()))
}
