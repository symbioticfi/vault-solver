package rfq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-errors/errors"
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

// observeHTTP owns one response recorder for access logs, metrics and recovery.
func (s *server) observeHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		// Recovery records the same final status that the client actually receives.
		defer func() {
			if value := recover(); value != nil {
				s.log.Error(errors.Errorf("panic: %v", value), "recovered panic in quote server",
					"method", r.Method, "path", r.URL.Path, "requestId", id)
				rec.WriteHeader(http.StatusInternalServerError)
			}
			duration := time.Since(start)
			s.metrics.observeHTTP(r.Method, r.URL.Path, rec.status, duration)
			s.log.Info("request", "method", r.Method, "route", routeLabel(r.URL.Path),
				"status", rec.status, "durationMs", duration.Milliseconds(), "requestId", id)
		}()
		next.ServeHTTP(rec, r)
	})
}

// statusRecorder records implicit writes and forwards informational responses
// without committing the final status. Repeated final headers are ignored.
type statusRecorder struct {
	http.ResponseWriter

	status      int
	wroteHeader bool
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *statusRecorder) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	if code >= 200 || code == http.StatusSwitchingProtocols {
		s.status, s.wroteHeader = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(body []byte) (int, error) {
	if !s.wroteHeader {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(body)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
