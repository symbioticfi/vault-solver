package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/go-logr/zapr"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

func TestBackendRequestIDPropagationAndFailures(t *testing.T) {
	tracetest.Install(t)
	for _, operation := range []string{"list", "executable", "get", "discounts", "resolve"} {
		for _, supplied := range []string{"", "inbound-request-42"} {
			t.Run(operation+"/"+supplied, func(t *testing.T) {
				var received, receivedTraceparent string
				calls := 0
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received = r.Header.Get(requestIDHeader)
					receivedTraceparent = r.Header.Get("traceparent")
					calls++
					w.Header().Set(requestIDHeader, "backend-generated-id")
					w.WriteHeader(http.StatusBadGateway)
				}))
				defer backend.Close()
				client := newBackendClient(backend.URL)
				invoke := func(ctx context.Context) error {
					ctx, span := otel.Tracer("test").Start(ctx, "op")
					defer span.End()
					switch operation {
					case "list":
						_, err := client.listOpenOrders(ctx, "filler", 10)
						return err
					case "executable":
						_, err := client.getExecutableOrder(ctx, "order", "filler")
						return err
					case "get":
						_, err := client.getOrder(ctx, "order")
						return err
					case "discounts":
						_, err := client.listDiscounts(ctx)
						return err
					default:
						_, err := client.resolveDiscount(ctx, "discount")
						return err
					}
				}
				var err error
				if supplied != "" {
					req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", nil)
					req.Header.Set(requestIDHeader, supplied)
					logRequests(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { err = invoke(r.Context()) }), logr.Discard()).ServeHTTP(httptest.NewRecorder(), req)
				} else {
					err = invoke(t.Context())
				}
				if received == "" || (supplied != "" && received != supplied) {
					t.Fatalf("received ID = %q", received)
				}
				if receivedTraceparent == "" {
					t.Fatalf("received traceparent = %q, want non-empty", receivedTraceparent)
				}
				var diagnostic *backendRequestError
				if !errors.As(err, &diagnostic) || diagnostic.RequestID() != received || strings.Contains(err.Error(), received) {
					t.Fatalf("error lost request ID: %v", err)
				}
				if diagnostic.ReasonCode() != "rfq_backend_request_failed" {
					t.Fatal(diagnostic.ReasonCode())
				}
				if calls != 1 {
					t.Fatalf("unexpected retries: %d", calls)
				}
			})
		}
	}
}

func TestBackendRequestIDSurvivesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), requestIDKey, "cancelled-operation"))
	cancel()
	_, err := newBackendClient("http://127.0.0.1:1").getOrder(ctx, "order")
	var diagnostic *backendRequestError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &diagnostic) || diagnostic.RequestID() != "cancelled-operation" || diagnostic.ReasonCode() != "rfq_backend_canceled" {
		t.Fatalf("error = %v", err)
	}
}

func TestBackendErrorLogsKeepCorrelationWithoutDynamicTitle(t *testing.T) {
	hub := sentry.CurrentHub()
	previous := hub.Client()
	t.Cleanup(func() { hub.BindClient(previous) })
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	production, flush := observability.NewLogger(false)
	t.Cleanup(flush)
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: "https://public@example.com/1", BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		events = append(events, event)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	hub.BindClient(client)
	var output bytes.Buffer
	jsonLog := zapr.NewLogger(zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.InfoLevel)))
	for _, id := range []string{"background-42", "inbound-99"} {
		ctx := context.WithValue(t.Context(), requestIDKey, id)
		err := errors.Errorf("poll: %w", backendError(ctx, errors.New("backend unavailable")))
		backendErrorLogger(production.WithName(Name), err).Error(err, "poll open orders")
		backendErrorLogger(jsonLog, err).Error(err, "poll open orders")
		var fields map[string]any
		if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		output.Reset()
		if fields["backendRequestId"] != id || strings.Contains(fields["error"].(string), id) {
			t.Fatalf("JSON correlation: %v", fields)
		}
	}
	if len(events) != 2 {
		t.Fatalf("event frequency = %d", len(events))
	}
	if events[0].Message != events[1].Message || strings.Contains(events[0].Message, "background-42") || !reflect.DeepEqual(events[0].Fingerprint, events[1].Fingerprint) {
		t.Fatalf("dynamic event title/grouping: %+v / %+v", events[0], events[1])
	}
	for i, id := range []string{"background-42", "inbound-99"} {
		if events[i].Contexts["log"]["backendRequestId"] != id {
			t.Fatalf("Sentry correlation lost: %+v", events[i])
		}
	}
}
