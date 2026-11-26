package rfq

import (
	"bytes"
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/reactor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type estimateRevertKind string

const (
	estimateNotReverted     estimateRevertKind = "none"
	estimateRevertUnknown   estimateRevertKind = "unknown"
	estimateRevertNonceUsed estimateRevertKind = "nonce_used"
	estimateRevertExpired   estimateRevertKind = "expired"
	estimateRevertRetryable estimateRevertKind = "retryable"
	estimateRevertFatal     estimateRevertKind = "fatal"
)

// revertDecoder names every revert RFQ calls can surface: Reactor, Executor and connector errors,
// the LiquidLane core contracts, Error(string) and Panic.
var revertDecoder = liquidlane.RevertDecoder(
	&reactor.ReactorMetaData, &executor.ExecutorMetaData, &connector.LiquidLaneConnectorMetaData)

// classifyEstimateRevert uses the vendored contracts' generated decoders. Exact ABI lengths stop
// malformed data from being treated as terminal proof; generated UnpackError requires four bytes.
// The returned description is the decoded revert, whatever the kind.
func classifyEstimateRevert(err error) (estimateRevertKind, string) {
	var revert *txmanager.ExecutionRevertError
	if !errors.As(err, &revert) {
		return estimateNotReverted, ""
	}
	description := revertDecoder.Describe(revert.Data)
	if len(revert.Data) < 4 {
		return estimateRevertUnknown, description
	}
	kind, size := estimateRevertUnknown, -1
	decoded, decodeErr := reactor.NewReactor().UnpackError(revert.Data)
	if decodeErr == nil {
		kind, size = reactorEstimateError(decoded)
	} else if decoded, decodeErr = executor.NewExecutor().UnpackError(revert.Data); decodeErr == nil {
		kind, size = executorEstimateError(decoded)
	}
	if kind == estimateRevertUnknown {
		return kind, description
	}
	if size != len(revert.Data) || !canonicalErrorArguments(revert.Data, decoded) {
		return estimateRevertUnknown, description + " (non-canonical encoding)"
	}
	return kind, description
}

func reactorEstimateError(decoded any) (estimateRevertKind, int) {
	switch decoded := decoded.(type) {
	case *reactor.ReactorNonceUsed:
		return estimateRevertNonceUsed, 4
	case *reactor.ReactorExpiredRequest:
		return estimateRevertExpired, 4
	case *reactor.ReactorInvalidFiller, *reactor.ReactorInvalidAdapter, *reactor.ReactorInvalidAmountIn,
		*reactor.ReactorInvalidOutput, *reactor.ReactorInvalidProtocolSignature, *reactor.ReactorInvalidTokenIn,
		*reactor.ReactorInvalidShortString:
		return estimateRevertFatal, 4
	case *reactor.ReactorInsufficientBalance:
		return estimateRevertFatal, 68
	case *reactor.ReactorSafeERC20FailedOperation:
		return estimateRevertRetryable, 36
	case *reactor.ReactorFailedCall:
		return estimateRevertRetryable, 4
	case *reactor.ReactorStringTooLong:
		return estimateRevertFatal, 68 + ((len(decoded.Str)+31)/32)*32
	default:
		return estimateRevertUnknown, -1
	}
}

func executorEstimateError(decoded any) (estimateRevertKind, int) {
	switch decoded.(type) {
	case *executor.ExecutorNotCaller, *executor.ExecutorNotReactor:
		return estimateRevertFatal, 4
	case *executor.ExecutorAddressEmptyCode, *executor.ExecutorOwnableInvalidOwner,
		*executor.ExecutorOwnableUnauthorizedAccount:
		return estimateRevertFatal, 36
	case *executor.ExecutorFailedCall:
		return estimateRevertRetryable, 4
	case *executor.ExecutorInsufficientBalance:
		return estimateRevertFatal, 68
	case *executor.ExecutorSafeERC20FailedOperation:
		return estimateRevertRetryable, 36
	default:
		return estimateRevertUnknown, -1
	}
}

// Generated ABI decoding permits padding/trailing words. Trust only canonical address padding and
// a canonical single-string offset; the size guard above has already validated every accessed byte.
func canonicalErrorArguments(data []byte, decoded any) bool {
	switch decoded.(type) {
	case *reactor.ReactorSafeERC20FailedOperation, *executor.ExecutorSafeERC20FailedOperation,
		*executor.ExecutorAddressEmptyCode, *executor.ExecutorOwnableInvalidOwner,
		*executor.ExecutorOwnableUnauthorizedAccount:
		return bytes.Equal(data[4:16], make([]byte, 12))
	case *reactor.ReactorStringTooLong:
		return bytes.Equal(data[4:35], make([]byte, 31)) && data[35] == 32
	default:
		return true
	}
}

// reconcileEstimateRevert is called only after the backend still reports open. Terminal backend
// evidence, including peer completion, is handled first by the caller; an absent view waits.
func (e *executionService) reconcileEstimateRevert(ctx context.Context, local *orderRecord) error {
	if local.EstimateKind == estimateRevertExpired {
		e.markExpired(ctx, local.OrderID, local.TxHash, "fill estimate reverted with Reactor request expired: "+
			errString(errors.New(local.LastError)))
		return nil
	}
	if local.EstimateKind == estimateRevertFatal || local.EstimateRetries >= e.maxNonceRetries {
		if e.store.markEstimateFailed(local.OrderID) {
			if e.metrics != nil {
				e.metrics.fillAmounts.ObserveOutcome(liquidlane.FillOutcomeFailure)
			}
			err := errors.New(local.LastError)
			observability.Log(ctx).Error(err, "fill estimate permanently failed",
				"revert", local.EstimateErrorName, "estimateRetries", local.EstimateRetries)
			return err
		}
		return nil
	}
	now := e.now()
	if local.RetryDeadline.IsZero() || !now.Before(local.RetryDeadline) {
		e.markExpired(ctx, local.OrderID, local.TxHash, deadlinePassed(local.RetryDeadline, now))
		return nil
	}
	retryAt := now.Add(e.pollInterval)
	if !retryAt.Before(local.RetryDeadline) {
		observability.Log(ctx).V(1).Info("estimate retry skipped: next poll is not before the order deadline; observing backend",
			"retryAt", retryAt, "deadline", local.RetryDeadline, "revert", local.EstimateErrorName)
		return nil
	}
	if e.store.scheduleEstimateRetry(local.OrderID, e.maxNonceRetries, retryAt) {
		observability.Log(ctx).V(1).Info("estimate revert retry scheduled", "retryAt", retryAt,
			"revert", local.EstimateErrorName, "estimateRetries", local.EstimateRetries+1)
	}
	return nil
}
