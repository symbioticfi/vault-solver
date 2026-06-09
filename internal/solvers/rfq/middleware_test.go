package rfq

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestLogRequests_AssignsAndPropagatesRequestID(t *testing.T) {
	var seen string
	h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestID(r.Context()) // handler sees the id on its context
		w.WriteHeader(http.StatusNoContent)
	}), logr.Discard())

	// No incoming id -> one is generated and echoed on the response.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))
	if got := rr.Header().Get(requestIDHeader); got == "" || got != seen {
		t.Fatalf("generated request id: header=%q ctx=%q (want equal, non-empty)", got, seen)
	}

	// An incoming id is propagated unchanged.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	req.Header.Set(requestIDHeader, "abc123")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if seen != "abc123" || rr.Header().Get(requestIDHeader) != "abc123" {
		t.Fatalf("incoming request id not propagated: ctx=%q header=%q", seen, rr.Header().Get(requestIDHeader))
	}
}

func TestRecoverPanics_Returns500(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), logr.Discard())

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/quote", nil)) // must not panic
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 after recovered panic", rr.Code)
	}
}
