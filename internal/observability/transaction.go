package observability

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"go.opentelemetry.io/otel/trace"
)

// RecordTransactionResult attaches transaction facts to the worker's current span.
func RecordTransactionResult(ctx context.Context, result txmanager.Result) {
	span := trace.SpanFromContext(ctx)
	if result.Hash != (common.Hash{}) {
		span.SetAttributes(AttrTxHash.String(result.Hash.Hex()))
	}
	outcome := "submission_error"
	switch {
	case result.Reverted():
		outcome = "reverted"
	case result.Included():
		outcome = "confirmed"
	case result.Uncertain():
		outcome = "uncertain"
	case result.NotAdmitted:
		outcome = "not_admitted"
	}
	span.SetAttributes(AttrTxOutcome.String(outcome))
}
