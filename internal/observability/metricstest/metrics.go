// Package metricstest contains assertions shared by Prometheus instrumentation tests.
package metricstest

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

const (
	externalOperationFamily = "solver_bot_external_operation_duration_seconds"
	workflowEventsFamily    = "solver_bot_workflow_events_total"
	workflowLastEventFamily = "solver_bot_workflow_last_event_timestamp"
	workflowAmountsFamily   = "solver_bot_workflow_amount_atomic_units_total"
	workflowItemsFamily     = "solver_bot_workflow_observed_items"
	workflowLastStateFamily = "solver_bot_workflow_last_observation_timestamp"
)

// RequireValue checks the single sample exposed by a collector.
func RequireValue(tb testing.TB, collector prometheus.Collector, want float64) {
	tb.Helper()
	if got := testutil.ToFloat64(collector); got != want {
		tb.Fatalf("metric value = %v, want %v", got, want)
	}
}

// RequireFamilyValue checks one counter or gauge selected by an exact label subset.
func RequireFamilyValue(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	familyName string,
	labels map[string]string,
	want float64,
) {
	tb.Helper()
	if got := FamilyValue(tb, gatherer, familyName, labels); got != want {
		tb.Fatalf("%s%v = %v, want %v", familyName, labels, got, want)
	}
}

// FamilyValue returns one counter or gauge selected by an exact label subset.
func FamilyValue(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	familyName string,
	labels map[string]string,
) float64 {
	tb.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		tb.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != familyName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !hasLabels(metric, labels) {
				continue
			}
			switch family.GetType() {
			case dto.MetricType_COUNTER:
				return metric.GetCounter().GetValue()
			case dto.MetricType_GAUGE:
				return metric.GetGauge().GetValue()
			case dto.MetricType_SUMMARY, dto.MetricType_UNTYPED, dto.MetricType_HISTOGRAM,
				dto.MetricType_GAUGE_HISTOGRAM:
				tb.Fatalf("metric family %s has unsupported type %s", familyName, family.GetType())
			}
		}
	}
	tb.Fatalf("missing metric %s%v", familyName, labels)
	return 0
}

// HistogramCount returns the sample count exposed by a histogram observer.
func HistogramCount(tb testing.TB, observer prometheus.Observer) uint64 {
	tb.Helper()
	return histogram(tb, observer).GetSampleCount()
}

// RequireHistogram checks a histogram's sample count and sum.
func RequireHistogram(tb testing.TB, observer prometheus.Observer, wantCount uint64, wantSum float64) {
	tb.Helper()
	value := histogram(tb, observer)
	if got := value.GetSampleCount(); got != wantCount {
		tb.Fatalf("histogram count = %d, want %d", got, wantCount)
	}
	if got := value.GetSampleSum(); got != wantSum {
		tb.Fatalf("histogram sum = %v, want %v", got, wantSum)
	}
}

func histogram(tb testing.TB, observer prometheus.Observer) *dto.Histogram {
	tb.Helper()
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		tb.Fatal("histogram observer does not implement prometheus.Metric")
	}
	var value dto.Metric
	if err := metric.Write(&value); err != nil {
		tb.Fatalf("write histogram: %v", err)
	}
	return value.GetHistogram()
}

func hasLabels(metric *dto.Metric, want map[string]string) bool {
	matched := 0
	for _, label := range metric.GetLabel() {
		if value, ok := want[label.GetName()]; ok && value == label.GetValue() {
			matched++
		}
	}
	return matched == len(want)
}

// RequireWorkflowEvent checks one pre-bound event count and timestamp.
func RequireWorkflowEvent(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	solver, event, outcome string,
	count, timestamp float64,
) {
	tb.Helper()
	labels := map[string]string{"solver": solver, "event": event, "outcome": outcome}
	RequireWorkflowEventCount(tb, gatherer, solver, event, outcome, count)
	RequireFamilyValue(tb, gatherer, workflowLastEventFamily, labels, timestamp)
}

// RequireWorkflowEventCount checks one pre-bound event counter.
func RequireWorkflowEventCount(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	solver, event, outcome string,
	count float64,
) {
	tb.Helper()
	RequireFamilyValue(tb, gatherer, workflowEventsFamily, map[string]string{
		"solver": solver, "event": event, "outcome": outcome,
	}, count)
}

// RequireWorkflowAmount checks one asset/kind amount counter.
func RequireWorkflowAmount(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	solver, event, asset, kind string,
	want float64,
) {
	tb.Helper()
	RequireFamilyValue(tb, gatherer, workflowAmountsFamily, map[string]string{
		"solver": solver, "event": event, "asset": asset, "kind": kind,
	}, want)
}

// RequireWorkflowState checks one complete state count and timestamp.
func RequireWorkflowState(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	solver, view string,
	count, timestamp float64,
) {
	tb.Helper()
	labels := map[string]string{"solver": solver, "view": view}
	RequireFamilyValue(tb, gatherer, workflowItemsFamily, labels, count)
	RequireFamilyValue(tb, gatherer, workflowLastStateFamily, labels, timestamp)
}

// RequireExternalOperationCount checks one pre-bound solver/operation/outcome series.
func RequireExternalOperationCount(
	tb testing.TB,
	gatherer prometheus.Gatherer,
	solver, operation, outcome string,
	want uint64,
) {
	tb.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		tb.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != externalOperationFamily {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !hasLabels(metric, map[string]string{
				"solver": solver, "operation": operation, "outcome": outcome,
			}) {
				continue
			}
			if got := metric.GetHistogram().GetSampleCount(); got != want {
				tb.Fatalf("%s/%s/%s count = %d, want %d", solver, operation, outcome, got, want)
			}
			return
		}
	}
	tb.Fatalf("missing external operation series %s/%s/%s", solver, operation, outcome)
}
