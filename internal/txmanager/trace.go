package txmanager

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// tracer carries no solver attribute: the manager is shared, so each send span stamps the owning
// solver from Request.Solver instead.
var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/txmanager", "")

// startSendSpan opens the span covering one request from admission to its terminal result. It
// outlives the caller's context, which the manager deliberately detaches from the broadcast, so the
// span itself is what carries the caller's trace into the worker and the lifecycle goroutine.
func startSendSpan(ctx context.Context, req Request) (context.Context, trace.Span) {
	//nolint:spancheck // the span is ended by endSendSpan once the lifecycle is terminal, not inline
	return tracer.Raw().Start(ctx, "txmanager.send "+req.Label, trace.WithAttributes(
		observability.AttrSolver.String(req.Solver),
		observability.AttrTxLabel.String(req.Label),
	))
}

// resultAttrs describes a terminal result. A request that never reached the wire has no hash, and
// the zero hash would read as a real one, so it is left off entirely.
func resultAttrs(res Result) []attribute.KeyValue {
	attrs := []attribute.KeyValue{observability.AttrTxOutcome.String(string(res.Outcome))}
	if res.Hash != (common.Hash{}) {
		attrs = append(attrs, observability.AttrTxHash.String(res.Hash.Hex()))
	}
	return attrs
}

// RecordResult stamps a send's outcome (and its hash, when it has one) on ctx's span. Call it once
// per span that should carry the result: the submission stage, and the pass it belongs to.
func RecordResult(ctx context.Context, res Result) {
	observability.SetAttributes(ctx, resultAttrs(res)...)
}

// endSendSpan closes a send span with the request's terminal result. Outcomes that mean the call did
// not execute as asked are errors; an inclusion the manager could not fully confirm is not.
func endSendSpan(span trace.Span, res Result) {
	span.SetAttributes(resultAttrs(res)...)
	switch res.Outcome {
	case OutcomeReverted, OutcomeCancelled, OutcomeSubmissionError, OutcomeTrackingStopped:
		message := string(res.Outcome)
		if res.Err != nil {
			span.RecordError(res.Err)
			message = res.Err.Error()
		}
		span.SetStatus(codes.Error, message)
	case OutcomeConfirmed, OutcomeIncludedUnconfirmed:
	}
	span.End()
}
