package threef

import (
	"context"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// maxSkipExamples caps the reasons one summary line carries verbatim.
const maxSkipExamples = 20

// skipValues matches the values inside a reason (quoted text, hex, numbers), masked to group reasons
// by the check that produced them.
var skipValues = regexp.MustCompile(`"[^"]*"|0x[0-9a-fA-F]+|[0-9]+`)

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
		checks, examples := summarizeSkips(l.reasons)
		l.debug.Info(msg, append(keysAndValues, "skipped", l.count, "checks", checks, "examples", examples)...)
	}
	return fmt.Sprintf("%d skipped, first %s", l.count, l.reasons[0])
}

// summarizeSkips counts reasons per check and keeps the first maxSkipExamples verbatim, followed by
// "N more" when some were left out.
func summarizeSkips(reasons []string) (checks map[string]int, examples []string) {
	checks = make(map[string]int)
	for _, reason := range reasons {
		checks[skipValues.ReplaceAllString(reason, "#")]++
	}
	n := min(len(reasons), maxSkipExamples)
	examples = reasons[:n:n] // full slice: appending "N more" must not overwrite reasons[n]
	if more := len(reasons) - n; more > 0 {
		examples = append(examples, fmt.Sprintf("%d more", more))
	}
	return checks, examples
}
