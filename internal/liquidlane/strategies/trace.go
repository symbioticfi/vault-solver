package strategies

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
	trace.Log("liquidlane "+decision+" declined", fields...)
}
