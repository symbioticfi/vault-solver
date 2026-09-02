package observability

import (
	"math/big"
	"slices"
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

type workflowEventKey struct {
	event   string
	outcome string
}

type workflowAmountKey struct {
	event string
	kind  string
}

// WorkflowEventSpec declares the bounded outcomes for one integration-owned event.
type WorkflowEventSpec struct {
	Event    string
	Outcomes []string
}

// WorkflowAmountSpec declares the bounded amount kinds for one integration-owned event. Assets may
// preinitialize configured or authoritative routes; observations can add other validated route assets.
type WorkflowAmountSpec struct {
	Event  string
	Kinds  []string
	Assets []string
}

// WorkflowSpec declares one solver's complete bounded workflow metric surface.
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

// WorkflowMetrics records homogeneous solver events, amounts, and observed item counts through
// shared metric families. All event, outcome, kind, and state labels are bound from WorkflowSpec at
// construction; runtime input can only select an existing series.
type WorkflowMetrics struct {
	operations map[string]*OperationObserver
	events     map[workflowEventKey]workflowEventMetrics
	amounts    map[workflowAmountKey]*prometheus.CounterVec
	states     map[string]workflowStateMetrics
	dropped    map[string]prometheus.Counter
}

// NewWorkflowMetrics registers one solver's bounded workflow metric surface.
func NewWorkflowMetrics(
	reg prometheus.Registerer,
	solver string,
	spec WorkflowSpec,
) (*WorkflowMetrics, error) {
	if reg == nil {
		return nil, errors.New("observability: workflow metrics registerer is required")
	}
	if solver == "" {
		return nil, errors.New("observability: workflow solver is required")
	}
	if err := validateWorkflowSpec(spec); err != nil {
		return nil, err
	}
	strategy := spec.Strategy
	if strategy == "" {
		strategy = unspecifiedWorkflowStrategy
	}
	wrapped := prometheus.WrapRegistererWith(prometheus.Labels{
		"solver": solver, "strategy": strategy,
	}, reg)
	events := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "solver_bot",
		Name:      "workflow_events_total",
		Help:      "Bounded solver workflow events by integration-owned event and outcome.",
	}, []string{"event", "outcome"})
	lastEvents := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "workflow_last_event_timestamp",
		Help:      "Unix timestamp of the last bounded solver workflow event by event and outcome.",
	}, []string{"event", "outcome"})
	amounts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "solver_bot",
		Name:      "workflow_amount_atomic_units_total",
		Help:      "Solver workflow amounts in asset atomic units; assets and kinds must not be aggregated across unlike units.",
	}, []string{"event", "asset", "kind"})
	states := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "workflow_observed_items",
		Help:      "Items in the last complete solver workflow observation by integration-owned state view.",
	}, []string{"view"})
	lastStates := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "workflow_last_observation_timestamp",
		Help:      "Unix timestamp of the last complete solver workflow observation by state view.",
	}, []string{"view"})
	operations := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "solver_bot",
		Name:      "external_operation_duration_seconds",
		Help:      "External operation duration by solver, allowlisted operation, and bounded outcome.",
		Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"operation", "outcome"})
	dropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "solver_bot",
		Name:      "workflow_dropped_observations_total",
		Help:      "Workflow observations rejected because their event, amount, or state dimension was not declared.",
	}, []string{"reason"})
	for _, collector := range []prometheus.Collector{
		events, lastEvents, amounts, states, lastStates, operations, dropped,
	} {
		if err := wrapped.Register(collector); err != nil {
			return nil, errors.Errorf("observability: register workflow metric: %w", err)
		}
	}

	metrics := &WorkflowMetrics{
		operations: make(map[string]*OperationObserver),
		events:     make(map[workflowEventKey]workflowEventMetrics),
		amounts:    make(map[workflowAmountKey]*prometheus.CounterVec),
		states:     make(map[string]workflowStateMetrics),
		dropped: map[string]prometheus.Counter{
			workflowDropUnknownEvent:  dropped.WithLabelValues(workflowDropUnknownEvent),
			workflowDropUnknownAmount: dropped.WithLabelValues(workflowDropUnknownAmount),
			workflowDropUnknownState:  dropped.WithLabelValues(workflowDropUnknownState),
		},
	}
	for _, operation := range spec.Operations {
		observer := &OperationObserver{}
		for outcome := ExternalOperationSuccess; outcome <= ExternalOperationError; outcome++ {
			observer.observers[outcome] = operations.WithLabelValues(
				operation, externalOperationOutcomeLabels[outcome],
			)
		}
		metrics.operations[operation] = observer
	}
	for _, event := range spec.Events {
		for _, outcome := range event.Outcomes {
			key := workflowEventKey{event: event.Event, outcome: outcome}
			metrics.events[key] = workflowEventMetrics{
				count: events.WithLabelValues(event.Event, outcome),
				last:  lastEvents.WithLabelValues(event.Event, outcome),
			}
		}
	}
	for _, amount := range spec.Amounts {
		for _, kind := range amount.Kinds {
			key := workflowAmountKey{event: amount.Event, kind: kind}
			bound := amounts.MustCurryWith(prometheus.Labels{"event": amount.Event, "kind": kind})
			metrics.amounts[key] = bound
			for _, asset := range amount.Assets {
				bound.WithLabelValues(strings.ToLower(asset))
			}
		}
	}
	for _, state := range spec.States {
		metrics.states[state] = workflowStateMetrics{
			value: states.WithLabelValues(state),
			last:  lastStates.WithLabelValues(state),
		}
	}
	return metrics, nil
}

func validateWorkflowSpec(spec WorkflowSpec) error {
	seen := make(map[string]struct{})
	add := func(kind string, parts ...string) error {
		if slices.Contains(parts, "") {
			return errors.Errorf("observability: workflow %s label is required", kind)
		}
		key := kind + "\x00" + strings.Join(parts, "\x00")
		if _, exists := seen[key]; exists {
			return errors.Errorf("observability: duplicate workflow %s %q", kind, strings.Join(parts, "/"))
		}
		seen[key] = struct{}{}
		return nil
	}
	for _, operation := range spec.Operations {
		if err := add("operation", operation); err != nil {
			return err
		}
	}
	for _, event := range spec.Events {
		if len(event.Outcomes) == 0 {
			return errors.New("observability: workflow event outcomes are required")
		}
		for _, outcome := range event.Outcomes {
			if err := add("event", event.Event, outcome); err != nil {
				return err
			}
		}
	}
	for _, amount := range spec.Amounts {
		if len(amount.Kinds) == 0 {
			return errors.New("observability: workflow amount kinds are required")
		}
		for _, kind := range amount.Kinds {
			if err := add("amount", amount.Event, kind); err != nil {
				return err
			}
			for _, asset := range amount.Assets {
				if err := add("amount asset", amount.Event, kind, strings.ToLower(asset)); err != nil {
					return err
				}
			}
		}
	}
	for _, state := range spec.States {
		if err := add("state", state); err != nil {
			return err
		}
	}
	return nil
}

// Operation returns one construction-time-bound dependency observer.
func (m *WorkflowMetrics) Operation(name string) *OperationObserver {
	if m == nil {
		return nil
	}
	return m.operations[name]
}

// ObserveEventAt adds a positive event count and updates its timestamp. Unknown event/outcome pairs
// increment a bounded drop counter rather than creating runtime-derived labels.
func (m *WorkflowMetrics) ObserveEventAt(event, outcome string, count float64, at time.Time) {
	if m == nil || count <= 0 {
		return
	}
	metric, ok := m.events[workflowEventKey{event: event, outcome: outcome}]
	if !ok {
		m.dropped[workflowDropUnknownEvent].Inc()
		return
	}
	metric.count.Add(count)
	metric.last.Set(float64(at.Unix()))
}

// ObserveEvent records one event at the current wall-clock time.
func (m *WorkflowMetrics) ObserveEvent(event, outcome string) {
	m.ObserveEventAt(event, outcome, 1, time.Now())
}

// AddAmount adds a positive atomic-unit amount to a pre-bound event/kind pair. Empty assets are
// ignored, unknown pairs increment a bounded drop counter, and asset labels are normalized to lowercase.
func (m *WorkflowMetrics) AddAmount(event, asset, kind string, amount *big.Int) {
	if m == nil || asset == "" || amount == nil || amount.Sign() <= 0 {
		return
	}
	metric, ok := m.amounts[workflowAmountKey{event: event, kind: kind}]
	if !ok {
		m.dropped[workflowDropUnknownAmount].Inc()
		return
	}
	value, _ := new(big.Float).SetInt(amount).Float64()
	metric.WithLabelValues(strings.ToLower(asset)).Add(value)
}

// ObserveStateAt publishes one complete state count and its observation timestamp. Unknown views
// increment a bounded drop counter rather than creating runtime-derived labels.
func (m *WorkflowMetrics) ObserveStateAt(view string, count int, at time.Time) {
	if m == nil {
		return
	}
	metric, ok := m.states[view]
	if !ok {
		m.dropped[workflowDropUnknownState].Inc()
		return
	}
	metric.value.Set(float64(count))
	metric.last.Set(float64(at.Unix()))
}
