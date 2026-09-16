package observability

import (
	"context"
	"testing"

	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

type codedError struct{ msg string }

func (e codedError) Error() string      { return e.msg }
func (e codedError) ReasonCode() string { return "backend_unreachable" }

func endedSpan(t *testing.T, rec interface {
	Ended() []sdktrace.ReadOnlySpan
}, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("span %q not ended", name)
	return nil
}

func attr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String()
		}
	}
	return ""
}

func TestTracerStartStampsSolverAndRecordsError(t *testing.T) {
	rec := tracetest.Install(t)
	tr := NewTracer("test", "rfq")
	_, end := tr.Start(t.Context(), "rfq.quote", AttrQuoteID.String("q1"))
	end(errors.Errorf("boom: %w", codedError{"backend down"}))
	end(nil) // second call is a no-op
	s := endedSpan(t, rec, "rfq.quote")
	if attr(s, "solver") != "rfq" || attr(s, "quote.id") != "q1" {
		t.Fatalf("attributes: %v", s.Attributes())
	}
	if s.Status().Code != codes.Error {
		t.Fatalf("status = %v, want Error", s.Status())
	}
	if attr(s, "reason_code") != "backend_unreachable" {
		t.Fatalf("reason_code missing: %v", s.Attributes())
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "exception" {
		t.Fatalf("expected one exception event, got %v", s.Events())
	}
}

func TestTracerCancelledIsNotAnError(t *testing.T) {
	rec := tracetest.Install(t)
	_, end := NewTracer("test", "rfq").Start(t.Context(), "rfq.order")
	end(errors.Errorf("wrapped: %w", context.Canceled))
	s := endedSpan(t, rec, "rfq.order")
	if s.Status().Code == codes.Error {
		t.Fatal("cancellation must not set Error status")
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "cancelled" {
		t.Fatalf("expected cancelled event, got %v", s.Events())
	}
}

func TestDeclineAndSetAttributes(t *testing.T) {
	rec := tracetest.Install(t)
	ctx, end := NewTracer("test", "uniswapx").Start(t.Context(), "uniswapx.quote")
	Decline(ctx, "no_quote", "adapter_paused")
	SetAttributes(ctx, AttrTxHash.String("0xabc"))
	end(nil)
	s := endedSpan(t, rec, "uniswapx.quote")
	if s.Status().Code == codes.Error {
		t.Fatal("decline must not set Error status")
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "declined" {
		t.Fatalf("expected declined event, got %v", s.Events())
	}
	if attr(s, "tx.hash") != "0xabc" {
		t.Fatalf("SetAttributes not applied: %v", s.Attributes())
	}
}

func TestStartLinked(t *testing.T) {
	rec := tracetest.Install(t)
	tr := NewTracer("test", "rfq")
	quoteCtx, endQuote := tr.Start(t.Context(), "rfq.quote")
	endQuote(nil)
	link := LinkFromContext(quoteCtx)
	_, end := tr.StartLinked(t.Context(), "rfq.order", []trace.Link{link})
	end(nil)
	s := endedSpan(t, rec, "rfq.order")
	if len(s.Links()) != 1 || s.Links()[0].SpanContext.TraceID() != link.SpanContext.TraceID() {
		t.Fatalf("link not recorded: %v", s.Links())
	}
}

// TestTracerResolvesProviderPerSpan pins the lazy resolution: a Tracer built before any provider is
// installed must still reach whichever provider is current when the span starts. Caching
// otel.Tracer(name) in NewTracer binds it to the first provider ever set, so the second subtest
// would record nothing.
func TestTracerResolvesProviderPerSpan(t *testing.T) {
	tr := NewTracer("test", "rfq")
	for _, name := range []string{"rfq.first", "rfq.second"} {
		t.Run(name, func(t *testing.T) {
			rec := tracetest.Install(t)
			_, end := tr.Start(t.Context(), name)
			end(nil)
			spans := rec.Ended()
			if len(spans) != 1 || spans[0].Name() != name {
				t.Fatalf("recorder saw %v, want exactly the %q span", spans, name)
			}
			if attr(spans[0], "solver") != "rfq" {
				t.Fatalf("attributes = %v, want solver=rfq", spans[0].Attributes())
			}
		})
	}
}

func TestNoopWithoutProvider(t *testing.T) {
	// No tracetest.Install: global provider is the no-op one. Everything must be safe and cheap.
	ctx, end := NewTracer("test", "rfq").Start(t.Context(), "rfq.quote")
	Decline(ctx, "x", "y")
	SetAttributes(ctx, AttrQuoteID.String("q"))
	end(errors.New("ignored"))
}

func BenchmarkStartEndNoop(b *testing.B) {
	tr := NewTracer("bench", "rfq")
	for b.Loop() {
		_, end := tr.Start(b.Context(), "s")
		end(nil)
	}
}

func BenchmarkStartEndRecording(b *testing.B) {
	tracetest.Install(b)
	tr := NewTracer("bench", "rfq")
	for b.Loop() {
		_, end := tr.Start(b.Context(), "s", AttrQuoteID.String("q"))
		end(nil)
	}
}
