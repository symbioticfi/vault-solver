package observability

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
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
	shutdown, enabled := NewTracing(t.Context(), Tracing{}, logr.Discard())
	if enabled {
		t.Fatal("expected tracing disabled")
	}
	if TracingEnabled() {
		t.Fatal("no provider installed, so the package must stay off")
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
	wasEnabled := TracingEnabled()
	t.Cleanup(func() { otel.SetTracerProvider(prev); InvalidateTracers(); SetEnabled(wasEnabled) })
	shutdown, enabled := NewTracing(t.Context(), Tracing{Version: "v", Commit: "c", Solvers: []string{"rfq"}, ChainID: 1}, logr.Discard())
	if !enabled {
		t.Fatal("expected tracing enabled")
	}
	if !TracingEnabled() {
		t.Fatal("installing a provider must switch the package on")
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

// The batch span processor sizes its queue and batch buffers straight from these variables, and a
// negative size panics inside the SDK. A bad value must only disable tracing, like any other setup
// error; a valid one still enables it.
func TestNewTracingValidatesBatchProcessorSizes(t *testing.T) {
	cases := []struct {
		name, queue, batch string
		want               bool
	}{
		{name: "negative queue size", queue: "-1"},
		{name: "negative batch size", batch: "-1"},
		{name: "valid sizes", queue: "100", batch: "10", want: true},
		{name: "unparseable sizes fall back to defaults", queue: "big", batch: "small", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_ENABLED", "true")
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
			t.Setenv("OTEL_BSP_MAX_QUEUE_SIZE", tc.queue)
			t.Setenv("OTEL_BSP_MAX_EXPORT_BATCH_SIZE", tc.batch)
			prev := otel.GetTracerProvider()
			wasEnabled := TracingEnabled()
			t.Cleanup(func() { otel.SetTracerProvider(prev); InvalidateTracers(); SetEnabled(wasEnabled) })

			shutdown, enabled := NewTracing(t.Context(), Tracing{}, logr.Discard())
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			_ = shutdown(ctx)

			if enabled != tc.want {
				t.Fatalf("NewTracing enabled = %v, want %v", enabled, tc.want)
			}
		})
	}
}
