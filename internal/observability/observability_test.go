package observability

import "testing"

func TestNewMetricsRegistersBuildAndRuntimeCollectors(t *testing.T) {
	const (
		version = "test-version"
		commit  = "test-commit"
	)

	metrics := NewMetrics(version, commit)

	if metrics.Registerer() != metrics.registry {
		t.Fatal("Registerer did not return the metrics registry")
	}

	families, err := metrics.registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	byName := make(map[string]int, len(families))
	for i, family := range families {
		byName[family.GetName()] = i
	}

	buildInfoIndex, ok := byName["solver_bot_build_info"]
	if !ok {
		t.Fatal("solver_bot_build_info metric not registered")
	}
	buildInfo := families[buildInfoIndex]
	if got, want := buildInfo.GetHelp(), "Build metadata; constant 1, labeled by version and commit."; got != want {
		t.Fatalf("build info help = %q, want %q", got, want)
	}
	buildMetrics := buildInfo.GetMetric()
	if got := len(buildMetrics); got != 1 {
		t.Fatalf("build info metric count = %d, want 1", got)
	}
	metric := buildMetrics[0]
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Fatalf("build info value = %v, want 1", got)
	}
	metricLabels := metric.GetLabel()
	if got := len(metricLabels); got != 2 {
		t.Fatalf("build info label count = %d, want 2", got)
	}
	labels := make(map[string]string, len(metricLabels))
	for _, label := range metricLabels {
		labels[label.GetName()] = label.GetValue()
	}
	if got := labels["version"]; got != version {
		t.Fatalf("version label = %q, want %q", got, version)
	}
	if got := labels["commit"]; got != commit {
		t.Fatalf("commit label = %q, want %q", got, commit)
	}

	for _, name := range []string{"go_goroutines", "process_cpu_seconds_total"} {
		if _, exists := byName[name]; !exists {
			t.Errorf("%s metric not registered", name)
		}
	}
}
