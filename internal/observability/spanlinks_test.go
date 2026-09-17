package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// tracedContext returns a context carrying a valid span context. SpanLinks only reads the span
// context, so these tests need no recording provider.
func tracedContext(t *testing.T) (context.Context, trace.SpanContext) {
	t.Helper()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x0b, 0xad, 0xca, 0xfe, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(t.Context(), sc), sc
}

func TestSpanLinksRememberLookup(t *testing.T) {
	ctx, sc := tracedContext(t)
	l := NewSpanLinks(2)
	l.Remember(ctx, " Q1 ", time.Minute)
	link, ok := l.Lookup("q1")
	if !ok || link.SpanContext.TraceID() != sc.TraceID() {
		t.Fatalf("lookup = %v, %v", link, ok)
	}
	if _, hit := l.Lookup("missing"); hit {
		t.Fatal("unexpected hit")
	}
	l.Remember(ctx, "", time.Minute)
	l.Remember(context.Background(), "noctx", time.Minute)
	if l.Len() != 1 {
		t.Fatalf("len = %d, want 1 (empty key and no-span ctx ignored)", l.Len())
	}
}

func TestSpanLinksTTLAndEviction(t *testing.T) {
	ctx, _ := tracedContext(t)
	l := NewSpanLinks(2)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	l.Remember(ctx, "a", 10*time.Second)
	l.Remember(ctx, "b", 10*time.Second)
	l.Remember(ctx, "c", 10*time.Second) // evicts a
	if _, ok := l.Lookup("a"); ok {
		t.Fatal("a should have been evicted")
	}
	if _, ok := l.Lookup("b"); !ok {
		t.Fatal("b should remain")
	}
	now = now.Add(11 * time.Second)
	if _, ok := l.Lookup("b"); ok {
		t.Fatal("b should have expired")
	}
	if l.Len() != 1 { // c still stored until swept or looked up
		t.Fatalf("len = %d", l.Len())
	}
}

// TestSpanLinksReRememberAfterExpiryDoesNotDesyncEviction guards against evicting a freshly
// re-remembered key because its stale, already-expired order slot is still queued for eviction.
func TestSpanLinksReRememberAfterExpiryDoesNotDesyncEviction(t *testing.T) {
	ctx, _ := tracedContext(t)
	l := NewSpanLinks(2)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }

	l.Remember(ctx, "a", 5*time.Second)
	now = now.Add(6 * time.Second)
	if _, ok := l.Lookup("a"); ok {
		t.Fatal("a should have expired")
	}
	l.Remember(ctx, "b", time.Minute)
	l.Remember(ctx, "a", time.Minute) // re-remember; must not be evicted by a's stale order slot

	if _, ok := l.Lookup("a"); !ok {
		t.Fatal("a should hit: it was just re-remembered")
	}
	if _, ok := l.Lookup("b"); !ok {
		t.Fatal("b should hit: it fits within capacity")
	}
}
