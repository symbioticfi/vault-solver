package rfq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// maxRequestBytes caps an inbound request body. The only caller is the trusted backend peer and
// quote payloads are small; this is a safety bound against an oversized/slow body.
const maxRequestBytes = 1 << 20 // 1 MiB

const requestIDHeader = "X-Request-Id"

type ctxKey int

const requestIDKey ctxKey = iota

// requestID returns the id assigned to the request by logRequests, or "" if absent.
func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// logRequests assigns (or propagates) a request id — exposed on the response header and the request
// context so handlers can correlate — and logs method/route/status/duration for every request.
func logRequests(next http.Handler, log logr.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("request",
			"method", r.Method, "route", routeLabel(r.URL.Path), "status", rec.status,
			"durationMs", time.Since(start).Milliseconds(), "requestId", id)
	})
}

// recoverPanics turns a handler panic into a 500 plus an Error log (which also reaches the Sentry
// sink) instead of tearing down the connection.
func recoverPanics(next http.Handler, log logr.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//nolint:contextcheck // recovery closure only logs + writes a status; no context-aware calls.
		defer func() {
			if v := recover(); v != nil {
				log.Error(errors.Errorf("panic: %v", v), "recovered panic in quote server",
					"method", r.Method, "path", r.URL.Path, "requestId", requestID(r.Context()))
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
