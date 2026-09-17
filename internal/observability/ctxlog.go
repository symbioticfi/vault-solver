package observability

import (
	"context"
	"sync/atomic"

	"github.com/go-logr/logr"
)

// defaultLog is the process logger Log falls back to when a context carries none. It stays nil until
// main calls SetDefaultLogger, and a nil pointer reads as logr.Discard.
var defaultLog atomic.Pointer[logr.Logger]

// SetDefaultLogger sets the fallback Log uses for contexts without a logger. main calls it once,
// right after building the root logger.
func SetDefaultLogger(log logr.Logger) { defaultLog.Store(&log) }

func defaultLogger() logr.Logger {
	if log := defaultLog.Load(); log != nil {
		return *log
	}
	return logr.Discard()
}

// WithLogger stores the base logger for ctx. Store the *base* logger, never a trace-stamped one:
// Log stamps the trace ids at retrieval, so nested spans get the right span_id and the keys never
// duplicate.
func WithLogger(ctx context.Context, log logr.Logger) context.Context {
	return logr.NewContext(ctx, log)
}

// Log returns the logger for ctx stamped with the current span's trace_id and span_id. It falls back
// to the process logger set by SetDefaultLogger when ctx carries none, so no line is ever dropped.
func Log(ctx context.Context) logr.Logger {
	base, err := logr.FromContext(ctx)
	if err != nil {
		base = defaultLogger()
	}
	return TraceLogger(ctx, base)
}
