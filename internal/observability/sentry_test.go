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

// The issue stream shows only the event title, so the logged error belongs in it. Grouping stays
// on the static message (see the fingerprint in Write), so this cannot split one log site into
// one issue per address or hash.
func TestEventTitleAppendsLoggedError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{name: "with error", fields: map[string]any{"error": "adapter.offerSigner() returned zero address", "adapter": "0xd8f6"}, want: "skipping adapter: resolution failed: adapter.offerSigner() returned zero address"},
		{name: "no error field", fields: map[string]any{"adapter": "0xd8f6"}, want: "skipping adapter: resolution failed"},
		{name: "empty error", fields: map[string]any{"error": ""}, want: "skipping adapter: resolution failed"},
		{name: "non-string error", fields: map[string]any{"error": 42}, want: "skipping adapter: resolution failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventTitle("skipping adapter: resolution failed", tc.fields); got != tc.want {
				t.Fatalf("eventTitle = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventSolverPrefersTheProcessFieldOverTheLoggerName(t *testing.T) {
	for _, tc := range []struct {
		logger string
		fields map[string]any
		want   string
	}{
		{logger: "txmanager", fields: map[string]any{"solver": "rfq"}, want: "rfq"},
		{logger: "lifi.txmanager", fields: map[string]any{}, want: "lifi"},
		{logger: "redstone-oev.ws", fields: map[string]any{"solver": ""}, want: "redstone-oev"},
		{logger: "", fields: map[string]any{}, want: ""},
	} {
		if got := eventSolver(tc.logger, tc.fields); got != tc.want {
			t.Errorf("eventSolver(%q, %v) = %q, want %q", tc.logger, tc.fields, got, tc.want)
		}
	}
}
