package lifi

import (
	"context"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestLIFIOrderQueueMetricsCollectLiveOwnerState(t *testing.T) {
	metrics, err := newLIFIMetrics(prometheus.NewRegistry(), nil, "")
	if err != nil {
		t.Fatal(err)
	}

	inbox := newOrderInbox(4)
	if err := inbox.enqueue(metricOrder("inbox-later", 1_500, 1_400)); err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(metricOrder("inbox-nearer", 1_300, 1_350)); err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(&submittedOrder{processed: make(chan struct{})}); err != nil {
		t.Fatal(err)
	}
	inbox.beginRecovery()
	inbox.markRecoveryRetry(metricOrder("recovery-retry", 1_150, 1_175), 0)

	capacityRetries := newReservationRetryQueue(2)
	if err := capacityRetries.enqueue(metricOrder("capacity", 1_200, 1_250), 0); err != nil {
		t.Fatal(err)
	}

	depositRetries := newOrderDepositRetryQueue(2)
	if err := depositRetries.schedule(metricOrder("deposit", 1_020, 1_030), time.Unix(1_000, 0)); err != nil {
		t.Fatal(err)
	}

	stopInbox := metrics.trackOrderQueue(orderQueueInbox, inbox.orderQueueSnapshot)
	stopRecovery := metrics.trackOrderQueue(
		orderQueueRecoveryRetry,
		inbox.recoveryRetryQueueSnapshot,
	)
	stopCapacity := metrics.trackOrderQueue(orderQueueCapacityRetry, capacityRetries.orderQueueSnapshot)
	stopDeposit := metrics.trackOrderQueue(orderQueueDepositRetry, depositRetries.orderQueueSnapshot)
	assertOrderQueueSnapshots(t, metrics.orderQueueMetrics, map[orderQueue]orderQueueSnapshot{
		orderQueueInbox:         {backlog: 2, nearestDeadline: 1_300},
		orderQueueRecoveryRetry: {backlog: 1, nearestDeadline: 1_150},
		orderQueueCapacityRetry: {backlog: 1, nearestDeadline: 1_200},
		orderQueueDepositRetry:  {backlog: 1, nearestDeadline: 1_020},
	})

	readyAt, ok := depositRetries.nextReadyAt()
	if !ok {
		t.Fatal("deposit retry was not scheduled")
	}
	if order, err := depositRetries.popReady(readyAt); err != nil || order == nil {
		t.Fatalf("pop deposit retry = %+v, %v", order, err)
	}
	if snapshot := depositRetries.orderQueueSnapshot(); snapshot != (orderQueueSnapshot{}) {
		t.Fatalf("deposit retry processing snapshot = %+v, want no queued order", snapshot)
	}

	stopInbox()
	stopRecovery()
	stopCapacity()
	stopDeposit()
	assertOrderQueueSnapshots(t, metrics.orderQueueMetrics, nil)
}

func TestLIFIOrderQueueMetricsIncludeBlockedInboxDelivery(t *testing.T) {
	inbox := newOrderInbox(1)
	if err := inbox.enqueue(metricOrder("waiting", 1_200, 1_100)); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	out := make(chan *submittedOrder)
	done := make(chan error, 1)
	go func() { done <- inbox.run(ctx, out) }()
	select {
	case <-inbox.space:
	case <-time.After(time.Second):
		t.Fatal("inbox did not start blocked delivery")
	}

	if snapshot := inbox.orderQueueSnapshot(); snapshot != (orderQueueSnapshot{backlog: 1, nearestDeadline: 1_100}) {
		t.Fatalf("blocked delivery snapshot = %+v", snapshot)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("inbox run error = %v, want cancellation", err)
	}
}

func TestLIFIOrderQueueMetricsConcurrentCollection(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(registry, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	const iterations = 256
	inbox := newOrderInbox(iterations)
	capacityRetries := newReservationRetryQueue(1)
	depositRetries := newOrderDepositRetryQueue(1)
	stopInbox := metrics.trackOrderQueue(orderQueueInbox, inbox.orderQueueSnapshot)
	defer stopInbox()
	stopCapacity := metrics.trackOrderQueue(orderQueueCapacityRetry, capacityRetries.orderQueueSnapshot)
	defer stopCapacity()
	stopDeposit := metrics.trackOrderQueue(orderQueueDepositRetry, depositRetries.orderQueueSnapshot)
	defer stopDeposit()

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-started
		now := time.Unix(1_000, 0)
		for index := range iterations {
			order := metricOrder(strconv.Itoa(index), 10_000, 11_000)
			if err := inbox.enqueue(order); err != nil {
				done <- err
				return
			}
			if err := capacityRetries.enqueue(order, uint64(index)); err != nil {
				done <- err
				return
			}
			capacityRetries.popReady(uint64(index + 1))
			if err := depositRetries.schedule(order, now); err != nil {
				done <- err
				return
			}
			readyAt, ok := depositRetries.nextReadyAt()
			if !ok {
				done <- errors.New("deposit retry was not scheduled")
				return
			}
			if _, err := depositRetries.popReady(readyAt); err != nil {
				done <- err
				return
			}
			depositRetries.finish(order)
			runtime.Gosched()
		}
		done <- nil
	}()
	close(started)
	for {
		if _, err := registry.Gather(); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			return
		default:
		}
	}
}

func TestLIFIMetricsRecordOnlyBoundedOrderSignals(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	metrics.observeOrderProcessing(orderProcessingSubmitted)
	metrics.observeOrderProcessing(orderProcessingOutcome("request-derived-value"))
	metrics.observeOrderQueueDrop(orderQueueInbox, errors.Errorf("wrapped: %w", errOrderInboxFull))
	metrics.observeOrderQueueDrop(orderQueueInbox, errOrderInboxClosed)
	metrics.observeOrderQueueDrop(orderQueueCapacityRetry, errOrderRetryFull)
	metrics.observeOrderQueueDrop(orderQueueDepositRetry, errOrderDepositRetryFull)
	metrics.observeOrderQueueDrop(orderQueueDepositRetry, errOrderDepositRetryExpired)
	metrics.observeOrderQueueDrop(orderQueue("request-derived-value"), errOrderInboxFull)

	for outcome, want := range map[string]float64{
		string(orderProcessingSubmitted): 1,
		string(orderProcessingOther):     1,
	} {
		metricstest.RequireWorkflowEventCount(t, reg, Name, "order_processing", outcome, want)
	}
	for _, queue := range orderDropQueues {
		metricstest.RequireWorkflowEventCount(t, reg, Name, "queue_drop", string(queue), 1)
	}

	var nilMetrics *lifiMetrics
	nilMetrics.observeOrderProcessing(orderProcessingSubmitted)
	nilMetrics.observeOrderQueueDrop(orderQueueInbox, errOrderInboxFull)
	stopTracking := nilMetrics.trackOrderQueue(orderQueueInbox, newOrderInbox(1).orderQueueSnapshot)
	stopTracking()
}

func assertOrderQueueSnapshots(
	t *testing.T,
	metrics *lifiOrderQueueMetrics,
	want map[orderQueue]orderQueueSnapshot,
) {
	t.Helper()
	for _, queue := range orderQueues {
		if got := metrics.snapshot(queue); got != want[queue] {
			t.Errorf("%s snapshot = %+v, want %+v", queue, got, want[queue])
		}
	}
}

func metricOrder(id string, expires, fillDeadline uint32) *submittedOrder {
	return &submittedOrder{
		OrderID: id,
		Order: inputsettler.StandardOrder{
			Expires:      expires,
			FillDeadline: fillDeadline,
		},
	}
}
