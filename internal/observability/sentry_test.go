package observability

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestInitSentry_DisabledWithoutDSN confirms the sink is strictly opt-in: with no SENTRY_DSN, no core
// is installed and flush is a harmless no-op.
func TestInitSentry_DisabledWithoutDSN(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	core, flush := initSentry()
	if core != nil {
		t.Fatalf("expected nil core when SENTRY_DSN is unset, got %T", core)
	}
	flush() // must not panic
}

// TestSentryCore_ForwardsOnlyErrorAndAbove pins the sink's level gate: debug and info entries (logr
// V(1).Info and Info) never reach Sentry, so demoting a noisy log line to debug also silences it there.
func TestSentryCore_ForwardsOnlyErrorAndAbove(t *testing.T) {
	core := &sentryCore{level: zapcore.ErrorLevel}
	for _, tc := range []struct {
		level zapcore.Level
		want  bool
	}{
		{zapcore.DebugLevel, false},
		{zapcore.InfoLevel, false},
		{zapcore.WarnLevel, false},
		{zapcore.ErrorLevel, true},
		{zapcore.DPanicLevel, true},
		{zapcore.PanicLevel, true},
		{zapcore.FatalLevel, true},
	} {
		if got := core.Enabled(tc.level); got != tc.want {
			t.Errorf("Enabled(%s) = %v, want %v", tc.level, got, tc.want)
		}
		checked := core.Check(zapcore.Entry{Level: tc.level, Message: "m"}, nil)
		if added := checked != nil; added != tc.want {
			t.Errorf("Check(%s) added core = %v, want %v", tc.level, added, tc.want)
		}
	}
}
