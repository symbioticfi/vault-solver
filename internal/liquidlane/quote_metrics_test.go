package liquidlane

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func boundTestQuoteReason(reason string) string {
	if reason == "unsupported_protocol" {
		return reason
	}
	return "unknown"
}

func TestQuoteMetricsHaveOnlyCoarseFixedDimensions(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewQuoteMetrics(reg, "test", boundTestQuoteReason)
	if err != nil {
		t.Fatal(err)
	}
	for range 10000 {
		metrics.Observe(false, "arbitrary request/error detail")
	}
	metrics.Observe(false, "unsupported_protocol")
	metrics.Observe(true, "arbitrary successful decision")
	if got := testutil.ToFloat64(metrics.results.WithLabelValues("rejected", "unknown")); got != 10000 {
		t.Fatalf("unknown count=%v", got)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].GetName() != "solver_bot_quote_results_total" || len(families[0].GetMetric()) != 3 {
		t.Fatalf("unexpected metric surface: %v", families)
	}
	for _, sample := range families[0].GetMetric() {
		for _, label := range sample.GetLabel() {
			switch label.GetName() {
			case "solver", "result", "reason":
			default:
				t.Fatalf("unbounded dimension=%s", label.GetName())
			}
		}
	}
}

func TestQuoteMetricsShareRegistryAcrossSolvers(t *testing.T) {
	reg := prometheus.NewRegistry()
	for _, name := range []string{"uniswapx-filler", "rfq-filler"} {
		m, err := NewQuoteMetrics(reg, name, boundTestQuoteReason)
		if err != nil {
			t.Fatal(err)
		}
		m.Observe(name == "rfq-filler", "unsupported_protocol")
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || len(families[0].GetMetric()) != 2 {
		t.Fatalf("solvers merged: %v", families)
	}
	for _, sample := range families[0].GetMetric() {
		if sample.GetCounter().GetValue() != 1 {
			t.Fatal("cross-solver double count")
		}
	}
}

func TestQuoteMetricsRegistrationFailure(t *testing.T) {
	reg := prometheus.NewRegistry()
	if _, err := NewQuoteMetrics(reg, "test", boundTestQuoteReason); err != nil {
		t.Fatal(err)
	}
	if _, err := NewQuoteMetrics(reg, "test", boundTestQuoteReason); err == nil {
		t.Fatal("expected duplicate registration failure")
	}
	var disabled *QuoteMetrics
	disabled.Observe(true, "")
}
