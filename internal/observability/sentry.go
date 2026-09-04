package observability

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

const sentryFlushTimeout = 2 * time.Second

func initSentry() zapcore.Core {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		return nil
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: os.Getenv("SENTRY_ENVIRONMENT"),
	}); err != nil {
		return nil
	}
	return &sentryCore{}
}

type sentryCore struct {
	fields []zapcore.Field
}

func (*sentryCore) Enabled(level zapcore.Level) bool {
	return level >= zapcore.ErrorLevel
}

func (c *sentryCore) With(fields []zapcore.Field) zapcore.Core {
	combined := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	combined = append(combined, c.fields...)
	combined = append(combined, fields...)
	return &sentryCore{fields: combined}
}

func (c *sentryCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *sentryCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range c.fields {
		field.AddTo(encoder)
	}
	for _, field := range fields {
		field.AddTo(encoder)
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		if len(encoder.Fields) != 0 {
			scope.SetContext("log", encoder.Fields)
		}
		scope.SetLevel(sentryLevel(entry.Level))
		sentry.CaptureMessage(entry.Message)
	})
	return nil
}

func (*sentryCore) Sync() error {
	sentry.Flush(sentryFlushTimeout)
	return nil
}

func sentryLevel(level zapcore.Level) sentry.Level {
	if level >= zapcore.DPanicLevel {
		return sentry.LevelFatal
	}
	return sentry.LevelError
}
