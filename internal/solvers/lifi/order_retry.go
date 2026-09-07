package lifi

import (
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
)

const (
	initialOrderDepositRetryBackoff = 250 * time.Millisecond
	maximumOrderDepositRetryBackoff = 5 * time.Second
	maximumOrderDepositRetryWindow  = 30 * time.Second
)

var (
	errOrderDepositRetryFull    = errors.New("order deposit retry queue is full")
	errOrderDepositRetryKey     = errors.New("order deposit retry requires a stable order key")
	errOrderDepositRetryExpired = errors.New("order expired before deposit became visible")
	errOrderDepositRetryWindow  = errors.New("order deposit retry window elapsed")
)

type orderDepositRetry struct {
	order     *submittedOrder
	backoff   time.Duration
	startedAt time.Time
	readyAt   time.Time
}

type reservationRetry struct {
	order      *submittedOrder
	generation uint64
}

// orderRetries is owned by the order worker. Capacity waits retain FIFO generations;
// deposit waits retain their own backoff/window even while a status read is in progress.
// A replay may wait for both reasons, so the two indices must remain independent.
// Only immutable queue observations are read by concurrent metrics collectors.
type orderRetries struct {
	observed                    atomic.Pointer[retryQueueSnapshot]
	capacity                    []reservationRetry
	deposits                    map[string]*orderDepositRetry
	capacityLimit, depositLimit int
}

type retryQueueSnapshot struct{ capacity, deposit orderQueueSnapshot }

func newOrderRetries(capacityLimit, depositLimit int) *orderRetries {
	if capacityLimit <= 0 || depositLimit <= 0 {
		panic("lifi: order retry capacities must be positive")
	}
	return &orderRetries{deposits: make(map[string]*orderDepositRetry), capacityLimit: capacityLimit, depositLimit: depositLimit}
}

func (q *orderRetries) enqueueCapacity(order *submittedOrder, generation uint64) error {
	defer q.publish(orderQueueCapacityRetry)

	key := orderInboxKey(order)
	if key != "" {
		for _, item := range q.capacity {
			if orderInboxKey(item.order) == key {
				return nil
			}
		}
	}
	if len(q.capacity) >= q.capacityLimit {
		return errOrderRetryFull
	}
	q.capacity = append(q.capacity, reservationRetry{order: order, generation: generation})
	return nil
}

func (q *orderRetries) popCapacity(generation uint64) *submittedOrder {
	defer q.publish(orderQueueCapacityRetry)

	if len(q.capacity) == 0 || q.capacity[0].generation >= generation {
		return nil
	}
	item := q.capacity[0]
	q.capacity[0] = reservationRetry{}
	q.capacity = q.capacity[1:]
	if len(q.capacity) == 0 {
		q.capacity = nil
	}
	return item.order
}

func (q *orderRetries) scheduleDeposit(order *submittedOrder, now time.Time) error {
	defer q.publish(orderQueueDepositRetry)

	key := orderInboxKey(order)
	if key == "" {
		return errOrderDepositRetryKey
	}
	state := q.deposits[key]
	if state != nil && !state.readyAt.IsZero() {
		return nil
	}
	if state == nil {
		if len(q.deposits) >= q.depositLimit {
			return errOrderDepositRetryFull
		}
		state = &orderDepositRetry{startedAt: now}
		q.deposits[key] = state
	}
	state.order = order
	retryEnd, boundErr := orderDepositRetryEnd(order, state.startedAt)
	finalReadyAt := retryEnd.Add(-initialOrderDepositRetryBackoff)
	if !now.Before(finalReadyAt) {
		delete(q.deposits, key)
		return boundErr
	}

	if state.backoff == 0 {
		state.backoff = initialOrderDepositRetryBackoff
	} else {
		state.backoff = min(2*state.backoff, maximumOrderDepositRetryBackoff)
	}
	state.readyAt = now.Add(state.backoff)
	if finalReadyAt.Before(state.readyAt) {
		state.readyAt = finalReadyAt
	}
	return nil
}

func orderDepositRetryEnd(order *submittedOrder, startedAt time.Time) (time.Time, error) {
	windowEnd := startedAt.Add(maximumOrderDepositRetryWindow)
	deadline := orderDeadline(order)
	if !deadline.IsZero() && !deadline.After(windowEnd) {
		return deadline, errOrderDepositRetryExpired
	}
	return windowEnd, errOrderDepositRetryWindow
}

func (q *orderRetries) nextDepositAt() (time.Time, bool) {
	state := q.nextDeposit()
	if state == nil {
		return time.Time{}, false
	}
	return state.readyAt, true
}

func (q *orderRetries) popDeposit(now time.Time) (*submittedOrder, error) {
	defer q.publish(orderQueueDepositRetry)

	state := q.nextDeposit()
	if state == nil || state.readyAt.After(now) {
		return nil, nil
	}
	state.readyAt = time.Time{}
	retryEnd, boundErr := orderDepositRetryEnd(state.order, state.startedAt)
	if !now.Before(retryEnd) {
		delete(q.deposits, orderInboxKey(state.order))
		return state.order, boundErr
	}
	return state.order, nil
}

func (q *orderRetries) nextDeposit() *orderDepositRetry {
	var next *orderDepositRetry
	for _, state := range q.deposits {
		if state.readyAt.IsZero() {
			continue
		}
		if next == nil || state.readyAt.Before(next.readyAt) {
			next = state
		}
	}
	return next
}

func (q *orderRetries) finishDeposit(order *submittedOrder) {
	defer q.publish(orderQueueDepositRetry)

	delete(q.deposits, orderInboxKey(order))
}

func (q *orderRetries) clearCapacity() { q.capacity = nil; q.publish(orderQueueCapacityRetry) }
func (q *orderRetries) clearDeposits() { clear(q.deposits); q.publish(orderQueueDepositRetry) }

func (q *orderRetries) snapshot() retryQueueSnapshot {
	if observed := q.observed.Load(); observed != nil {
		return *observed
	}
	return retryQueueSnapshot{}
}

// Update only the changed queue; a timed deposit retry must not rescan a full capacity backlog.
func (q *orderRetries) publish(queue orderQueue) {
	observation := q.snapshot()
	switch queue {
	case orderQueueInbox, orderQueueRecoveryRetry:
		panic("lifi: order retries only observe capacity and deposit queues")
	case orderQueueCapacityRetry:
		observation.capacity = orderQueueSnapshot{backlog: len(q.capacity)}
		for _, item := range q.capacity {
			observation.capacity.nearestDeadline = earlierOrderDeadlineUnix(observation.capacity.nearestDeadline, item.order)
		}
	case orderQueueDepositRetry:
		observation.deposit = orderQueueSnapshot{}
		for _, state := range q.deposits {
			if !state.readyAt.IsZero() {
				observation.deposit.backlog++
				observation.deposit.nearestDeadline = earlierOrderDeadlineUnix(observation.deposit.nearestDeadline, state.order)
			}
		}
	}
	q.observed.Store(&observation)
}
