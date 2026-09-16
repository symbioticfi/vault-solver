package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

func TestSpanLinksRememberLookup(t *testing.T) {
	tracetest.Install(t)
	l := NewSpanLinks(2)
	ctx, span := otel.Tracer("x").Start(t.Context(), "q")
	span.End()
	l.Remember(ctx, " Q1 ", time.Minute)
	link, ok := l.Lookup("q1")
	if !ok || link.SpanContext.TraceID() != span.SpanContext().TraceID() {
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
	tracetest.Install(t)
	l := NewSpanLinks(2)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	ctx, span := otel.Tracer("x").Start(t.Context(), "q")
	span.End()
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
	tracetest.Install(t)
	l := NewSpanLinks(2)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	ctx, span := otel.Tracer("x").Start(t.Context(), "q")
	span.End()

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
