package observability

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	sdktracetest "go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// installRecorder is what observability/tracetest.Install does, duplicated because that package
// imports this one: these tests cannot import it back.
func installRecorder(tb testing.TB) *sdktracetest.SpanRecorder {
	tb.Helper()
	recorder := sdktracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	InvalidateTracers()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	tb.Cleanup(func() {
		_ = provider.Shutdown(tb.Context())
		otel.SetTracerProvider(prevProvider)
		InvalidateTracers()
		otel.SetTextMapPropagator(prevPropagator)
	})
	return recorder
}
