package observability

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel"
)

func TestTracingEnabled(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false}, {"1", true}, {"true", true}, {"TRUE", true}, {" yes ", true}, {"on", true},
		{"enabled", true}, {"0", false}, {"false", false}, {"off", false}, {"garbage", false},
	}
	for _, c := range cases {
		if got := tracingEnabled(c.in); got != c.want {
			t.Errorf("tracingEnabled(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNewTracingDisabledInstallsPropagatorOnly(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_ENABLED", "")
	shutdown, enabled := NewTracing(t.Context(), Tracing{Name: "test"}, logr.Discard())
	if enabled {
		t.Fatal("expected tracing disabled")
	}
	if _, ok := otel.GetTextMapPropagator().(interface{ Fields() []string }); !ok {
		t.Fatal("propagator not installed")
	}
	if fields := otel.GetTextMapPropagator().Fields(); len(fields) == 0 {
		t.Fatal("propagator has no fields; expected traceparent/tracestate/baggage")
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestNewTracingEnabledRegistersProvider(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1") // never reached; batcher exports async
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev); InvalidateTracers() })
	shutdown, enabled := NewTracing(t.Context(), Tracing{Name: "test", Version: "v", Commit: "c", Solvers: []string{"rfq"}, ChainID: 1}, logr.Discard())
	if !enabled {
		t.Fatal("expected tracing enabled")
	}
	_, span := otel.Tracer("x").Start(t.Context(), "probe")
	if !span.SpanContext().IsValid() || !span.IsRecording() {
		t.Fatal("expected a recording span from the registered provider")
	}
	span.End()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_ = shutdown(ctx) // export to a closed port fails; must not error the caller path or hang
}

func TestTraceLogger(t *testing.T) {
	var lines []string
	log := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{})

	// No span in context: TraceLogger must return a logger that stamps no trace fields.
	TraceLogger(t.Context(), log).Info("bare")
	if len(lines) != 1 || strings.Contains(lines[0], "trace_id") {
		t.Fatalf("expected no trace_id without a span, got %q", lines)
	}
	lines = nil

	installRecorder(t)
	ctx, span := otel.Tracer("x").Start(t.Context(), "s")
	defer span.End()
	TraceLogger(ctx, log).Info("hello")
	want := "\"trace_id\"=\"" + span.SpanContext().TraceID().String() + "\""
	if len(lines) != 1 || !strings.Contains(lines[0], want) || !strings.Contains(lines[0], "\"span_id\"") {
		t.Fatalf("log line missing trace fields: %q", lines)
	}
}
