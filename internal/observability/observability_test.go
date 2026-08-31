package observability

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestServiceReadyMetricMirrorsReadinessProbe(t *testing.T) {
	metrics, health := NewMetrics()
	server := NewHTTPServer("", metrics)

	assertReadiness := func(wantReady bool) {
		t.Helper()
		wantMetric := 0.0
		wantStatus := http.StatusServiceUnavailable
		if wantReady {
			wantMetric = 1
			wantStatus = http.StatusOK
		}
		if got := testutil.ToFloat64(metrics.serviceReady); got != wantMetric {
			t.Fatalf("service-ready metric = %v, want %v", got, wantMetric)
		}
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		if got := response.Code; got != wantStatus {
			t.Fatalf("readyz status = %d, want %d", got, wantStatus)
		}
	}

	assertReadiness(false)
	health.SetReady(true)
	assertReadiness(true)
	health.SetReady(false)
	assertReadiness(false)
}

func TestSolverInfoRecordsConfiguredMembership(t *testing.T) {
	metrics, _ := NewMetrics()
	metrics.SetSolvers([]string{"rfq-filler", "redstone-oev"})

	for _, name := range []string{"rfq-filler", "redstone-oev"} {
		if got := testutil.ToFloat64(metrics.solverInfo.WithLabelValues(name)); got != 1 {
			t.Errorf("solver info %q = %v, want 1", name, got)
		}
	}
	if got := testutil.CollectAndCount(metrics.solverInfo); got != 2 {
		t.Fatalf("solver info series = %d, want 2", got)
	}
}

func TestExternalOperationObserversUseOnlyPreboundSeries(t *testing.T) {
	metrics, err := NewWorkflowMetrics(prometheus.NewRegistry(), "lifi", WorkflowSpec{
		Operations: []string{"poll", "refresh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	poll := metrics.Operation("poll")
	poll.Observe(ExternalOperationSuccess, 150*time.Millisecond)
	poll.Observe(0, 250*time.Millisecond)
	poll.Observe(ExternalOperationOutcome(255), 350*time.Millisecond)

	metricstest.RequireHistogram(t, poll.observers[ExternalOperationSuccess], 1, 0.15)
	metricstest.RequireHistogram(t, poll.observers[ExternalOperationDegraded], 0, 0)
	metricstest.RequireHistogram(t, poll.observers[ExternalOperationSkipped], 0, 0)
	metricstest.RequireHistogram(t, poll.observers[ExternalOperationError], 2, 0.6)
	if metrics.Operation("request-derived") != nil {
		t.Fatal("undeclared operation was bound")
	}
}

func TestOperationTimerPreservesCompletedOutcomeAcrossCancellation(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewWorkflowMetrics(reg, "solver", WorkflowSpec{Operations: []string{"poll"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	timer := StartOperation(metrics.Operation("poll"))
	timer.Finish(ctx, ExternalOperationSuccess)
	cancel()
	timer.Finish(ctx, ExternalOperationError)

	cancelledTimer := StartOperation(metrics.Operation("poll"))
	cancelledTimer.Finish(ctx, ExternalOperationError)
	degradedTimer := StartOperation(metrics.Operation("poll"))
	degradedTimer.Finish(ctx, ExternalOperationDegraded)

	metricstest.RequireExternalOperationCount(t, reg, "solver", "poll", "success", 1)
	metricstest.RequireExternalOperationCount(t, reg, "solver", "poll", "degraded", 0)
	metricstest.RequireExternalOperationCount(t, reg, "solver", "poll", "skipped", 2)
	metricstest.RequireExternalOperationCount(t, reg, "solver", "poll", "error", 0)
}

func TestWorkflowMetricsBindAndReportOnlyDeclaredLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	spec := WorkflowSpec{
		Strategy:   "webhook",
		Operations: []string{"poll"},
		Events:     []WorkflowEventSpec{{Event: "quote", Outcomes: []string{"success", "error"}}},
		Amounts: []WorkflowAmountSpec{{
			Event: "quote", Kinds: []string{"input"}, Assets: []string{"0xABC"},
		}},
		States: []string{"offers"},
	}
	metrics, err := NewWorkflowMetrics(reg, "solver-a", spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkflowMetrics(reg, "solver-b", spec); err != nil {
		t.Fatalf("register second solver: %v", err)
	}
	now := time.Unix(123, 0)
	metrics.ObserveEventAt("quote", "success", 2, now)
	metrics.ObserveEventAt("quote", "request-derived", 9, now)
	metrics.AddAmount("quote", "0xABC", "input", big.NewInt(50))
	metrics.AddAmount("quote", "0xABC", "request-derived", big.NewInt(90))
	metrics.ObserveStateAt("offers", 3, now)
	metrics.ObserveStateAt("request-derived", 8, now)

	event := metrics.events[workflowEventKey{event: "quote", outcome: "success"}]
	if count, timestamp := testutil.ToFloat64(event.count), testutil.ToFloat64(event.last); count != 2 || timestamp != 123 {
		t.Fatalf("event = (%v, %v), want (2, 123)", count, timestamp)
	}
	if got := testutil.ToFloat64(metrics.amounts[workflowAmountKey{
		event: "quote", kind: "input",
	}].WithLabelValues("0xabc")); got != 50 {
		t.Fatalf("amount = %v, want 50", got)
	}
	state := metrics.states["offers"]
	if count, timestamp := testutil.ToFloat64(state.value), testutil.ToFloat64(state.last); count != 3 || timestamp != 123 {
		t.Fatalf("state = (%v, %v), want (3, 123)", count, timestamp)
	}
	if got := testutil.CollectAndCount(metrics.events[workflowEventKey{
		event: "quote", outcome: "error",
	}].count); got != 1 {
		t.Fatalf("pre-bound error series = %d, want 1", got)
	}
	for reason, want := range map[string]float64{
		workflowDropUnknownEvent:  1,
		workflowDropUnknownAmount: 1,
		workflowDropUnknownState:  1,
	} {
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_dropped_observations_total", map[string]string{
			"solver": "solver-a", "reason": reason,
		}, want)
	}
}

func TestWorkflowMetricsAllowSharedPreinitializedAssetsAcrossKinds(t *testing.T) {
	reg := prometheus.NewRegistry()
	_, err := NewWorkflowMetrics(reg, "solver", WorkflowSpec{Amounts: []WorkflowAmountSpec{
		{Event: "quote", Kinds: []string{"input"}, Assets: []string{"0xABC"}},
		{Event: "quote", Kinds: []string{"output"}, Assets: []string{"0xABC"}},
	}})
	if err != nil {
		t.Fatalf("register shared route asset: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "solver_bot_workflow_amount_atomic_units_total" && len(family.GetMetric()) == 2 {
			return
		}
	}
	t.Fatal("shared route asset did not preinitialize both amount kinds")
}

func TestWorkflowMetricsRejectInvalidSpecBeforeRegistration(t *testing.T) {
	for _, spec := range []WorkflowSpec{
		{Operations: []string{""}},
		{Operations: []string{"poll", "poll"}},
		{Events: []WorkflowEventSpec{{Event: "", Outcomes: []string{"success"}}}},
		{Events: []WorkflowEventSpec{{Event: "quote", Outcomes: []string{""}}}},
		{Events: []WorkflowEventSpec{{Event: "quote", Outcomes: []string{"ok", "ok"}}}},
		{Amounts: []WorkflowAmountSpec{{Event: "quote"}}},
		{Amounts: []WorkflowAmountSpec{{Event: "quote", Kinds: []string{"input"}, Assets: []string{""}}}},
		{Amounts: []WorkflowAmountSpec{{Event: "quote", Kinds: []string{"input"}, Assets: []string{"0xABC", "0xabc"}}}},
		{States: []string{"offers", "offers"}},
	} {
		reg := prometheus.NewRegistry()
		if _, err := NewWorkflowMetrics(reg, "solver", spec); err == nil {
			t.Fatalf("invalid spec %+v was accepted", spec)
		}
		families, err := reg.Gather()
		if err != nil || len(families) != 0 {
			t.Fatalf("invalid spec registered metrics: families=%d err=%v", len(families), err)
		}
	}
	if _, err := NewWorkflowMetrics(nil, "solver", WorkflowSpec{}); err == nil {
		t.Fatal("nil registerer was accepted")
	}
	if _, err := NewWorkflowMetrics(prometheus.NewRegistry(), "", WorkflowSpec{}); err == nil {
		t.Fatal("empty solver was accepted")
	}
}
