package observability

import (
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

// sentryFlushTimeout bounds how long shutdown waits for buffered Sentry events to be delivered.
const sentryFlushTimeout = 2 * time.Second

// initSentry initializes Sentry from SENTRY_DSN (with optional SENTRY_ENVIRONMENT) and returns a
// zapcore that forwards Error+ log entries to Sentry, plus a flush func for shutdown. When SENTRY_DSN
// is unset (the default) Sentry is disabled: the core is nil and flush is a no-op, so the sink is
// strictly opt-in via the env keys.
func initSentry() (zapcore.Core, func()) {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		return nil, func() {}
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: os.Getenv("SENTRY_ENVIRONMENT"),
	}); err != nil {
		// A bad DSN must not take down the process; just leave the sink disabled.
		return nil, func() {}
	}
	return &sentryCore{level: zapcore.ErrorLevel}, func() { sentry.Flush(sentryFlushTimeout) }
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
		// Loggers are named per solver ("rfq", "lifi.txmanager", ...), so the name says which
		// deployment an event came from even when the log site is shared code like txmanager.
		tags := eventTags(e.LoggerName, enc.Fields)
		for key, value := range tags {
			scope.SetTag(key, value)
		}
		// Group by solver and static message, not the title: the title carries the error text so
		// the issue stream shows the cause, while one log site in one solver still maps to one issue.
		scope.SetFingerprint([]string{tags["solver"], e.Message})
		sentry.CaptureMessage(eventTitle(e.Message, enc.Fields))
	})
	return nil
}

// eventTags picks the searchable attribution for an event. "solver" is the log field run.go
// stamps (process-wide with one solver, per solver otherwise), falling back to the first logger
// name segment ("rfq.txmanager" -> "rfq"); "label" is the txmanager request label, which is how
// shared components attribute work when several solvers share a process.
func eventTags(loggerName string, fields map[string]any) map[string]string {
	tags := map[string]string{}
	if loggerName != "" {
		tags["logger"] = loggerName
	}
	solver, _ := fields["solver"].(string)
	if solver == "" {
		solver, _, _ = strings.Cut(loggerName, ".")
	}
	if solver != "" {
		tags["solver"] = solver
	}
	if label, ok := fields["label"].(string); ok && label != "" {
		tags["label"] = label
	}
	return tags
}

// eventTitle is the log message followed by the logged error, when there is one. logr's
// Error(err, msg) reaches zap as the static msg plus an "error" field, so without this the issue
// stream only ever shows the message.
func eventTitle(message string, fields map[string]any) string {
	if errText, ok := fields["error"].(string); ok && errText != "" {
		return message + ": " + errText
	}
	return message
}

func (c *sentryCore) Sync() error { sentry.Flush(sentryFlushTimeout); return nil }

func sentryLevel(l zapcore.Level) sentry.Level {
	if l >= zapcore.DPanicLevel {
		return sentry.LevelFatal
	}
	return sentry.LevelError
}
