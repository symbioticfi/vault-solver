package httptransport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-errors/errors"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type trackingReadCloser struct {
	io.Reader

	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestLimitResponsesRejectsChunkedBody(t *testing.T) {
	base := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: -1,
			Body:          io.NopCloser(strings.NewReader("12345")),
		}, nil
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.test", nil)
	resp, err := LimitResponses(base, 4).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("read error = %v, want ErrResponseTooLarge", err)
	}
	var responseErr *ResponseTooLargeError
	if !errors.As(err, &responseErr) {
		t.Fatalf("read error = %T %v, want *ResponseTooLargeError", err, err)
	}
	if responseErr.Limit != 4 {
		t.Fatalf("response error limit = %d, want 4", responseErr.Limit)
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("read error = %T %v, want *http.MaxBytesError", err, err)
	}
}

func TestLimitResponsesRejectsDeclaredLength(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader("12345")}
	base := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 5,
			Body:          body,
		}, nil
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.test", nil)
	resp, err := LimitResponses(base, 4).RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
		t.Fatalf("response = %#v, want nil", resp)
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
	var responseErr *ResponseTooLargeError
	if !errors.As(err, &responseErr) {
		t.Fatalf("error = %T %v, want *ResponseTooLargeError", err, err)
	}
	if responseErr.Limit != 4 {
		t.Fatalf("response error limit = %d, want 4", responseErr.Limit)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestLimitResponsesUsesDefaultTransportForNilBase(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	called := false
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 0,
			Body:          http.NoBody,
		}, nil
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.test", nil)
	resp, err := LimitResponses(nil, 4).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if !called {
		t.Fatal("default transport was not called")
	}
}

func TestLimitResponsesPanicsForNonPositiveLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int64
	}{
		{name: "zero", limit: 0},
		{name: "negative", limit: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				got := recover()
				if got != "httptransport: response limit must be positive" {
					t.Fatalf("panic = %v, want response limit message", got)
				}
			}()
			LimitResponses(http.DefaultTransport, tc.limit)
		})
	}
}
