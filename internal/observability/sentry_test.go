package observability

import "testing"

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
