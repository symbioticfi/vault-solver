// Package metricstest contains assertions shared by Prometheus instrumentation tests.
package metricstest

import (
	"math"
	"strings"
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

// RequireFamilyValue checks the unique counter or gauge selected by a label subset.
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

// FamilyValue returns the unique counter or gauge selected by a label subset. It fails when the
// subset is ambiguous, so an assertion cannot accidentally validate an arbitrary strategy or series.
func FamilyValue(tb testing.TB, gatherer prometheus.Gatherer, familyName string, labels map[string]string) float64 {
	tb.Helper()
	metric := selectedMetric(tb, gatherer, familyName, labels)
	if metric.GetCounter() != nil {
		return metric.GetCounter().GetValue()
	}
	if metric.GetGauge() != nil {
		return metric.GetGauge().GetValue()
	}
	tb.Fatalf("metric family %s is not a counter or gauge", familyName)
	return 0
}

func selectedMetric(tb testing.TB, gatherer prometheus.Gatherer, familyName string, labels map[string]string) *dto.Metric {
	tb.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		tb.Fatalf("gather metrics: %v", err)
	}
	matches := make([]*dto.Metric, 0, 1)
	for _, family := range families {
		if family.GetName() == familyName {
			for _, sample := range family.GetMetric() {
				if hasLabels(sample, labels) {
					matches = append(matches, sample)
				}
			}
		}
	}
	if len(matches) != 1 {
		tb.Fatalf("metric %s%v matched %d series, want exactly one", familyName, labels, len(matches))
		return nil
	}
	return matches[0]
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
	if got := value.GetSampleSum(); math.Abs(got-wantSum) > 1e-12*max(1, math.Abs(got), math.Abs(wantSum)) {
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
		"solver": solver, "event": event, "asset": strings.ToLower(asset), "kind": kind,
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
func RequireExternalOperationCount(tb testing.TB, gatherer prometheus.Gatherer, solver, operation, outcome string, want uint64) {
	tb.Helper()
	labels := map[string]string{"solver": solver, "operation": operation, "outcome": outcome}
	sample := selectedMetric(tb, gatherer, externalOperationFamily, labels)
	if sample.GetHistogram() == nil {
		tb.Fatalf("external operation %v is not a histogram", labels)
	}
	if got := sample.GetHistogram().GetSampleCount(); got != want {
		tb.Fatalf("external operation %v count = %d, want %d", labels, got, want)
	}
}
