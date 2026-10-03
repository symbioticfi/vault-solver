package lifi

import (
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

func TestOrderWorkerCapacityAdmissionSignals(t *testing.T) {
	for _, tt := range []struct {
		name        string
		tracked     bool
		full        bool
		expired     bool
		wantOutcome orderProcessingOutcome
		wantDrop    float64
		wantError   bool
		wantRetain  bool
	}{
		{
			name: "retained by nonce timer", tracked: true, full: true,
			wantOutcome: orderProcessingCapacityDeferred, wantRetain: true,
		},
		{
			name: "retained by capacity queue", tracked: true,
			wantOutcome: orderProcessingCapacityDeferred, wantRetain: true,
		},
		{
			name: "genuinely dropped without timer", full: true,
			wantOutcome: orderProcessingCapacityDropped, wantDrop: 1, wantError: true,
		},
		{
			name: "nonce deadline elapsed during handoff", tracked: true, full: true, expired: true,
			wantOutcome: orderProcessingNotActionable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := tracetest.Install(t)
			log, logs := tracetest.CaptureLogs(t, 1)
			reg := prometheus.NewRegistry()
			metrics, err := newLIFIMetrics(reg, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			solver := &Solver{log: log, metrics: metrics}
			now := time.Unix(2_000_000_000, 0)
			deadline := now.Add(time.Minute)
			order := &submittedOrder{OrderID: "blocked"}
			capacityRetries := newReservationRetryQueue(1)
			nonceRetries := newOrderDepositRetryQueue(1)
			if tt.full {
				if err := capacityRetries.enqueue(&submittedOrder{OrderID: "already-waiting"}, 0); err != nil {
					t.Fatal(err)
				}
			}
			if tt.tracked {
				if err := nonceRetries.scheduleBefore(order, now, deadline); err != nil {
					t.Fatal(err)
				}
				now = now.Add(initialOrderDepositRetryBackoff)
				if got, err := nonceRetries.popReady(now); got != order || err != nil {
					t.Fatalf("start nonce retry = %v/%v, want ready order", got, err)
				}
				if tt.expired {
					now = deadline.Add(-initialOrderDepositRetryBackoff)
				}
			}

			ctx, end := tracer.Start(solverContext(t, solver), orderNonceStage)
			outcome, attemptErr := solver.deferOrderForCapacity(ctx, order, 0, capacityRetries, nonceRetries, now)
			solver.metrics.observeOrderProcessing(outcome)
			end(attemptErr)
			if outcome != tt.wantOutcome || (attemptErr != nil) != tt.wantError {
				t.Fatalf("capacity admission = %s/%v, want %s/error=%t", outcome, attemptErr, tt.wantOutcome, tt.wantError)
			}
			if tt.wantError && !errors.Is(attemptErr, errOrderRetryFull) {
				t.Fatalf("genuine capacity drop = %v, want full", attemptErr)
			}
			retained := capacityRetries.contains(order) || nonceRetries.contains(order)
			if retained != tt.wantRetain {
				t.Fatalf("order retained = %t, want %t", retained, tt.wantRetain)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "queue_drop", string(orderQueueCapacityRetry), tt.wantDrop)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "queue_drop", string(orderQueueNonceRetry), 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "order_processing", string(tt.wantOutcome), 1)
			if tt.wantOutcome != orderProcessingCapacityDropped {
				metricstest.RequireWorkflowEventCount(t, reg, Name, "order_processing", string(orderProcessingCapacityDropped), 0)
			}
			span := tracetest.Ended(t, rec, orderNonceStage)
			if (span.Status().Code == codes.Error) != tt.wantError {
				t.Fatalf("capacity admission span = %v, want error=%t", span.Status(), tt.wantError)
			}
			entries := strings.Join(logs(), "\n")
			if strings.Contains(entries, "dropped newest order") != tt.wantError ||
				strings.Contains(entries, `"error":`) != tt.wantError {
				t.Fatalf("capacity admission logs = %s, want drop/error=%t", entries, tt.wantError)
			}
			if tt.expired {
				if !strings.Contains(entries, "nonce reconciliation reached the order deadline") ||
					!tracetest.HasEvent(span, "declined") {
					t.Fatalf("expired capacity admission has no expected decline: logs=%s events=%v", entries, span.Events())
				}
			}
			if tt.tracked && tt.full && !tt.expired {
				readyAt, ok := nonceRetries.nextReadyAt()
				if !ok || readyAt.Sub(now) != 2*initialOrderDepositRetryBackoff {
					t.Fatalf("fallback backoff = %s/%t, want preserved 500ms", readyAt.Sub(now), ok)
				}
				if got, err := nonceRetries.popReady(deadline); got != order || !errors.Is(err, errOrderNonceRetryExpired) {
					t.Fatalf("fallback deadline = %v/%v, want original deadline expiry", got, err)
				}
			}
		})
	}
}
