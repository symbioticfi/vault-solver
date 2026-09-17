package observability_test

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

// captureJSON returns a JSON logger and the lines it has emitted so far.
func captureJSON() (logr.Logger, func() []string) {
	var lines []string
	log := funcr.NewJSON(func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1})
	return log, func() []string { return lines }
}

func TestLogReturnsTheContextLoggerWithoutASpan(t *testing.T) {
	log, lines := captureJSON()
	ctx := observability.WithLogger(t.Context(), log.WithValues("solver", "rfq"))

	observability.Log(ctx).Info("bare")

	got := lines()
	if len(got) != 1 {
		t.Fatalf("lines = %v, want exactly one", got)
	}
	if !strings.Contains(got[0], `"solver":"rfq"`) {
		t.Fatalf("line lost the stored logger's values: %s", got[0])
	}
	if strings.Contains(got[0], "trace_id") {
		t.Fatalf("no span in context, want no trace_id: %s", got[0])
	}
}

func TestLogStampsTheCurrentSpan(t *testing.T) {
	tracetest.Install(t)
	log, lines := captureJSON()
	ctx, span := otel.Tracer("x").Start(observability.WithLogger(t.Context(), log), "s")
	defer span.End()

	observability.Log(ctx).Info("hello")

	got := lines()
	if len(got) != 1 {
		t.Fatalf("lines = %v, want exactly one", got)
	}
	sc := span.SpanContext()
	if !strings.Contains(got[0], `"trace_id":"`+sc.TraceID().String()+`"`) ||
		!strings.Contains(got[0], `"span_id":"`+sc.SpanID().String()+`"`) {
		t.Fatalf("line does not carry the span's ids: %s", got[0])
	}
}

// Storing the base logger (never a stamped one) is what keeps a nested span from inheriting its
// parent's span_id or logging two of each key.
func TestLogStampsNestedSpansOnceWithTheInnerSpan(t *testing.T) {
	tracetest.Install(t)
	log, lines := captureJSON()
	outerCtx, outer := otel.Tracer("x").Start(observability.WithLogger(t.Context(), log), "outer")
	defer outer.End()
	innerCtx, inner := otel.Tracer("x").Start(outerCtx, "inner")
	defer inner.End()

	observability.Log(innerCtx).Info("nested")

	got := lines()
	if len(got) != 1 {
		t.Fatalf("lines = %v, want exactly one", got)
	}
	for _, key := range []string{"trace_id", "span_id"} {
		if n := strings.Count(got[0], `"`+key+`":`); n != 1 {
			t.Fatalf("%s appears %d times, want once: %s", key, n, got[0])
		}
	}
	if !strings.Contains(got[0], `"span_id":"`+inner.SpanContext().SpanID().String()+`"`) {
		t.Fatalf("line does not carry the inner span id: %s", got[0])
	}
	if strings.Contains(got[0], outer.SpanContext().SpanID().String()) {
		t.Fatalf("line carries the outer span id: %s", got[0])
	}
}

func TestLogFallsBackToTheDefaultLogger(t *testing.T) {
	log, lines := captureJSON()
	observability.SetDefaultLogger(log)
	t.Cleanup(func() { observability.SetDefaultLogger(logr.Discard()) })

	observability.Log(t.Context()).Info("fallback")

	got := lines()
	if len(got) != 1 || !strings.Contains(got[0], `"msg":"fallback"`) {
		t.Fatalf("lines = %v, want the line on the default logger", got)
	}
}
