package lifi

import (
	"testing"
	"time"

	"github.com/go-errors/errors"
)

func TestOrderDepositRetryQueueIsBoundedAndCoalescesReplays(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	queue := newOrderDepositRetryQueue(1)
	first := &submittedOrder{OrderID: "first"}
	if err := queue.schedule(first, now); err != nil {
		t.Fatal(err)
	}
	if err := queue.schedule(&submittedOrder{OrderID: "first"}, now); err != nil {
		t.Fatal(err)
	}
	if queue.len() != 1 || len(queue.byKey) != 1 {
		t.Fatalf("coalesced queue: ready=%d tracked=%d, want 1/1", queue.len(), len(queue.byKey))
	}
	if err := queue.schedule(&submittedOrder{OrderID: "second"}, now); !errors.Is(err, errOrderDepositRetryFull) {
		t.Fatalf("overflow error = %v, want %v", err, errOrderDepositRetryFull)
	}
	if order := popOrderDepositRetry(t, queue, now.Add(initialOrderDepositRetryBackoff-time.Nanosecond)); order != nil {
		t.Fatalf("order became ready before its backoff: %+v", order)
	}
	if order := popOrderDepositRetry(t, queue, now.Add(initialOrderDepositRetryBackoff)); order != first {
		t.Fatalf("ready order = %p, want %p", order, first)
	}
}

func TestOrderDepositRetryQueueStopsAtOrderDeadline(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "expiring"}
	order.Order.Expires = uint32(now.Add(2 * time.Second).Unix())
	queue := newOrderDepositRetryQueue(1)

	if err := queue.schedule(order, now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		readyAt, ok := queue.nextReadyAt()
		if !ok || popOrderDepositRetry(t, queue, readyAt) != order {
			t.Fatal("scheduled retry was not ready")
		}
		if err := queue.schedule(order, readyAt); err != nil {
			t.Fatalf("reschedule before deadline: %v", err)
		}
	}
	readyAt, ok := queue.nextReadyAt()
	if !ok || popOrderDepositRetry(t, queue, readyAt) != order {
		t.Fatal("final retry before deadline was not ready")
	}
	if err := queue.schedule(order, readyAt); !errors.Is(err, errOrderDepositRetryExpired) {
		t.Fatalf("deadline error = %v, want %v", err, errOrderDepositRetryExpired)
	}
	if queue.len() != 0 || len(queue.byKey) != 0 {
		t.Fatalf("expired queue: ready=%d tracked=%d, want 0/0", queue.len(), len(queue.byKey))
	}
}

func TestOrderDepositRetryQueueStopsAfterNineRetries(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "missing"}
	queue := newOrderDepositRetryQueue(1)
	if err := queue.schedule(order, now); err != nil {
		t.Fatal(err)
	}
	for retry := 1; retry <= maximumOrderDepositRetries; retry++ {
		readyAt, ok := queue.nextReadyAt()
		if !ok || popOrderDepositRetry(t, queue, readyAt) != order {
			t.Fatalf("retry %d was not ready", retry)
		}
		err := queue.schedule(order, readyAt)
		if retry < maximumOrderDepositRetries {
			if err != nil {
				t.Fatalf("retry %d reschedule: %v", retry, err)
			}
			continue
		}
		if !errors.Is(err, errOrderDepositRetryAttempts) {
			t.Fatalf("retry %d error = %v, want %v", retry, err, errOrderDepositRetryAttempts)
		}
	}
	if queue.len() != 0 || len(queue.byKey) != 0 {
		t.Fatalf("exhausted queue: ready=%d tracked=%d, want 0/0", queue.len(), len(queue.byKey))
	}
}

func TestOrderDepositRetryDelayUsesRetryClock(t *testing.T) {
	now := time.Unix(1_234_567, 0)
	for _, tt := range []struct {
		name    string
		readyAt time.Time
		want    time.Duration
	}{
		{name: "future", readyAt: now.Add(3 * time.Second), want: 3 * time.Second},
		{name: "ready", readyAt: now},
		{name: "past", readyAt: now.Add(-time.Second)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := orderDepositRetryDelay(tt.readyAt, now); got != tt.want {
				t.Fatalf("delay = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOrderDepositRetryQueueRejectsLateDequeue(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "late"}
	queue := newOrderDepositRetryQueue(1)
	if err := queue.schedule(order, now); err != nil {
		t.Fatal(err)
	}

	got, err := queue.popReady(now.Add(maximumOrderDepositRetryWindow))
	if got != order {
		t.Fatalf("late order = %p, want %p", got, order)
	}
	if !errors.Is(err, errOrderDepositRetryWindow) {
		t.Fatalf("late dequeue error = %v, want %v", err, errOrderDepositRetryWindow)
	}
	if queue.len() != 0 || len(queue.byKey) != 0 {
		t.Fatalf("late queue: ready=%d tracked=%d, want 0/0", queue.len(), len(queue.byKey))
	}
}

func popOrderDepositRetry(
	t *testing.T,
	queue *orderDepositRetryQueue,
	now time.Time,
) *submittedOrder {
	t.Helper()
	order, err := queue.popReady(now)
	if err != nil {
		t.Fatalf("pop ready: %v", err)
	}
	return order
}
