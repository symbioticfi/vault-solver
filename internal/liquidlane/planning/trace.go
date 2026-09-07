package planning

import "github.com/go-logr/logr"

// DecisionTrace emits optional debug-only decision details. Callers decide
// whether tracing is enabled and attach protocol correlation fields.
type DecisionTrace func(message string, keyValues ...any)

// NewDecisionTrace binds correlation fields once. Protocol packages supply the
// logger; planning owns only the common debug transport.
func NewDecisionTrace(log logr.Logger, fields ...any) DecisionTrace {
	debug := log.V(1)
	if !debug.Enabled() {
		return nil
	}
	return debug.WithValues(append([]any(nil), fields...)...).Info
}

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
	trace.Log("liquidlane "+decision+" declined", fields...)
}
