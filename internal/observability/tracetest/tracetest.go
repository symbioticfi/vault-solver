// Package tracetest installs a recording OpenTelemetry provider for tests and asserts on what it
// recorded.
package tracetest

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
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

// AllEnded returns every ended span with the given name, in the order they ended.
func AllEnded(rec Recorder, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// RequireSpans fails the test unless every named span has ended.
func RequireSpans(tb testing.TB, rec Recorder, names ...string) {
	tb.Helper()
	ended := Names(rec)
	got := make(map[string]bool, len(ended))
	for _, name := range ended {
		got[name] = true
	}
	for _, name := range names {
		if !got[name] {
			tb.Fatalf("missing span %q; ended spans: %v", name, ended)
		}
	}
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

// RequireAttr fails the test unless the span carries key with the given value.
func RequireAttr(tb testing.TB, s sdktrace.ReadOnlySpan, key, want string) {
	tb.Helper()
	if got := Attr(s, key); got != want {
		tb.Fatalf("span %q %s = %q, want %q", s.Name(), key, got, want)
	}
}

// RequireNoAttr fails the test when the span carries key at all.
func RequireNoAttr(tb testing.TB, s sdktrace.ReadOnlySpan, key string) {
	tb.Helper()
	if got := Attr(s, key); got != "" {
		tb.Fatalf("span %q %s = %q, want it unset", s.Name(), key, got)
	}
}

// EventAttr returns the value key carries on the span's first event named event, and how many such
// events the span recorded.
func EventAttr(s sdktrace.ReadOnlySpan, event, key string) (value string, count int) {
	for _, e := range s.Events() {
		if e.Name != event {
			continue
		}
		count++
		if count > 1 {
			continue
		}
		for _, kv := range e.Attributes {
			if string(kv.Key) == key {
				value = kv.Value.AsString()
			}
		}
	}
	return value, count
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

// CaptureLogs returns a JSON logger at the given verbosity and a func reading back the lines it has
// emitted. Safe to log from several goroutines, which the pipelines under test do.
func CaptureLogs(tb testing.TB, verbosity int) (logr.Logger, func() []string) {
	tb.Helper()
	var mu sync.Mutex
	var lines []string
	log := funcr.NewJSON(func(entry string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, entry)
	}, funcr.Options{Verbosity: verbosity})
	return log, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(lines)
	}
}

// RequireTraceIDsOnce fails the test when a line carries trace_id or span_id more than once, which
// is what storing an already-stamped logger would produce.
func RequireTraceIDsOnce(tb testing.TB, lines []string) {
	tb.Helper()
	for _, line := range lines {
		for _, key := range []string{`"trace_id"`, `"span_id"`} {
			if n := strings.Count(line, key); n > 1 {
				tb.Fatalf("%s appears %d times in %s", key, n, line)
			}
		}
	}
}
