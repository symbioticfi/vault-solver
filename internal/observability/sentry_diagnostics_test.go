package observability

import (
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/go-errors/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type testClientDiagnosticError struct{ id string }

func (e *testClientDiagnosticError) Error() string      { return "backend unavailable: " + e.id }
func (e *testClientDiagnosticError) ReasonCode() string { return "rfq_backend_request_failed" }
func (e *testClientDiagnosticError) RequestID() string  { return e.id }

func TestSentryDiagnosticGroupingAndContext(t *testing.T) {
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: "https://public@example.com/1", BeforeSend: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event { events = append(events, e); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	hub := sentry.CurrentHub()
	previous := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previous) })
	core := &sentryCore{level: zapcore.ErrorLevel}
	logger := zap.New(core).Named("lifi").With(zap.String("solver", "lifi"))
	logger.Error("order feed: ignored order", zap.String("reason_code", "lifi_invalid_integer"), zap.String("orderId", "one"), zap.Error(errors.New("invalid 123")))
	logger.Error("order feed: ignored order", zap.String("reason_code", "lifi_invalid_integer"), zap.String("orderId", "two"), zap.Error(errors.New("invalid 456")))
	logger.Error("order feed: ignored order", zap.String("reason_code", "lifi_invalid_address"))
	logger.Error("untouched log", zap.String("orderId", "one"))
	logger.Error("untouched log", zap.String("orderId", "two"))
	logger.Info("ignored unsupported order", zap.String("reason_code", "lifi_unsupported_order_type"))
	zap.New(core).Named("rfq").Error("order poll failed", zap.Error(errors.Errorf("poll: %w", &testClientDiagnosticError{id: "request-42"})))
	if len(events) != 6 {
		t.Fatalf("event frequency = %d, want 6", len(events))
	}
	if !reflect.DeepEqual(events[0].Fingerprint, events[1].Fingerprint) || reflect.DeepEqual(events[0].Fingerprint, events[2].Fingerprint) {
		t.Fatal("reason grouping failed")
	}
	if !reflect.DeepEqual(events[3].Fingerprint, []string{"lifi", "untouched log"}) || !reflect.DeepEqual(events[3].Fingerprint, events[4].Fingerprint) {
		t.Fatal("legacy grouping changed")
	}
	if events[0].Contexts["log"]["orderId"] != "one" || events[1].Contexts["log"]["orderId"] != "two" {
		t.Fatal("event context lost")
	}
	if !reflect.DeepEqual(events[5].Fingerprint, []string{"rfq", "order poll failed", "rfq_backend_request_failed"}) || events[5].Contexts["log"]["backendRequestId"] != "request-42" {
		t.Fatalf("wrapped diagnostic lost: %+v", events[5])
	}
}
