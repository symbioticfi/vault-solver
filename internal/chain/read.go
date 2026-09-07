package chain

import (
	"context"

	"github.com/go-errors/errors"
)

// Multicaller is the batched read boundary used by typed contract readers.
type Multicaller interface {
	Multicall(ctx context.Context, calls []Call) ([]CallResult, error)
}

// ReadOne handles the envelope of a single typed contract read. The generated
// decoder owns ABI validation; callers own protocol-specific value constraints.
func ReadOne[T any](ctx context.Context, reader Multicaller, call Call, decode func([]byte) (T, error)) (T, error) {
	var zero T
	results, err := reader.Multicall(ctx, []Call{call})
	if err != nil {
		return zero, err
	}
	if len(results) != 1 || !results[0].Success {
		return zero, errors.New("single contract read failed")
	}
	return decode(results[0].ReturnData)
}

// Decode checks a Multicall result before invoking its generated ABI decoder.
// A reverted call's bytes are never interpreted as a successful return value.
func Decode[T any](result CallResult, decode func([]byte) (T, error)) (T, error) {
	if !result.Success {
		var zero T
		return zero, errors.New("call failed")
	}
	return decode(result.ReturnData)
}
