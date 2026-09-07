package rfq

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestLogRequests_AssignsAndPropagatesRequestID(t *testing.T) {
	var seen string
	h := (&server{log: logr.Discard()}).observeHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestID(r.Context()) // handler sees the id on its context
		w.WriteHeader(http.StatusNoContent)
	}))

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
	h := (&server{log: logr.Discard()}).observeHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/quote", nil)) // must not panic
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 after recovered panic", rr.Code)
	}
}

func TestHTTPRecorderKeepsCommittedResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    int
	}{
		{"implicit success", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
			w.WriteHeader(http.StatusBadGateway)
		}, http.StatusOK},
		{"panic after response", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
			panic("after response")
		}, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h := (&server{log: logr.Discard()}).observeHTTP(tc.handler)
			h.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))
			if rr.Code != tc.want {
				t.Fatalf("status=%d, want %d", rr.Code, tc.want)
			}
		})
	}
}
