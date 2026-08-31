package observability

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
)

func TestServeUntilCompletesOnCancellation(t *testing.T) {
	addr := unusedLoopbackAddr(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var logs []string
	log := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
	srv := NewHTTPServer(addr, NewMetrics("test", "test"), &Health{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ServeUntil(ctx, srv, log)
	}()

	waitForHealth(t, "http://"+addr+"/healthz")
	cancel()
	waitForServer(t, done)
	if len(logs) != 0 {
		t.Fatalf("ServeUntil logged on cancellation: %v", logs)
	}

	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatalf("listener still bound after ServeUntil returned: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close rebound listener: %v", err)
	}
}

func TestServeUntilCompletesOnBindError(t *testing.T) {
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve occupied address: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close occupied listener: %v", err)
		}
	}()

	var logs []string
	log := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
	srv := NewHTTPServer(listener.Addr().String(), NewMetrics("test", "test"), &Health{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ServeUntil(t.Context(), srv, log)
	}()

	waitForServer(t, done)
	if len(logs) != 1 || !strings.Contains(logs[0], "observability server failed") {
		t.Fatalf("ServeUntil bind-error logs = %v, want one observability server failed entry", logs)
	}
}

func unusedLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}
	return addr
}

func waitForHealth(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build health request: %v", err)
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && closeErr == nil {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("health endpoint %s did not become ready", url)
}

func waitForServer(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeUntil did not return promptly")
	}
}

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
