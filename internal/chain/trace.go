package chain

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// rpcTracer carries no solver attribute: the chain client is shared by every solver. RPC spans
// start through StartKind for the client span kind and record the HTTP transport's bounded outcome.
var rpcTracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/chain", "")

// rpcRequestTrace is the span for one logical JSON-RPC request across endpoint attempts. It ends
// where the metrics observation finishes: on response-body close, or when every endpoint failed.
type rpcRequestTrace struct {
	span trace.Span
	end  observability.EndFunc
}

// beginTrace opens the span for one logical request. With tracing disabled it builds nothing: the
// nil trace it returns absorbs the attempt and finish calls on the hot RPC path.
func (t *fallbackTransport) beginTrace(
	ctx context.Context, request rpcRequestInfo,
) (context.Context, *rpcRequestTrace) {
	if !observability.TracingEnabled() {
		return ctx, nil
	}
	ctx, end := rpcTracer.StartKind(ctx, request.boundedMethod, trace.SpanKindClient,
		attribute.String("rpc.system", "jsonrpc"),
		attribute.String("rpc.method", request.boundedMethod),
		attribute.String("rpc.jsonrpc.request_id", request.requestID),
		attribute.String("chain.rpc.role", t.role),
		attribute.Bool("chain.rpc.batch", request.boundedMethod == "batch"),
	)
	return ctx, &rpcRequestTrace{span: trace.SpanFromContext(ctx), end: end}
}

// attempt records one endpoint attempt. endpoint is the role-local ordinal, never a URL.
func (tr *rpcRequestTrace) attempt(endpoint string, outcome rpcOutcome) {
	if tr == nil || !tr.span.IsRecording() {
		return
	}
	tr.span.AddEvent("attempt", trace.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.String("outcome", string(outcome)),
	))
}

// finish ends the request span with the outcome the metrics recorded. The outcome is already a
// bounded label and the per-endpoint errors are on the attempt events, so the status is set from it
// directly and the shared end policy only closes the span. Repeat calls are no-ops.
func (tr *rpcRequestTrace) finish(outcome rpcOutcome) {
	if tr == nil {
		return
	}
	if outcome != rpcOutcomeSuccess {
		tr.span.SetStatus(codes.Error, string(outcome))
	}
	tr.end(nil)
}
