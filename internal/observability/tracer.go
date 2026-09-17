package observability

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span attribute keys shared by every solver (spec §9.3). Neutral names: the solver attribute
// disambiguates, so an operator searches quote.id regardless of integration.
const (
	AttrSolver         = attribute.Key("solver")
	AttrRequestID      = attribute.Key("request.id")
	AttrQuoteID        = attribute.Key("quote.id")
	AttrQuoteTraceID   = attribute.Key("quote.trace_id")
	AttrOrderID        = attribute.Key("order.id")
	AttrOrderHash      = attribute.Key("order.hash")
	AttrOrderOnchainID = attribute.Key("order.onchain_id")
	AttrAuctionID      = attribute.Key("auction.id")
	AttrAdapter        = attribute.Key("adapter.address")
	AttrRequestAddress = attribute.Key("request.address")
	AttrStrategy       = attribute.Key("strategy.name")
	AttrTxLabel        = attribute.Key("tx.label")
	AttrTxHash         = attribute.Key("tx.hash")
	AttrTxNonce        = attribute.Key("tx.nonce")
	AttrTxOutcome      = attribute.Key("tx.outcome")
	AttrTxAttempt      = attribute.Key("tx.attempt")
	AttrReasonCode     = attribute.Key("reason_code")
)

// EndFunc ends a span, recording err when non-nil. Safe to call more than once: the SDK ignores a
// second End, and every record it would make first is gated on the span still recording.
type EndFunc func(err error)

// noopEnd is what every Start returns while tracing is disabled, so that path allocates nothing.
var noopEnd EndFunc = func(error) {}

// tracerGeneration counts the TracerProviders installed in this process. Every Tracer caches the
// tracer it resolved together with the generation it saw, so a span start costs no lock, and a new
// provider invalidates every cache at once.
var tracerGeneration atomic.Uint64

// InvalidateTracers makes every Tracer re-resolve its tracer from the global provider. Call it
// after otel.SetTracerProvider — NewTracing and the tracetest helper do — or spans started through
// a Tracer keep going to the provider that was current before.
func InvalidateTracers() { tracerGeneration.Add(1) }

// resolvedTracer is one tracer plus the provider generation it was resolved from.
type resolvedTracer struct {
	generation uint64
	tracer     trace.Tracer
}

// Tracer starts spans that carry the owning solver's name. Obtain one per package with NewTracer.
type Tracer struct {
	name     string                  // instrumentation scope
	opts     []trace.SpanStartOption // the solver attribute, precomputed; nil for shared components
	resolved atomic.Pointer[resolvedTracer]
}

// NewTracer returns a Tracer for the instrumentation scope name (the package import path) that
// stamps solver on every span when solver is non-empty (shared components pass ""). Uses the global
// provider, so it is a no-op until NewTracing enables it.
func NewTracer(name, solver string) *Tracer {
	t := &Tracer{name: name}
	if solver != "" {
		t.opts = []trace.SpanStartOption{trace.WithAttributes(AttrSolver.String(solver))}
	}
	return t
}

// Raw exposes the underlying tracer for code that must hold a trace.Span across goroutines
// (txmanager keeps one span from submission to receipt). Resolving through the global provider
// takes two process-wide mutexes, so the result is cached and re-resolved only once a new provider
// bumps the generation: a Tracer built before NewTracing installs the real provider still reaches
// it, without paying that cost per span.
func (t *Tracer) Raw() trace.Tracer {
	generation := tracerGeneration.Load()
	if cached := t.resolved.Load(); cached != nil && cached.generation == generation {
		return cached.tracer
	}
	resolved := &resolvedTracer{generation: generation, tracer: otel.Tracer(t.name)}
	t.resolved.Store(resolved)
	return resolved.tracer
}

// Start begins a child span of ctx.
func (t *Tracer) Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, EndFunc) {
	return t.StartLinked(ctx, name, nil, attrs...)
}

// StartKind begins a child span of ctx with an explicit span kind, for the client spans an RPC or
// HTTP boundary reports. The end policy is the one every other span goes through.
func (t *Tracer) StartKind(
	ctx context.Context, name string, kind trace.SpanKind, attrs ...attribute.KeyValue,
) (context.Context, EndFunc) {
	return t.start(ctx, name, nil, []trace.SpanStartOption{trace.WithSpanKind(kind)}, attrs)
}

// StartLinked begins a child span of ctx with links to earlier spans (spec §12).
func (t *Tracer) StartLinked(
	ctx context.Context, name string, links []trace.Link, attrs ...attribute.KeyValue,
) (context.Context, EndFunc) {
	return t.start(ctx, name, links, nil, attrs)
}

func (t *Tracer) start(
	ctx context.Context,
	name string,
	links []trace.Link,
	extra []trace.SpanStartOption,
	attrs []attribute.KeyValue,
) (context.Context, EndFunc) {
	if !enabled.Load() {
		return ctx, noopEnd
	}
	// Clip so an append never writes into the precomputed slice two goroutines share.
	opts := slices.Clip(t.opts)
	if len(extra) > 0 {
		opts = append(opts, extra...)
	}
	if len(attrs) > 0 {
		opts = append(opts, trace.WithAttributes(attrs...))
	}
	if len(links) > 0 {
		opts = append(opts, trace.WithLinks(links...))
	}
	//nolint:spancheck // span is ended by the returned EndFunc, not inline
	ctx, span := t.Raw().Start(ctx, name, opts...)
	ctx = withStamped(ctx, span.SpanContext())
	return ctx, func(err error) { endSpan(span, err) } //nolint:spancheck // see above
}

// StartLinkedKey starts name linked to the span remembered under key (spec §12). On a hit it adds
// the link and quote.trace_id, stamps quoteTraceId on the context's base logger, and returns the
// linked trace id; on a miss it records a link_miss event naming the key and returns "". Never
// fails: a nil links map or an unknown key is an ordinary miss.
func (t *Tracer) StartLinkedKey(
	ctx context.Context, links *SpanLinks, key, name string, attrs ...attribute.KeyValue,
) (context.Context, EndFunc, string) {
	if !enabled.Load() {
		return ctx, noopEnd, ""
	}
	var (
		traceID string
		linked  []trace.Link
	)
	if link, ok := links.Lookup(key); ok {
		traceID = link.SpanContext.TraceID().String()
		linked = []trace.Link{link}
		attrs = append(slices.Clip(attrs), AttrQuoteTraceID.String(traceID))
	}
	ctx, end := t.StartLinked(ctx, name, linked, attrs...)
	if traceID == "" {
		LinkMiss(ctx, key)
		return ctx, end, ""
	}
	if base, err := logr.FromContext(ctx); err == nil {
		ctx = WithLogger(ctx, WithQuoteTrace(base, traceID))
	}
	return ctx, end, traceID
}

// WithQuoteTrace stamps the linked quote's trace id on log, the key that joins a fill's log lines to
// the quote that priced it. One definition: a rename here must not leave a solver behind.
func WithQuoteTrace(log logr.Logger, traceID string) logr.Logger {
	if traceID == "" {
		return log
	}
	return log.WithValues("quoteTraceId", traceID)
}

// LinkMiss records that the span remembered under key was gone — restart, eviction, or it was never
// ours. Best effort (spec §12): the work proceeds identically, only the link is lost. Callers
// resolving several keys at once emit one event per missed key.
func LinkMiss(ctx context.Context, key string) {
	trace.SpanFromContext(ctx).AddEvent("link_miss", trace.WithAttributes(attribute.String("key", key)))
}

// EndSpan ends span under the shared policy, for the few callers that hold a trace.Span rather than
// an EndFunc (txmanager carries one from admission to receipt). Not idempotent on its own: the SDK
// ignores a second End, but a second call would record the error twice.
func EndSpan(span trace.Span, err error) { endSpan(span, err) }

// spanStatusDescriber lets an error bound the text of the error status it produces. A span status
// carrying a remote system's message is unbounded and differs per provider, so the chain client
// wraps RPC failures to report the same short outcome on every transport. The error itself is still
// what RecordError puts on the span.
type spanStatusDescriber interface{ SpanStatus() string }

func endSpan(span trace.Span, err error) {
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		span.AddEvent("cancelled")
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, spanStatusMessage(err))
		var coded interface{ ReasonCode() string }
		if errors.As(err, &coded) {
			span.SetAttributes(AttrReasonCode.String(coded.ReasonCode()))
		}
	}
	span.End()
}

func spanStatusMessage(err error) string {
	var described spanStatusDescriber
	if errors.As(err, &described) {
		return described.SpanStatus()
	}
	return err.Error()
}

// Decline records an expected non-error outcome (no quote, not profitable, paused adapter) on the
// current span as a declined event. Status is left unset, mirroring the V(1) logging rule.
func Decline(ctx context.Context, decision, reason string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.AddEvent("declined", trace.WithAttributes(
		attribute.String("decision", decision), attribute.String("reason", reason),
	))
}

// SetAttributes adds attributes to the current span (e.g. a tx hash learned after Send returns).
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}
