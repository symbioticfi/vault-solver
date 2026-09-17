package chain

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// rpcTransportWS and rpcTransportIPC label the non-HTTP transports on connect and call spans. An
// HTTP(S) endpoint carries no transport label: fallbackTransport already spans each of its requests.
const (
	rpcTransportWS  = "ws"
	rpcTransportIPC = "ipc"
)

// rpcTracer carries no solver attribute: the chain client is shared by every solver. RPC spans
// start through StartKind for the client span kind and end through the shared policy, with the
// pre-filter below classifying what a node's answer means.
var rpcTracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/chain", "")

// rpcRequestTrace is the span for one logical JSON-RPC request across endpoint attempts. It ends
// where the metrics observation finishes: on response-body close, or when every endpoint failed.
type rpcRequestTrace struct {
	span trace.Span
	end  observability.EndFunc
}

func (t *fallbackTransport) beginTrace(
	ctx context.Context, request rpcRequestInfo,
) (context.Context, *rpcRequestTrace) {
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
	if !tr.span.IsRecording() {
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
	if outcome != rpcOutcomeSuccess {
		tr.span.SetStatus(codes.Error, string(outcome))
	}
	tr.end(nil)
}

// traceConnect spans the dial of a non-HTTP endpoint. For a websocket the returned context is what
// the handshake headers are injected from; IPC has no handshake to carry them.
func traceConnect(ctx context.Context, role, transport string) (context.Context, observability.EndFunc) {
	ctx, end := rpcTracer.StartKind(ctx, "chain.rpc.connect", trace.SpanKindClient,
		attribute.String("chain.rpc.role", role),
		attribute.String("chain.rpc.transport", transport),
	)
	return ctx, func(err error) { end(classifyRPCSpanError(ctx, err)) }
}

// classifyRPCSpanError maps an RPC failure to what the shared span-end policy should record, so a
// ws or IPC call classifies the way the same answer does over HTTP. Two answers are routine rather
// than failures: a caller that cancelled or ran out of time, and a node with nothing to return
// (ethclient reports a null result as ethereum.NotFound, which is the ordinary answer while a
// transaction is unmined or a block is unknown, and the HTTP path counts it a success). Anything
// else is a real failure, reported under a bounded outcome instead of the node's own message.
func classifyRPCSpanError(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		trace.SpanFromContext(ctx).AddEvent("cancelled")
		return nil
	case errors.Is(err, ethereum.NotFound):
		trace.SpanFromContext(ctx).AddEvent("not_found")
		return nil
	default:
		return rpcSpanError{outcome: rpcSpanOutcome(err), cause: err}
	}
}

// rpcSpanOutcome names a failure with the same vocabulary the HTTP transport's metrics use: a
// JSON-RPC error envelope the node answered with, or a failure to reach it at all.
func rpcSpanOutcome(err error) rpcOutcome {
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		return rpcOutcomeRPCError
	}
	return rpcOutcomeTransportError
}

// rpcSpanError bounds the span status text of an RPC failure. The cause stays wrapped, so the
// exception event the shared policy records still carries the node's message and errors.Is at the
// call site is unaffected.
type rpcSpanError struct {
	outcome rpcOutcome
	cause   error
}

func (e rpcSpanError) Error() string      { return e.cause.Error() }
func (e rpcSpanError) Unwrap() error      { return e.cause }
func (e rpcSpanError) SpanStatus() string { return string(e.outcome) }
