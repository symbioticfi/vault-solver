package observability_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

func TestErrorUnlessCanceled(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancelExpired()
	for _, tc := range []struct {
		name      string
		ctx       context.Context
		err       error
		wantError bool
	}{
		{name: "own cancellation", ctx: canceled, err: errors.Errorf("rpc: %w", context.Canceled)},
		{name: "deadline", ctx: expired, err: errors.Errorf("rpc: %w", context.DeadlineExceeded), wantError: true},
		{name: "canceled error under live context", ctx: t.Context(), err: context.Canceled, wantError: true},
		{name: "other error under canceled context", ctx: canceled, err: errors.New("boom"), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, lines := captureJSON()
			observability.ErrorUnlessCanceled(observability.WithLogger(tc.ctx, log), tc.err, "op failed", "k", "v")
			got := lines()
			if len(got) != 1 {
				t.Fatalf("lines = %v, want one", got)
			}
			isError := !strings.Contains(got[0], `"level":`)
			if isError != tc.wantError {
				t.Fatalf("logged at Error=%v want %v: %s", isError, tc.wantError, got[0])
			}
			if !strings.Contains(got[0], `"k":"v"`) || !strings.Contains(got[0], "error") {
				t.Fatalf("line lost fields: %s", got[0])
			}
		})
	}
}
