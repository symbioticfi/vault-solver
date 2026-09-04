package observability

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestWorkflowAcceptsOnlyDeclaredEvents(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewWorkflowMetrics(registry, "test", WorkflowSpec{
		Strategy: "compact", Events: []WorkflowEventSpec{{Event: "quote", Outcomes: []string{"quoted"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	metrics.ObserveEvent("quote", "quoted")
	metrics.ObserveEvent("quote", "request-derived")

	want := `
# HELP solver_bot_workflow_dropped_observations_total Workflow observations rejected because their event, amount, or state dimension was not declared.
# TYPE solver_bot_workflow_dropped_observations_total counter
solver_bot_workflow_dropped_observations_total{reason="unknown_amount",solver="test",strategy="compact"} 0
solver_bot_workflow_dropped_observations_total{reason="unknown_event",solver="test",strategy="compact"} 1
solver_bot_workflow_dropped_observations_total{reason="unknown_state",solver="test",strategy="compact"} 0
# HELP solver_bot_workflow_events_total Bounded solver workflow events by integration-owned event and outcome.
# TYPE solver_bot_workflow_events_total counter
solver_bot_workflow_events_total{event="quote",outcome="quoted",solver="test",strategy="compact"} 1
`
	if err := testutil.GatherAndCompare(
		registry, strings.NewReader(want),
		"solver_bot_workflow_events_total", "solver_bot_workflow_dropped_observations_total",
	); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowRejectsInvalidSpecBeforeRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()
	invalid := WorkflowSpec{Operations: []string{"poll", "poll"}}
	if _, err := NewWorkflowMetrics(registry, "test", invalid); err == nil {
		t.Fatal("duplicate operation accepted")
	}
	if _, err := NewWorkflowMetrics(registry, "test", WorkflowSpec{Operations: []string{"poll"}}); err != nil {
		t.Fatalf("invalid spec registered partial collectors: %v", err)
	}
}
