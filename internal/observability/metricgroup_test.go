package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestMetricGroupFailureLeavesParentUntouched(t *testing.T) {
	for _, conflict := range []string{"construction", "publication"} {
		t.Run(conflict, func(t *testing.T) {
			parent := prometheus.NewRegistry()
			group := NewMetricGroup("test_")
			group.Counter("first_total", "first").WithLabelValues().Inc()
			group.Gauge("second", "second").WithLabelValues().Set(1)
			if conflict == "construction" {
				group.Counter("first_total", "first")
			} else {
				parent.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_second", Help: "other schema"}))
			}
			if err := group.Publish(parent); err == nil {
				t.Fatal("conflict accepted")
			}
			families, err := parent.Gather()
			testcheck.NoError(t, err)
			for _, family := range families {
				if family.GetName() == "test_first_total" {
					t.Fatal("partially published component")
				}
			}
		})
	}
}
