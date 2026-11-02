package liquidlane

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestFillMetricsObserve(t *testing.T) {
	reg := prometheus.NewRegistry()
	spec := FillWorkflowSpec()
	spec.Events = append(spec.Events, observability.WorkflowEventSpec{
		Event: "fill", Outcomes: []string{FillOutcomeFailure, FillOutcomeNotAdmitted},
	})
	workflow, err := observability.NewWorkflowMetrics(reg, "rfq", spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observability.NewWorkflowMetrics(reg, "lifi", FillWorkflowSpec()); err != nil {
		t.Fatalf("register second solver: %v", err)
	}
	rfq := NewFillMetrics(workflow)
	rfq.now = func() time.Time { return time.Unix(123, 0) }
	rfq.Observe(&types.Receipt{Status: types.ReceiptStatusSuccessful})
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", FillOutcomeSuccess, 1, 123)
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", FillOutcomeFailure, 0, 0)
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", FillOutcomeNotAdmitted, 0, 0)
	metricstest.RequireNoWorkflowAmounts(t, reg, "rfq")

	rfq.Observe(&types.Receipt{Status: types.ReceiptStatusFailed})
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", FillOutcomeSuccess, 1, 123)

	rfq.ObserveOutcome(FillOutcomeFailure)
	rfq.ObserveOutcome(FillOutcomeNotAdmitted)
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", FillOutcomeFailure, 1, 123)
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", FillOutcomeNotAdmitted, 1, 123)
}

func TestPlannedSurplus(t *testing.T) {
	if got := PlannedSurplus(big.NewInt(110), big.NewInt(100)); got.String() != "10" {
		t.Fatalf("surplus = %s, want 10", got)
	}
	if got := PlannedSurplus(big.NewInt(100), big.NewInt(100)); got.Sign() != 0 {
		t.Fatalf("zero surplus = %s", got)
	}
}
