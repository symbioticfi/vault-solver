package lifi

import "github.com/go-errors/errors"

type permanentFillDecisionError struct {
	cause error
}

func (e *permanentFillDecisionError) Error() string { return e.cause.Error() }
func (e *permanentFillDecisionError) Unwrap() error { return e.cause }

// MarkPermanentFillDecisionError identifies a deterministic rejection of one fill input.
// The order worker treats unmarked strategy errors as transient recovery failures.
func MarkPermanentFillDecisionError(err error) error {
	if err == nil || IsPermanentFillDecisionError(err) {
		return err
	}
	return &permanentFillDecisionError{cause: err}
}

// IsPermanentFillDecisionError reports whether a strategy rejected the fill input permanently.
func IsPermanentFillDecisionError(err error) bool {
	var permanent *permanentFillDecisionError
	return errors.As(err, &permanent)
}
