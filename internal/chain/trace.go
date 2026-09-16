package chain

import (
	"context"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// rpcTracer carries no solver attribute: the chain client is shared by every solver. Spans start
// from Raw rather than observability.Start because RPC spans set the client span kind and end from
// rpcRequestTrace.finish, not from a deferred EndFunc. Raw re-resolves the provider whenever one is
// installed, so a tracer built before NewTracing still reaches it.
var rpcTracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/chain", "")

// rpcRequestTrace is the span for one logical JSON-RPC request across endpoint attempts. It ends
// where the metrics observation finishes: on response-body close, or when every endpoint failed.
type rpcRequestTrace struct {
	span trace.Span
	once sync.Once
}

func (t *fallbackTransport) beginTrace(
	ctx context.Context, request rpcRequestInfo,
) (context.Context, *rpcRequestTrace) {
	//nolint:spancheck // the span is ended by rpcRequestTrace.finish, not inline
	ctx, span := rpcTracer.Raw().Start(ctx, request.boundedMethod,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("rpc.system", "jsonrpc"),
			attribute.String("rpc.method", request.boundedMethod),
			attribute.String("rpc.jsonrpc.request_id", request.requestID),
			attribute.String("chain.rpc.role", t.role),
			attribute.Bool("chain.rpc.batch", request.boundedMethod == "batch"),
		),
	)
	return ctx, &rpcRequestTrace{span: span} //nolint:spancheck // see above
}

// attempt records one endpoint attempt. endpoint is the role-local ordinal, never a URL.
func (tr *rpcRequestTrace) attempt(endpoint string, outcome rpcOutcome) {
	tr.span.AddEvent("attempt", trace.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.String("outcome", string(outcome)),
	))
}

func (tr *rpcRequestTrace) finish(outcome rpcOutcome) {
	tr.once.Do(func() {
		if outcome != rpcOutcomeSuccess {
			tr.span.SetStatus(codes.Error, string(outcome))
		}
		tr.span.End()
	})
}

func injectTraceHeaders(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}
