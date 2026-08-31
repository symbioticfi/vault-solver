package strategies

import "github.com/go-logr/logr"

// DecisionTrace emits optional debug-only decision details. Callers decide
// whether tracing is enabled and attach protocol correlation fields.
type DecisionTrace func(message string, keyValues ...any)

func NewDecisionTrace(log logr.Logger, baseFields ...any) DecisionTrace {
	debug := log.V(1)
	if !debug.Enabled() {
		return nil
	}
	base := append([]any(nil), baseFields...)
	return func(message string, keyValues ...any) {
		fields := make([]any, 0, len(base)+len(keyValues))
		fields = append(fields, base...)
		fields = append(fields, keyValues...)
		debug.Info(message, fields...)
	}
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
