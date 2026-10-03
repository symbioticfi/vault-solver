package rfq

import (
	"bytes"
	"context"

	"github.com/go-errors/errors"

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
	estimateRevertFatal     estimateRevertKind = "fatal"
)

// classifyEstimateRevert uses the vendored contracts' generated decoders. Exact ABI lengths stop
// malformed data from being treated as terminal proof; generated UnpackError requires four bytes.
func classifyEstimateRevert(err error) (estimateRevertKind, string) {
	var revert *txmanager.ExecutionRevertError
	if !errors.As(err, &revert) {
		return estimateNotReverted, ""
	}
	if len(revert.Data) < 4 {
		return estimateRevertUnknown, "unknown"
	}
	decoded, decodeErr := reactor.NewReactor().UnpackError(revert.Data)
	if decodeErr == nil {
		kind, name, size := reactorEstimateError(decoded)
		if size == len(revert.Data) && canonicalErrorArguments(revert.Data, decoded) {
			return kind, name
		}
		return estimateRevertUnknown, "unknown"
	}
	decoded, decodeErr = executor.NewExecutor().UnpackError(revert.Data)
	if decodeErr == nil {
		name, size := executorEstimateError(decoded)
		if size == len(revert.Data) && canonicalErrorArguments(revert.Data, decoded) {
			return estimateRevertFatal, name
		}
	}
	return estimateRevertUnknown, "unknown"
}

func reactorEstimateError(decoded any) (estimateRevertKind, string, int) {
	switch decoded := decoded.(type) {
	case *reactor.ReactorNonceUsed:
		return estimateRevertNonceUsed, "NonceUsed", 4
	case *reactor.ReactorExpiredRequest:
		return estimateRevertExpired, "ExpiredRequest", 4
	case *reactor.ReactorInvalidFiller:
		return estimateRevertFatal, "InvalidFiller", 4
	case *reactor.ReactorInvalidAdapter:
		return estimateRevertFatal, "InvalidAdapter", 4
	case *reactor.ReactorInsufficientBalance:
		return estimateRevertFatal, "InsufficientBalance", 68
	case *reactor.ReactorSafeERC20FailedOperation:
		return estimateRevertFatal, "SafeERC20FailedOperation", 36
	case *reactor.ReactorInvalidAmountIn:
		return estimateRevertFatal, "InvalidAmountIn", 4
	case *reactor.ReactorInvalidOutput:
		return estimateRevertFatal, "InvalidOutput", 4
	case *reactor.ReactorInvalidProtocolSignature:
		return estimateRevertFatal, "InvalidProtocolSignature", 4
	case *reactor.ReactorInvalidTokenIn:
		return estimateRevertFatal, "InvalidTokenIn", 4
	case *reactor.ReactorFailedCall:
		return estimateRevertFatal, "FailedCall", 4
	case *reactor.ReactorInvalidShortString:
		return estimateRevertFatal, "InvalidShortString", 4
	case *reactor.ReactorStringTooLong:
		return estimateRevertFatal, "StringTooLong", 68 + ((len(decoded.Str)+31)/32)*32
	default:
		return estimateRevertUnknown, "unknown", -1
	}
}

func executorEstimateError(decoded any) (string, int) {
	switch decoded.(type) {
	case *executor.ExecutorAddressEmptyCode:
		return "AddressEmptyCode", 36
	case *executor.ExecutorNotCaller:
		return "NotCaller", 4
	case *executor.ExecutorNotReactor:
		return "NotReactor", 4
	case *executor.ExecutorOwnableInvalidOwner:
		return "OwnableInvalidOwner", 36
	case *executor.ExecutorOwnableUnauthorizedAccount:
		return "OwnableUnauthorizedAccount", 36
	case *executor.ExecutorFailedCall:
		return "FailedCall", 4
	case *executor.ExecutorInsufficientBalance:
		return "InsufficientBalance", 68
	case *executor.ExecutorSafeERC20FailedOperation:
		return "SafeERC20FailedOperation", 36
	default:
		return "unknown", -1
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
		observability.Decline(ctx, "fill_expired", "Reactor request expired")
		e.markExpired(local.OrderID, local.TxHash, "Reactor request expired")
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
		e.markExpired(local.OrderID, local.TxHash, "order deadline has passed")
		return nil
	}
	retryAt := now.Add(e.pollInterval)
	if retryAt.Before(local.RetryDeadline) && e.store.scheduleEstimateRetry(local.OrderID, e.maxNonceRetries, retryAt) {
		observability.Log(ctx).V(1).Info("undecoded estimate revert retry scheduled", "retryAt", retryAt,
			"estimateRetries", local.EstimateRetries+1)
	}
	return nil
}
