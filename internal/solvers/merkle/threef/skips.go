package threef

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// skipLog collects one pass's skip reasons. The first is always formatted for the span decline, the
// rest only when debug logging is on, so a quiet pass costs one Sprintf however many rows it skips.
type skipLog struct {
	debug   logr.Logger
	verbose bool
	count   int
	reasons []string
}

func newSkipLog(ctx context.Context) *skipLog {
	debug := observability.Log(ctx).V(1)
	return &skipLog{debug: debug, verbose: debug.Enabled()}
}

func (l *skipLog) addf(format string, args ...any) {
	l.count++
	if l.count == 1 || l.verbose {
		l.reasons = append(l.reasons, fmt.Sprintf(format, args...))
	}
}

// flush writes the bounded debug summary and returns "N skipped, first R" for a span decline, or ""
// when nothing was skipped.
func (l *skipLog) flush(msg string, keysAndValues ...any) string {
	if l.count == 0 {
		return ""
	}
	if l.verbose {
		checks, examples := observability.SummarizeReasons(l.reasons, observability.MaxReasonExamples)
		l.debug.Info(msg, append(keysAndValues, "skipped", l.count, "checks", checks, "examples", examples)...)
	}
	return fmt.Sprintf("%d skipped, first %s", l.count, l.reasons[0])
}
