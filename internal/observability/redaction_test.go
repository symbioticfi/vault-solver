package observability

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-errors/errors"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const syntheticURL = "https://synthetic-user:synthetic-password@rpc.invalid/synthetic-path?key=synthetic-query#synthetic-fragment" //nolint:gosec // G101: invented credentials exclusively for redaction regressions.

func TestRedactBothLogSinks(t *testing.T) {
	var output bytes.Buffer
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: "https://public@example.com/1", BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		events = append(events, event)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	hub := sentry.CurrentHub()
	previous := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previous) })
	stdout := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel)
	core := &redactingCore{zapcore.NewTee(stdout, &sentryCore{level: zapcore.ErrorLevel})}
	logger := zapr.NewLogger(zap.New(core).Named("rfq")).WithValues("endpoint", syntheticURL, "count", 3, "ready", true)
	logger.V(1).Info("debug "+syntheticURL, "url", syntheticURL)
	logger.Info("ordinary info", "url", syntheticURL)
	diagnostic := errors.Errorf("request %s: %w", syntheticURL, &testClientDiagnosticError{id: "request-42"})
	logger.Error(&url.Error{Op: "Post", URL: syntheticURL, Err: diagnostic}, "read failed "+syntheticURL, "url", syntheticURL)
	if len(events) != 1 {
		t.Fatalf("Sentry events = %d, want 1", len(events))
	}
	encoded, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{output.String(), string(encoded)} {
		for _, marker := range []string{"synthetic-user", "synthetic-password", "synthetic-path", "synthetic-query", "synthetic-fragment"} {
			if strings.Contains(value, marker) {
				t.Error("URL credential marker leaked into log sink")
			}
		}
		if !strings.Contains(value, "https://rpc.invalid") {
			t.Error("endpoint diagnostic lost")
		}
	}
	fields := events[0].Contexts["log"]
	ready, _ := fields["ready"].(bool)
	if fields["reason_code"] != "rfq_backend_request_failed" || fields["backendRequestId"] != "request-42" || fields["count"] != int64(3) || !ready {
		t.Error("structured diagnostics changed")
	}
	if len(events[0].Fingerprint) != 3 || events[0].Fingerprint[2] != "rfq_backend_request_failed" {
		t.Error("diagnostic grouping changed")
	}
}

func TestRedactPreservesSamplingAndErrorOutput(t *testing.T) {
	for _, redact := range []bool{false, true} {
		var output bytes.Buffer
		core := zapcore.NewSamplerWithOptions(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel), time.Hour, 1, 0)
		if redact {
			core = &redactingCore{core}
		}
		logger := zap.New(core)
		for range 4 {
			logger.Info("same message", zap.String("endpoint", syntheticURL))
		}
		if strings.Count(output.String(), "\n") != 1 {
			t.Error("sampling changed")
		}
	}
	var errorOutput bytes.Buffer
	core := &redactingCore{zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(failingLogWriter{}), zapcore.InfoLevel)}
	zap.New(core, zap.ErrorOutput(zapcore.AddSync(&errorOutput))).Info("write failure")
	if !strings.Contains(errorOutput.String(), "synthetic sink failure") {
		t.Error("logger error output lost")
	}
}

type failingLogWriter struct{}

func (failingLogWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic sink failure") }

func TestRedactConcurrentBoundFields(t *testing.T) {
	var output bytes.Buffer
	core := &redactingCore{zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.Lock(zapcore.AddSync(&output)), zapcore.DebugLevel)}
	logger := zap.New(core).With(zap.String("endpoint", syntheticURL))
	fields := []zap.Field{zap.String("dynamic", syntheticURL)}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() { logger.Info("concurrent", fields...) })
	}
	workers.Wait()
	if fields[0].String != syntheticURL {
		t.Error("caller fields mutated")
	}
	if strings.Contains(output.String(), "synthetic-path") {
		t.Error("concurrent redaction bypass")
	}
}
