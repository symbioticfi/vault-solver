package observability

import (
	"context"

	"github.com/go-errors/errors"
)

// Canceled reports whether err is ctx's own cancellation. A deadline is not a cancellation, so
// timeouts stay visible.
func Canceled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) && errors.Is(ctx.Err(), context.Canceled)
}

// ErrorUnlessCanceled logs err at Error, or at V(1) when it is ctx's cancellation (shutdown,
// reconnect, client disconnect). Use it only where a cancel leaves nothing half done.
func ErrorUnlessCanceled(ctx context.Context, err error, msg string, keysAndValues ...any) {
	if Canceled(ctx, err) {
		Log(ctx).V(1).Info(msg, append([]any{"error", err.Error()}, keysAndValues...)...)
		return
	}
	Log(ctx).Error(err, msg, keysAndValues...)
}
