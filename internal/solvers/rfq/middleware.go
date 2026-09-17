package rfq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// maxRequestBytes caps an inbound request body. The only caller is the trusted backend peer and
// quote payloads are small; this is a safety bound against an oversized/slow body.
const maxRequestBytes = 1 << 20 // 1 MiB

// quoteLinkTTL is how long a served quote's span context stays linkable from the fill that wins it
// (spec §12). The quote response carries no expiry, so this fixed window stands in for the backend's
// award latency; expiring it early only costs the link, never the fill.
const quoteLinkTTL = 10 * time.Minute

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
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		r = r.WithContext(ctx)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		observability.Log(ctx).Info("request",
			"method", r.Method, "route", routeLabel(r.URL.Path), "status", rec.status,
			"durationMs", time.Since(start).Milliseconds(), "requestId", id)
	})
}

// recoverPanics turns a handler panic into a 500 plus an Error log (which also reaches the Sentry
// sink) instead of tearing down the connection.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//nolint:contextcheck // recovery closure: it reads the request context to log, never passes one on.
		defer func() {
			if v := recover(); v != nil {
				observability.Log(r.Context()).Error(errors.Errorf("panic: %v", v),
					"recovered panic in quote server",
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
