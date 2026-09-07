package observability

import (
	"os"
	"slices"
	"strings"
	"sync"
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
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: os.Getenv("SENTRY_ENVIRONMENT"),
	})
	if err != nil {
		// A bad DSN must not take down the process; just leave the sink disabled.
		return nil, func() {}
	}
	flush := sync.OnceFunc(func() { client.Flush(sentryFlushTimeout) })
	return &sentryCore{level: zapcore.ErrorLevel, client: client, flush: flush}, flush
}

// sentryCore is a barebones zapcore that captures Error+ entries as Sentry events, attaching the log
// fields as Sentry "extra" context. It is teed alongside the normal zap core, so logging is unchanged
// and Sentry only ever sees error-and-above.
type sentryCore struct {
	client *sentry.Client
	flush  func()
	level  zapcore.Level
	fields []zapcore.Field
}

func (c *sentryCore) Enabled(l zapcore.Level) bool { return l >= c.level }

func (c *sentryCore) With(fields []zapcore.Field) zapcore.Core {
	return &sentryCore{level: c.level, client: c.client, flush: c.flush, fields: append(slices.Clone(c.fields), fields...)}
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
	if c.client == nil {
		return nil
	}
	// The logger owns its Sentry client; each write creates an independent event.
	// No process-global hub or mutable scope participates in concurrent logging.
	tags := eventTags(e.LoggerName, enc.Fields)
	event := sentry.NewEvent()
	event.Message = eventTitle(e.Message, enc.Fields)
	event.Timestamp = e.Time
	event.Level = sentryLevel(e.Level)
	event.Tags = tags
	event.Fingerprint = []string{tags["solver"], e.Message}
	if len(enc.Fields) > 0 {
		event.Contexts["log"] = enc.Fields
	}
	c.client.CaptureEvent(event, nil, nil)
	return nil
}

// eventTags picks the searchable attribution for an event. "solver" is the log field app
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

func (c *sentryCore) Sync() error {
	if c.flush != nil {
		c.flush()
	}
	return nil
}

func sentryLevel(l zapcore.Level) sentry.Level {
	if l >= zapcore.DPanicLevel {
		return sentry.LevelFatal
	}
	return sentry.LevelError
}
