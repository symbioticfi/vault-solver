package observability

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
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

// stampedKey addresses the per-span stamp slot on a context.
type stampedKey struct{}

// stamped memoises one span's stamped logger. The slot is empty until the first Log call fills it,
// so a span whose lines are all suppressed never builds one.
type stamped struct {
	spanID trace.SpanID
	once   sync.Once
	log    logr.Logger
}

func (s *stamped) logger(ctx context.Context, sc trace.SpanContext) logr.Logger {
	s.once.Do(func() {
		base, err := logr.FromContext(ctx)
		if err != nil {
			base = defaultLogger()
		}
		s.log = stampLogger(base, sc)
	})
	return s.log
}

// withStamped gives ctx a fresh, empty stamp slot for sc.
func withStamped(ctx context.Context, sc trace.SpanContext) context.Context {
	if !sc.IsValid() {
		return ctx
	}
	return context.WithValue(ctx, stampedKey{}, &stamped{spanID: sc.SpanID()})
}

func stampLogger(log logr.Logger, sc trace.SpanContext) logr.Logger {
	return log.WithValues("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}

// WithLogger stores the base logger for ctx. Store the *base* logger, never a trace-stamped one:
// Log stamps the trace ids at retrieval, so nested spans get the right span_id and the keys never
// duplicate. A narrowed logger gets a fresh stamp slot, so it replaces the memoised one.
func WithLogger(ctx context.Context, log logr.Logger) context.Context {
	ctx = logr.NewContext(ctx, log)
	return withStamped(ctx, trace.SpanContextFromContext(ctx))
}

// Log returns the logger for ctx stamped with the current span's trace_id and span_id. It falls back
// to the process logger set by SetDefaultLogger when ctx carries none, so no line is ever dropped.
// The stamp is memoised per span, so repeated lines under one span reuse the same logger.
func Log(ctx context.Context) logr.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if slot, _ := ctx.Value(stampedKey{}).(*stamped); slot != nil && slot.spanID == sc.SpanID() {
		return slot.logger(ctx, sc)
	}
	base, err := logr.FromContext(ctx)
	if err != nil {
		base = defaultLogger()
	}
	if !sc.IsValid() {
		return base
	}
	return stampLogger(base, sc)
}
