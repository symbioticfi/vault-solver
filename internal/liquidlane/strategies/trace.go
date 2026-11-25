package strategies

import (
	"fmt"
	"strings"
)

const declinedSuffix = " declined"

// DecisionTrace emits optional debug-only decision details. Callers decide
// whether tracing is enabled and attach protocol correlation fields.
type DecisionTrace func(message string, keyValues ...any)

func (trace DecisionTrace) Log(message string, keyValues ...any) {
	if trace != nil {
		trace(message, keyValues...)
	}
}

func (trace DecisionTrace) Decline(decision, reason string, keyValues ...any) {
	if trace == nil {
		return
	}
	fields := append([]any{"reason", reason}, keyValues...)
	trace.Log("liquidlane "+decision+declinedSuffix, fields...)
}

// RecordDeclines forwards to trace and keeps the latest decline, so a caller left without a
// solution can log the failed condition even when debug tracing is off.
func (trace DecisionTrace) RecordDeclines() (DecisionTrace, func() string) {
	var last string
	return func(message string, keyValues ...any) {
			if strings.HasSuffix(message, declinedSuffix) {
				last = formatEvent(message, keyValues)
			}
			trace.Log(message, keyValues...)
		}, func() string {
			if last == "" {
				return "no decline recorded"
			}
			return last
		}
}

func formatEvent(message string, keyValues []any) string {
	var b strings.Builder
	b.WriteString(message)
	for i := 0; i+1 < len(keyValues); i += 2 {
		fmt.Fprintf(&b, " %v=%v", keyValues[i], keyValues[i+1])
	}
	return b.String()
}
