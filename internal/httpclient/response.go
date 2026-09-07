// Package httpclient owns the response boundary for generated HTTP clients.
package httpclient

import (
	"net/http"
	"strings"

	"github.com/go-errors/errors"
)

// Execute owns the response returned by a generated request. Generated clients
// have already decoded or retained their diagnostic body; it is closed here on
// success and failure. Errors preserve the original cause and upstream detail.
func Execute[T any](operation string, request func() (T, *http.Response, error)) (T, error) {
	output, response, err := request()
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		return output, nil
	}
	var zero T
	status := "no response"
	if response != nil {
		status = response.Status
	}
	var upstream interface {
		error
		Body() []byte
	}
	if errors.As(err, &upstream) {
		if body := strings.TrimSpace(string(upstream.Body())); body != "" {
			// Some generated error strings format model pointers incorrectly. Keep
			// the cause for errors.Is/As without duplicating it in the diagnostic.
			return zero, &responseError{message: operation + ": " + status + ": " + body, cause: err}
		}
	}
	return zero, errors.Errorf("%s: %s: %w", operation, status, err)
}

type responseError struct {
	message string
	cause   error
}

func (e *responseError) Error() string { return e.message }
func (e *responseError) Unwrap() error { return e.cause }
