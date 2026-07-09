package httptransport

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-errors/errors"
)

var ErrResponseTooLarge = errors.New("http response body too large")

type ResponseTooLargeError struct {
	Limit int64
	Cause error
}

func (e *ResponseTooLargeError) Error() string {
	return "http response body exceeds " + strconv.FormatInt(e.Limit, 10) + " bytes"
}

func (e *ResponseTooLargeError) Unwrap() error {
	return e.Cause
}

func (e *ResponseTooLargeError) Is(target error) bool {
	return target == ErrResponseTooLarge
}

type responseLimitTransport struct {
	base  http.RoundTripper
	limit int64
}

type responseLimitReadCloser struct {
	io.ReadCloser

	limit int64
}

func (r *responseLimitReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return n, &ResponseTooLargeError{Limit: r.limit, Cause: err}
	}
	return n, err
}

func LimitResponses(base http.RoundTripper, limit int64) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if limit <= 0 {
		panic("httptransport: response limit must be positive")
	}
	return &responseLimitTransport{base: base, limit: limit}
}

func (t *responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.ContentLength > t.limit {
		_ = resp.Body.Close()
		return nil, &ResponseTooLargeError{Limit: t.limit}
	}
	resp.Body = &responseLimitReadCloser{
		ReadCloser: http.MaxBytesReader(nil, resp.Body, t.limit),
		limit:      t.limit,
	}
	return resp, nil
}
