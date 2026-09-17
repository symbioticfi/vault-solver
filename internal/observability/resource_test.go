package observability

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
)

// The exported resource identifies the process to the backend: the service triple this bot fills in
// plus the telemetry.sdk.* attributes every OTLP consumer expects. A recording span is the only
// handle a caller of NewTracing has on the provider's resource.
func TestNewTracingResourceDescribesProcessAndSDK(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1") // never reached; batcher exports async
	prev := otel.GetTracerProvider()
	wasEnabled := TracingEnabled()
	t.Cleanup(func() { otel.SetTracerProvider(prev); InvalidateTracers(); SetEnabled(wasEnabled) })
	shutdown, enabled := NewTracing(t.Context(), Tracing{
		Version: "v1.2.3", Commit: "abc123", Solvers: []string{"rfq", "lifi"}, ChainID: 1,
	}, logr.Discard())
	if !enabled {
		t.Fatal("expected tracing enabled")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	defer func() { _ = shutdown(ctx) }()

	_, span := otel.Tracer("x").Start(t.Context(), "probe")
	defer span.End()
	resourced, ok := span.(interface{ Resource() *resource.Resource })
	if !ok {
		t.Fatalf("span %T does not expose its resource", span)
	}
	got := make(map[string]string)
	for _, kv := range resourced.Resource().Attributes() {
		got[string(kv.Key)] = kv.Value.String()
	}
	want := map[string]string{
		"service.name":           "vault-solver",
		"service.version":        "v1.2.3",
		"vault_solver.commit":    "abc123",
		"vault_solver.solvers":   "rfq,lifi",
		"chain.id":               "1",
		"telemetry.sdk.language": "go",
		"telemetry.sdk.name":     "opentelemetry",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("resource %s = %q, want %q (resource: %v)", key, got[key], value, got)
		}
	}
	if got["telemetry.sdk.version"] == "" {
		t.Fatalf("resource has no telemetry.sdk.version: %v", got)
	}
}
