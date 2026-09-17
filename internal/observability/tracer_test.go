package observability_test

import (
	"context"
	stderrors "errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

type codedError struct{ msg string }

func (e codedError) Error() string      { return e.msg }
func (e codedError) ReasonCode() string { return "backend_unreachable" }

func TestTracerStartStampsSolverAndRecordsError(t *testing.T) {
	rec := tracetest.Install(t)
	tr := observability.NewTracer("test", "rfq")
	_, end := tr.Start(t.Context(), "rfq.quote", observability.AttrQuoteID.String("q1"))
	end(errors.Errorf("boom: %w", codedError{"backend down"}))
	end(nil) // second call is a no-op
	s := tracetest.Ended(t, rec, "rfq.quote")
	if tracetest.Attr(s, "solver") != "rfq" || tracetest.Attr(s, "quote.id") != "q1" {
		t.Fatalf("attributes: %v", s.Attributes())
	}
	if s.Status().Code != codes.Error {
		t.Fatalf("status = %v, want Error", s.Status())
	}
	if tracetest.Attr(s, "reason_code") != "backend_unreachable" {
		t.Fatalf("reason_code missing: %v", s.Attributes())
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "exception" {
		t.Fatalf("expected one exception event, got %v", s.Events())
	}
}

func TestTracerCancelledIsNotAnError(t *testing.T) {
	rec := tracetest.Install(t)
	_, end := observability.NewTracer("test", "rfq").Start(t.Context(), "rfq.order")
	end(errors.Errorf("wrapped: %w", context.Canceled))
	s := tracetest.Ended(t, rec, "rfq.order")
	if s.Status().Code == codes.Error {
		t.Fatal("cancellation must not set Error status")
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "cancelled" {
		t.Fatalf("expected cancelled event, got %v", s.Events())
	}
}

func TestDeclineAndSetAttributes(t *testing.T) {
	rec := tracetest.Install(t)
	ctx, end := observability.NewTracer("test", "uniswapx").Start(t.Context(), "uniswapx.quote")
	observability.Decline(ctx, "no_quote", "adapter_paused")
	observability.SetAttributes(ctx, observability.AttrTxHash.String("0xabc"))
	end(nil)
	s := tracetest.Ended(t, rec, "uniswapx.quote")
	if s.Status().Code == codes.Error {
		t.Fatal("decline must not set Error status")
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "declined" {
		t.Fatalf("expected declined event, got %v", s.Events())
	}
	if tracetest.Attr(s, "tx.hash") != "0xabc" {
		t.Fatalf("SetAttributes not applied: %v", s.Attributes())
	}
}

func TestStartLinked(t *testing.T) {
	rec := tracetest.Install(t)
	tr := observability.NewTracer("test", "rfq")
	quoteCtx, endQuote := tr.Start(t.Context(), "rfq.quote")
	endQuote(nil)
	link := trace.Link{SpanContext: trace.SpanContextFromContext(quoteCtx)}
	_, end := tr.StartLinked(t.Context(), "rfq.order", []trace.Link{link})
	end(nil)
	s := tracetest.Ended(t, rec, "rfq.order")
	if len(s.Links()) != 1 || s.Links()[0].SpanContext.TraceID() != link.SpanContext.TraceID() {
		t.Fatalf("link not recorded: %v", s.Links())
	}
}

// TestTracerResolvesProviderPerSpan pins the lazy resolution: a Tracer built before any provider is
// installed must still reach whichever provider is current when the span starts. Caching
// otel.Tracer(name) in NewTracer binds it to the first provider ever set, so the second subtest
// would record nothing.
func TestTracerResolvesProviderPerSpan(t *testing.T) {
	tr := observability.NewTracer("test", "rfq")
	for _, name := range []string{"rfq.first", "rfq.second"} {
		t.Run(name, func(t *testing.T) {
			rec := tracetest.Install(t)
			_, end := tr.Start(t.Context(), name)
			end(nil)
			spans := rec.Ended()
			if len(spans) != 1 || spans[0].Name() != name {
				t.Fatalf("recorder saw %v, want exactly the %q span", spans, name)
			}
			if tracetest.Attr(spans[0], "solver") != "rfq" {
				t.Fatalf("attributes = %v, want solver=rfq", spans[0].Attributes())
			}
		})
	}
}

func TestNoopWithoutProvider(t *testing.T) {
	// No tracetest.Install: global provider is the no-op one. Everything must be safe and cheap.
	ctx, end := observability.NewTracer("test", "rfq").Start(t.Context(), "rfq.quote")
	observability.Decline(ctx, "x", "y")
	observability.SetAttributes(ctx, observability.AttrQuoteID.String("q"))
	end(errors.New("ignored"))
}

func BenchmarkStartEndNoop(b *testing.B) {
	tr := observability.NewTracer("bench", "rfq")
	for b.Loop() {
		_, end := tr.Start(b.Context(), "s")
		end(nil)
	}
}

func BenchmarkStartEndRecording(b *testing.B) {
	tracetest.Install(b)
	tr := observability.NewTracer("bench", "rfq")
	for b.Loop() {
		_, end := tr.Start(b.Context(), "s", observability.AttrQuoteID.String("q"))
		end(nil)
	}
}

// The parallel variants are what the tracer cache is for: resolving the provider per span start
// serializes every solver goroutine on the global delegate's mutex and the SDK's tracer map.
func BenchmarkStartEndNoopParallel(b *testing.B) {
	tr := observability.NewTracer("bench", "rfq")
	ctx := b.Context()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, end := tr.Start(ctx, "s")
			end(nil)
		}
	})
}

func BenchmarkStartEndRecordingParallel(b *testing.B) {
	tracetest.Install(b)
	tr := observability.NewTracer("bench", "rfq")
	ctx := b.Context()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, end := tr.Start(ctx, "s", observability.AttrQuoteID.String("q"))
			end(nil)
		}
	})
}

func TestStartLinkedKeyLinksAndStampsTheLogger(t *testing.T) {
	rec := tracetest.Install(t)
	tr := observability.NewTracer("test", "rfq")
	links := observability.NewSpanLinks()
	log, lines := tracetest.CaptureLogs(t, 1)

	quoteCtx, endQuote := tr.Start(t.Context(), "rfq.quote")
	links.Remember(quoteCtx, "q1", time.Minute)
	endQuote(nil)
	quoteTrace := trace.SpanContextFromContext(quoteCtx).TraceID().String()

	ctx, end, gotTrace := tr.StartLinkedKey(
		observability.WithLogger(t.Context(), log), links, "q1", "rfq.order",
		observability.AttrOrderID.String("o1"),
	)
	observability.Log(ctx).Info("linked")
	end(nil)

	if gotTrace != quoteTrace {
		t.Fatalf("returned trace id = %q, want %q", gotTrace, quoteTrace)
	}
	span := tracetest.Ended(t, rec, "rfq.order")
	if len(span.Links()) != 1 || span.Links()[0].SpanContext.TraceID().String() != quoteTrace {
		t.Fatalf("link not recorded: %v", span.Links())
	}
	tracetest.RequireAttr(t, span, "quote.trace_id", quoteTrace)
	tracetest.RequireAttr(t, span, "order.id", "o1")
	if tracetest.HasEvent(span, "link_miss") {
		t.Fatal("a resolved link must not record link_miss")
	}
	got := lines()
	if len(got) != 1 || !strings.Contains(got[0], `"quoteTraceId":"`+quoteTrace+`"`) {
		t.Fatalf("lines = %v, want one carrying quoteTraceId", got)
	}
	tracetest.RequireTraceIDsOnce(t, got)
}

func TestStartLinkedKeyRecordsAMissAndCarriesOn(t *testing.T) {
	rec := tracetest.Install(t)
	tr := observability.NewTracer("test", "rfq")

	// A nil map is the same miss as an unknown key: linking is best effort (spec §12).
	for name, links := range map[string]*observability.SpanLinks{
		"rfq.order.nil":   nil,
		"rfq.order.empty": observability.NewSpanLinks(),
	} {
		_, end, gotTrace := tr.StartLinkedKey(t.Context(), links, "q1", name)
		end(nil)
		if gotTrace != "" {
			t.Fatalf("%s: trace id = %q, want empty on a miss", name, gotTrace)
		}
		span := tracetest.Ended(t, rec, name)
		if len(span.Links()) != 0 {
			t.Fatalf("%s: links = %v, want none", name, span.Links())
		}
		tracetest.RequireNoAttr(t, span, "quote.trace_id")
		key, misses := tracetest.EventAttr(span, "link_miss", "key")
		if misses != 1 || key != "q1" {
			t.Fatalf("%s: link_miss events = %d with key %q, want 1 with q1", name, misses, key)
		}
	}
}

func TestEndRedactsURLsFromRecordedErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantType string
		want     []string
		forbid   []string
	}{
		{
			name: "http url error",
			err: &url.Error{
				Op:  "Post",
				URL: "https://eth.example/v2/SECRETKEY?apikey=abc",
				Err: stderrors.New("dial tcp 1.2.3.4:443: connect: connection refused"),
			},
			wantType: "*url.Error",
			want:     []string{`Post "https://eth.example": dial tcp 1.2.3.4:443: connect: connection refused`},
			forbid:   []string{"SECRETKEY", "abc"},
		},
		{
			name:     "websocket url",
			err:      stderrors.New("dial wss://user:pw@ws.example:8443/ws/WSKEY?x=1: bad handshake"),
			wantType: "*errors.errorString",
			want:     []string{"dial wss://ws.example:8443: bad handshake"},
			forbid:   []string{"WSKEY", "user", "pw@"},
		},
		{
			name:     "no url",
			err:      stderrors.New("execution reverted: insufficient balance"),
			wantType: "*errors.errorString",
			want:     []string{"execution reverted: insufficient balance"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := tracetest.Install(t)
			_, end := observability.NewTracer("test", "rfq").Start(t.Context(), "rfq.quote")
			end(tc.err)
			s := tracetest.Ended(t, rec, "rfq.quote")
			if len(s.Events()) != 1 || s.Events()[0].Name != "exception" {
				t.Fatalf("expected one exception event, got %v", s.Events())
			}
			var message, typ string
			for _, kv := range s.Events()[0].Attributes {
				switch kv.Key {
				case "exception.message":
					message = kv.Value.AsString()
				case "exception.type":
					typ = kv.Value.AsString()
				}
			}
			if typ != tc.wantType {
				t.Fatalf("exception.type = %q, want %q", typ, tc.wantType)
			}
			for _, text := range []string{message, s.Status().Description} {
				for _, want := range tc.want {
					if !strings.Contains(text, want) {
						t.Fatalf("%q does not contain %q", text, want)
					}
				}
				for _, secret := range tc.forbid {
					if strings.Contains(text, secret) {
						t.Fatalf("%q leaks %q", text, secret)
					}
				}
			}
		})
	}
}
