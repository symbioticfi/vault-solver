package observability

import "testing"

// Until main calls SetDefaultLogger, Log must still be safe: the fallback discards instead of
// dereferencing a nil pointer.
func TestDefaultLoggerDiscardsUntilSet(t *testing.T) {
	previous := defaultLog.Load()
	t.Cleanup(func() { defaultLog.Store(previous) })
	defaultLog.Store(nil)

	if sink := defaultLogger().GetSink(); sink != nil {
		t.Fatalf("unset default logger sink = %v, want nil (discard)", sink)
	}
	Log(t.Context()).Info("dropped") // must not panic
}
