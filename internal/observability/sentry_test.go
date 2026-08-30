package observability

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

type countingTransport struct {
	flushes     atomic.Int64
	lastTimeout atomic.Int64
}

func (t *countingTransport) Flush(timeout time.Duration) bool {
	t.flushes.Add(1)
	t.lastTimeout.Store(int64(timeout))
	return true
}

func (*countingTransport) FlushWithContext(context.Context) bool { return true }
func (*countingTransport) Configure(sentry.ClientOptions)        {}
func (*countingTransport) SendEvent(*sentry.Event)               {}
func (*countingTransport) Close()                                {}

func TestSentryCore_Enabled(t *testing.T) {
	core := &sentryCore{}
	tests := map[string]struct {
		level zapcore.Level
		want  bool
	}{
		"debug":  {level: zapcore.DebugLevel},
		"warn":   {level: zapcore.WarnLevel},
		"error":  {level: zapcore.ErrorLevel, want: true},
		"dpanic": {level: zapcore.DPanicLevel, want: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := core.Enabled(test.level); got != test.want {
				t.Fatalf("Enabled(%s) = %t, want %t", test.level, got, test.want)
			}
		})
	}
}

func TestSentryCore_With(t *testing.T) {
	core := &sentryCore{fields: []zapcore.Field{{Key: "existing"}}}
	with := core.With([]zapcore.Field{{Key: "added"}}).(*sentryCore)

	if len(core.fields) != 1 || core.fields[0].Key != "existing" {
		t.Fatalf("original fields changed: %+v", core.fields)
	}
	if len(with.fields) != 2 || with.fields[0].Key != "existing" || with.fields[1].Key != "added" {
		t.Fatalf("With fields = %+v", with.fields)
	}
	if with.Enabled(zapcore.WarnLevel) || !with.Enabled(zapcore.ErrorLevel) {
		t.Fatal("With changed the Error+ threshold")
	}
}

func TestNewLoggerCleanupFlushesSentryOnce(t *testing.T) {
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")

	hub := sentry.CurrentHub()
	previousClient := hub.Client()
	t.Cleanup(func() { hub.BindClient(previousClient) })

	_, cleanup := NewLogger(false)
	transport := &countingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:                    "https://public@example.com/1",
		Transport:              transport,
		DisableLogs:            true,
		DisableMetrics:         true,
		DisableTelemetryBuffer: true,
	})
	if err != nil {
		t.Fatalf("create Sentry client: %v", err)
	}
	hub.BindClient(client)

	cleanup()

	if got := transport.flushes.Load(); got != 1 {
		t.Fatalf("expected logger cleanup to flush Sentry once, got %d flushes", got)
	}
	if got := time.Duration(transport.lastTimeout.Load()); got != sentryFlushTimeout {
		t.Fatalf("expected Sentry flush timeout %s, got %s", sentryFlushTimeout, got)
	}
}

// TestInitSentry_DisabledWithoutDSN confirms the sink is strictly opt-in: with no SENTRY_DSN, no core
// is installed.
func TestInitSentry_DisabledWithoutDSN(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	core := initSentry()
	if core != nil {
		t.Fatalf("expected nil core when SENTRY_DSN is unset, got %T", core)
	}
}
