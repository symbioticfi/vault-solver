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
)

const rpcTracerName = "github.com/symbioticfi/vault-solver/internal/chain"

// rpcTracer is the plain tracer rather than observability.NewTracer: RPC spans need a client span
// kind, and the chain client is shared by every solver so they carry no solver attribute. It is
// resolved per request, not cached, because a tracer taken from the global provider before
// NewTracing installs the real one keeps delegating to whichever provider was set first.
func rpcTracer() trace.Tracer { return otel.Tracer(rpcTracerName) }

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
	ctx, span := rpcTracer().Start(ctx, request.boundedMethod,
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
