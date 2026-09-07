package lifi

import (
	"testing"
	"time"

	"github.com/go-errors/errors"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestOrderDepositRetryQueueIsBoundedAndCoalescesReplays(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	queue := newOrderRetries(1, 1)
	first := &submittedOrder{OrderID: "first"}
	testcheck.NoError(t, queue.scheduleDeposit(first, now))
	testcheck.NoError(t, queue.scheduleDeposit(&submittedOrder{OrderID: "first"}, now))
	if len(queue.deposits) != 1 {
		t.Fatalf("coalesced queue: tracked=%d, want 1", len(queue.deposits))
	}
	if err := queue.scheduleDeposit(&submittedOrder{OrderID: "second"}, now); !errors.Is(err, errOrderDepositRetryFull) {
		t.Fatalf("overflow error = %v, want %v", err, errOrderDepositRetryFull)
	}
	if order := popOrderDepositRetry(t, queue, now.Add(initialOrderDepositRetryBackoff-time.Nanosecond)); order != nil {
		t.Fatalf("order became ready before its backoff: %+v", order)
	}
	if order := popOrderDepositRetry(t, queue, now.Add(initialOrderDepositRetryBackoff)); order != first {
		t.Fatalf("ready order = %p, want %p", order, first)
	}
}

func TestOrderDepositRetryQueueReturnsEarliestState(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	queue := newOrderRetries(1, 2)
	later := &submittedOrder{OrderID: "later"}
	earlier := &submittedOrder{OrderID: "earlier"}
	testcheck.NoError(t, queue.scheduleDeposit(later, now.Add(100*time.Millisecond)))
	testcheck.NoError(t, queue.scheduleDeposit(earlier, now))

	readyAt, ok := queue.nextDepositAt()
	if !ok || !readyAt.Equal(now.Add(initialOrderDepositRetryBackoff)) {
		t.Fatalf("next retry = %s/%t", readyAt, ok)
	}
	if order := popOrderDepositRetry(t, queue, readyAt); order != earlier {
		t.Fatalf("first retry = %p, want %p", order, earlier)
	}
	readyAt, ok = queue.nextDepositAt()
	if !ok || !readyAt.Equal(now.Add(100*time.Millisecond+initialOrderDepositRetryBackoff)) {
		t.Fatalf("following retry = %s/%t", readyAt, ok)
	}
	if order := popOrderDepositRetry(t, queue, readyAt); order != later {
		t.Fatalf("second retry = %p, want %p", order, later)
	}
}

func TestOrderDepositRetryQueueStopsAtOrderDeadline(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "expiring"}
	order.Order.Expires = uint32(now.Add(2 * time.Second).Unix())
	queue := newOrderRetries(1, 1)

	testcheck.NoError(t, queue.scheduleDeposit(order, now))
	for range 2 {
		readyAt, ok := queue.nextDepositAt()
		if !ok || popOrderDepositRetry(t, queue, readyAt) != order {
			t.Fatal("scheduled retry was not ready")
		}
		testcheck.NoError(t, queue.scheduleDeposit(order, readyAt), "reschedule before deadline: %v")
	}
	readyAt, ok := queue.nextDepositAt()
	if !ok || popOrderDepositRetry(t, queue, readyAt) != order {
		t.Fatal("final retry before deadline was not ready")
	}
	if err := queue.scheduleDeposit(order, readyAt); !errors.Is(err, errOrderDepositRetryExpired) {
		t.Fatalf("deadline error = %v, want %v", err, errOrderDepositRetryExpired)
	}
	if len(queue.deposits) != 0 {
		t.Fatalf("expired queue: tracked=%d, want 0", len(queue.deposits))
	}
}

func TestOrderDepositRetryQueueSchedulesFinalReadBeforeWindow(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "missing"}
	queue := newOrderRetries(1, 1)
	wantOffsets := []time.Duration{
		250 * time.Millisecond,
		750 * time.Millisecond,
		1750 * time.Millisecond,
		3750 * time.Millisecond,
		7750 * time.Millisecond,
		12750 * time.Millisecond,
		17750 * time.Millisecond,
		22750 * time.Millisecond,
		27750 * time.Millisecond,
		maximumOrderDepositRetryWindow - initialOrderDepositRetryBackoff,
	}
	checkAt := now
	for retry, wantOffset := range wantOffsets {
		if err := queue.scheduleDeposit(order, checkAt); err != nil {
			t.Fatalf("retry %d schedule: %v", retry+1, err)
		}
		readyAt, ok := queue.nextDepositAt()
		if !ok {
			t.Fatalf("retry %d was not scheduled", retry+1)
		}
		if got := readyAt.Sub(now); got != wantOffset {
			t.Fatalf("retry %d offset = %s, want %s", retry+1, got, wantOffset)
		}
		if popOrderDepositRetry(t, queue, readyAt) != order {
			t.Fatalf("retry %d was not ready", retry+1)
		}
		checkAt = readyAt
	}
	if err := queue.scheduleDeposit(order, checkAt); !errors.Is(err, errOrderDepositRetryWindow) {
		t.Fatalf("final retry error = %v, want %v", err, errOrderDepositRetryWindow)
	}
	if len(queue.deposits) != 0 {
		t.Fatalf("elapsed queue: tracked=%d, want 0", len(queue.deposits))
	}
}

func TestOrderDepositRetryQueueRejectsLateDequeue(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "late"}
	queue := newOrderRetries(1, 1)
	testcheck.NoError(t, queue.scheduleDeposit(order, now))

	got, err := queue.popDeposit(now.Add(maximumOrderDepositRetryWindow))
	if got != order {
		t.Fatalf("late order = %p, want %p", got, order)
	}
	if !errors.Is(err, errOrderDepositRetryWindow) {
		t.Fatalf("late dequeue error = %v, want %v", err, errOrderDepositRetryWindow)
	}
	if len(queue.deposits) != 0 {
		t.Fatalf("late queue: tracked=%d, want 0", len(queue.deposits))
	}
}

func TestOrderRetriesKeepCapacityAndDepositWaitsIndependent(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "same-order"}
	retries := newOrderRetries(1, 1)
	testcheck.NoError(t, retries.scheduleDeposit(order, now))
	testcheck.NoError(t, retries.enqueueCapacity(order, 0))
	if retries.popCapacity(1) != order || len(retries.deposits) != 1 {
		t.Fatal("capacity release lost the deposit wait")
	}
	testcheck.NoError(t, retries.enqueueCapacity(order, 1))
	if popOrderDepositRetry(t, retries, now.Add(initialOrderDepositRetryBackoff)) != order {
		t.Fatal("deposit retry lost its original backoff")
	}
	if len(retries.deposits) != 1 || retries.snapshot().deposit.backlog != 0 || retries.snapshot().capacity.backlog != 1 {
		t.Fatal("in-progress deposit must remain tracked independently of queued capacity")
	}
	retries.finishDeposit(order)
	if retries.popCapacity(2) != order || len(retries.deposits) != 0 {
		t.Fatal("finishing the deposit retry lost the capacity wait")
	}
}

func popOrderDepositRetry(
	t *testing.T,
	queue *orderRetries,
	now time.Time,
) *submittedOrder {
	t.Helper()
	order, err := queue.popDeposit(now)
	testcheck.NoError(t, err, "pop ready: %v")
	return order
}
