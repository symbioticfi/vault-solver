// Package tracetest installs a recording OpenTelemetry provider for tests and asserts on what it
// recorded.
package tracetest

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/symbioticfi/vault-solver/internal/observability"
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
	observability.InvalidateTracers()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	tb.Cleanup(func() {
		// Background, not tb.Context(): the test's context is already cancelled by cleanup time and
		// Shutdown would abandon the flush.
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(prevProvider)
		observability.InvalidateTracers()
		otel.SetTextMapPropagator(prevPropagator)
	})
	return recorder
}

// Recorder is the slice of *tracetest.SpanRecorder the assertions below need. Callers keeping their
// own narrow recorder interface can pass it straight through.
type Recorder interface {
	Ended() []sdktrace.ReadOnlySpan
}

// Ended returns the ended span with the given name, failing the test when it is missing.
func Ended(tb testing.TB, rec Recorder, name string) sdktrace.ReadOnlySpan {
	tb.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
	}
	tb.Fatalf("span %q not ended; ended spans: %v", name, Names(rec))
	return nil
}

// Names returns the names of every ended span, in the order they ended.
func Names(rec Recorder) []string {
	ended := rec.Ended()
	out := make([]string, 0, len(ended))
	for _, s := range ended {
		out = append(out, s.Name())
	}
	return out
}

// Attr returns the span attribute's value as a string, or "" when the span carries no such key.
func Attr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String()
		}
	}
	return ""
}

// RequireChildOf fails the test unless child's parent is parent.
func RequireChildOf(tb testing.TB, child, parent sdktrace.ReadOnlySpan) {
	tb.Helper()
	if got, want := child.Parent().SpanID(), parent.SpanContext().SpanID(); got != want {
		tb.Fatalf("span %q parent = %s, want %q (%s)", child.Name(), got, parent.Name(), want)
	}
}

// HasEvent reports whether the span recorded an event with the given name.
func HasEvent(s sdktrace.ReadOnlySpan, name string) bool {
	for _, e := range s.Events() {
		if e.Name == name {
			return true
		}
	}
	return false
}

// RequireNoErrorSpans fails the test when any ended span carries an error status.
func RequireNoErrorSpans(tb testing.TB, rec Recorder) {
	tb.Helper()
	for _, s := range rec.Ended() {
		if s.Status().Code == codes.Error {
			tb.Fatalf("span %q ended with error status %q", s.Name(), s.Status().Description)
		}
	}
}
