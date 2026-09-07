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
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestLIFIOrderQueueMetricsCollectLiveOwnerState(t *testing.T) {
	metrics, err := newLIFIMetrics(prometheus.NewRegistry(), nil, "")
	testcheck.NoError(t, err)

	inbox := newOrderInbox(4)
	testcheck.NoError(t, inbox.enqueue(metricOrder("inbox-later", 1_500, 1_400)))
	testcheck.NoError(t, inbox.enqueue(metricOrder("inbox-nearer", 1_300, 1_350)))
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{processed: make(chan struct{})}))
	inbox.beginRecovery()
	inbox.markRecoveryRetry(metricOrder("recovery-retry", 1_150, 1_175), 0)

	retries := newOrderRetries(2, 2)
	testcheck.NoError(t, retries.enqueueCapacity(metricOrder("capacity", 1_200, 1_250), 0))

	testcheck.NoError(t, retries.scheduleDeposit(metricOrder("deposit", 1_020, 1_030), time.Unix(1_000, 0)))

	stopInbox := metrics.trackOrderQueue(orderQueueInbox, inbox.orderQueueSnapshot)
	stopRecovery := metrics.trackOrderQueue(
		orderQueueRecoveryRetry,
		inbox.recoveryRetryQueueSnapshot,
	)
	stopCapacity := metrics.trackOrderQueue(orderQueueCapacityRetry, func() orderQueueSnapshot { return retries.snapshot().capacity })
	stopDeposit := metrics.trackOrderQueue(orderQueueDepositRetry, func() orderQueueSnapshot { return retries.snapshot().deposit })
	assertOrderQueueSnapshots(t, metrics.orderQueueMetrics, map[orderQueue]orderQueueSnapshot{
		orderQueueInbox:         {backlog: 2, nearestDeadline: 1_300},
		orderQueueRecoveryRetry: {backlog: 1, nearestDeadline: 1_150},
		orderQueueCapacityRetry: {backlog: 1, nearestDeadline: 1_200},
		orderQueueDepositRetry:  {backlog: 1, nearestDeadline: 1_020},
	})

	readyAt, ok := retries.nextDepositAt()
	if !ok {
		t.Fatal("deposit retry was not scheduled")
	}
	if order, err := retries.popDeposit(readyAt); err != nil || order == nil {
		t.Fatalf("pop deposit retry = %+v, %v", order, err)
	}
	if snapshot := retries.snapshot().deposit; snapshot != (orderQueueSnapshot{}) {
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
	testcheck.NoError(t, inbox.enqueue(metricOrder("waiting", 1_200, 1_100)))

	ctx, cancel := context.WithCancel(t.Context())
	out := make(chan *submittedOrder)
	done := make(chan error, 1)
	go func() { done <- inbox.run(ctx, out) }()
	testcheck.ReceiveWithin(t, inbox.space, time.Second, "inbox did not start blocked delivery")

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
	testcheck.NoError(t, err)

	const iterations = 256
	inbox := newOrderInbox(iterations)
	retries := newOrderRetries(1, 1)
	stopInbox := metrics.trackOrderQueue(orderQueueInbox, inbox.orderQueueSnapshot)
	defer stopInbox()
	stopCapacity := metrics.trackOrderQueue(orderQueueCapacityRetry, func() orderQueueSnapshot { return retries.snapshot().capacity })
	defer stopCapacity()
	stopDeposit := metrics.trackOrderQueue(orderQueueDepositRetry, func() orderQueueSnapshot { return retries.snapshot().deposit })
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
			if err := retries.enqueueCapacity(order, uint64(index)); err != nil {
				done <- err
				return
			}
			retries.popCapacity(uint64(index + 1))
			if err := retries.scheduleDeposit(order, now); err != nil {
				done <- err
				return
			}
			readyAt, ok := retries.nextDepositAt()
			if !ok {
				done <- errors.New("deposit retry was not scheduled")
				return
			}
			if _, err := retries.popDeposit(readyAt); err != nil {
				done <- err
				return
			}
			retries.finishDeposit(order)
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
			testcheck.NoError(t, err)
			return
		default:
		}
	}
}

func TestLIFIMetricsRecordOnlyBoundedOrderSignals(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, nil, "")
	testcheck.NoError(t, err)

	metrics.observeOrderProcessing(orderProcessingSubmitted)
	metrics.observeOrderProcessing(orderProcessingOutcome("request-derived-value"))
	metrics.observeOrderQueueDrop(orderQueueInbox, errors.Errorf("wrapped: %w", errOrderInboxFull))
	metrics.observeOrderQueueDrop(orderQueueInbox, errOrderInboxClosed)
	metrics.observeOrderQueueDrop(orderQueueCapacityRetry, errOrderRetryFull)
	metrics.observeOrderQueueDrop(orderQueueDepositRetry, errOrderDepositRetryFull)
	metrics.observeOrderQueueDrop(orderQueueDepositRetry, errOrderDepositRetryKey)
	metrics.observeOrderQueueDrop(orderQueueDepositRetry, errOrderDepositRetryExpired)
	metrics.observeOrderQueueDrop(orderQueueDepositRetry, errOrderDepositRetryWindow)
	metrics.observeOrderQueueDrop(orderQueue("request-derived-value"), errOrderInboxFull)

	for outcome, want := range map[string]float64{
		string(orderProcessingSubmitted): 1,
		string(orderProcessingOther):     1,
	} {
		metricstest.RequireWorkflowEventCount(t, reg, Name, "order_processing", outcome, want)
	}
	for _, queue := range orderDropQueues {
		want := float64(1)
		if queue == orderQueueDepositRetry {
			want = 4
		}
		metricstest.RequireWorkflowEventCount(t, reg, Name, "queue_drop", string(queue), want)
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
