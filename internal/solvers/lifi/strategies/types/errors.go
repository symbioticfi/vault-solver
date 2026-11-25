package types

import "github.com/go-errors/errors"

// ErrUnsupportedOutputContext marks an unsupported pricing format, not malformed known data.
var ErrUnsupportedOutputContext = errors.New("unsupported output context")

// ErrFillDeclined marks a strategy declining a well-formed order; the wrapping error names the
// failed check and its values. The order worker treats it as an expected skip, not a failure.
var ErrFillDeclined = errors.New("fill declined")

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
