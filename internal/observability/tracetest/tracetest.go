// Package tracetest installs a recording OpenTelemetry provider for tests.
package tracetest

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Install registers a synchronous recording TracerProvider and the W3C propagator for the test's
// lifetime and returns the recorder. Spans are available from recorder.Ended() once ended.
func Install(tb testing.TB) *tracetest.SpanRecorder {
	tb.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	tb.Cleanup(func() {
		_ = provider.Shutdown(tb.Context())
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})
	return recorder
}
