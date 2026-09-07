package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestWorkflowRegistrationFailureLeavesNoPartialCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "solver_bot_workflow_observed_items", Help: "conflicting descriptor",
	}))
	_, err := NewWorkflowMetrics(registry, "example", WorkflowSpec{
		Events: []WorkflowEventSpec{{Event: "fill", Outcomes: []string{"success"}}},
		States: []string{"orders"},
	})
	if err == nil {
		t.Fatal("expected registration conflict")
	}
	families, err := registry.Gather()
	testcheck.NoError(t, err)
	if len(families) != 1 || families[0].GetName() != "solver_bot_workflow_observed_items" {
		t.Fatalf("failed registration left partial families: %+v", families)
	}
}
