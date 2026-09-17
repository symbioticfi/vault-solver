package chain

import (
	"context"
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// rpcTransportWS and rpcTransportIPC label the non-HTTP transports on connect and call spans. An
// HTTP(S) endpoint carries no transport label: fallbackTransport already spans each of its requests.
const (
	rpcTransportWS  = "ws"
	rpcTransportIPC = "ipc"
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

// traceConnect spans the dial of a non-HTTP endpoint. For a websocket the returned context is what
// the handshake headers are injected from; IPC has no handshake to carry them.
func traceConnect(ctx context.Context, role, transport string) (context.Context, func(error)) {
	//nolint:spancheck // the span is ended by the returned func, not inline
	ctx, span := rpcTracer.Raw().Start(ctx, "chain.rpc.connect",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("chain.rpc.role", role),
			attribute.String("chain.rpc.transport", transport),
		),
	)
	return ctx, func(err error) { endClientSpan(span, err) } //nolint:spancheck // see above
}

// endClientSpan ends an RPC client span, recording err unless the caller simply cancelled or the
// node had nothing to return. ethclient turns a null result into ethereum.NotFound, which is the
// routine answer while a transaction is unmined or a block is unknown; over HTTP the same response
// classifies as a success, so treat it as one here too rather than colouring the span red.
func endClientSpan(span trace.Span, err error) {
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		span.AddEvent("cancelled")
	case errors.Is(err, ethereum.NotFound):
		span.AddEvent("not_found")
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func injectTraceHeaders(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}
