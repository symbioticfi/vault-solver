package observability

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

// sentryFlushTimeout bounds how long shutdown waits for buffered Sentry events to be delivered.
const sentryFlushTimeout = 2 * time.Second

// initSentry initializes Sentry from SENTRY_DSN (with optional SENTRY_ENVIRONMENT) and returns a
// zapcore that forwards Error+ log entries to Sentry. When SENTRY_DSN is unset (the default), Sentry
// is disabled and the core is nil, so the sink is strictly opt-in via the env keys.
func initSentry() zapcore.Core {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		return nil
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: os.Getenv("SENTRY_ENVIRONMENT"),
	}); err != nil {
		// A bad DSN must not take down the process; just leave the sink disabled.
		return nil
	}
	return &sentryCore{level: zapcore.ErrorLevel}
}

// sentryCore is a barebones zapcore that captures Error+ entries as Sentry events, attaching the log
// fields as Sentry "extra" context. It is teed alongside the normal zap core, so logging is unchanged
// and Sentry only ever sees error-and-above.
type sentryCore struct {
	level  zapcore.Level
	fields []zapcore.Field
}

func (c *sentryCore) Enabled(l zapcore.Level) bool { return l >= c.level }

func (c *sentryCore) With(fields []zapcore.Field) zapcore.Core {
	return &sentryCore{level: c.level, fields: append(append([]zapcore.Field{}, c.fields...), fields...)}
}

func (c *sentryCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}

func (c *sentryCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.fields {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		if len(enc.Fields) > 0 {
			scope.SetContext("log", enc.Fields)
		}
		scope.SetLevel(sentryLevel(e.Level))
		sentry.CaptureMessage(e.Message)
	})
	return nil
}

func (c *sentryCore) Sync() error { sentry.Flush(sentryFlushTimeout); return nil }

func sentryLevel(l zapcore.Level) sentry.Level {
	if l >= zapcore.DPanicLevel {
		return sentry.LevelFatal
	}
	return sentry.LevelError
}
