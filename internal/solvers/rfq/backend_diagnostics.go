package rfq

import (
	"context"
	"net/http"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// Reuse the inbound request ID when present; polling has no inbound HTTP request.
func backendRequestContext(ctx context.Context) context.Context {
	if requestID(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, newRequestID())
}

type backendRequestTransport struct{ base http.RoundTripper }

func (t backendRequestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set(requestIDHeader, requestID(req.Context()))
	return t.base.RoundTrip(clone)
}

type backendRequestError struct {
	id  string
	err error
}

func (e *backendRequestError) Error() string {
	return e.err.Error()
}
func (e *backendRequestError) Unwrap() error     { return e.err }
func (e *backendRequestError) RequestID() string { return e.id }
func (e *backendRequestError) ReasonCode() string {
	switch {
	case errors.Is(e.err, context.DeadlineExceeded):
		return "rfq_backend_deadline"
	case errors.Is(e.err, context.Canceled):
		return "rfq_backend_canceled"
	default:
		return "rfq_backend_request_failed"
	}
}

func backendError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	return &backendRequestError{id: requestID(ctx), err: err}
}

// Preserve backend correlation in ordinary logs as well as the Sentry sink, including
// background polling where no inbound request logger exists.
func backendErrorLogger(log logr.Logger, err error) logr.Logger {
	var diagnostic *backendRequestError
	if errors.As(err, &diagnostic) {
		return log.WithValues("backendRequestId", diagnostic.RequestID(), "reason_code", diagnostic.ReasonCode())
	}
	return log
}
