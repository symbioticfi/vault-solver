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

// orderDepositRetryQueue is mutated exclusively by the order worker; metrics may
// read immutable published observations concurrently. A state remains indexed while its order is
// being retried, so a concurrent feed replay cannot reset the backoff/window state
// between status reads.
type orderDepositRetryQueue struct {
	observed atomic.Pointer[orderQueueSnapshot]
	byKey    map[string]*orderDepositRetry
	capacity int
}

func newOrderDepositRetryQueue(capacity int) *orderDepositRetryQueue {
	if capacity <= 0 {
		panic("lifi: order deposit retry capacity must be positive")
	}
	return &orderDepositRetryQueue{
		byKey:    make(map[string]*orderDepositRetry),
		capacity: capacity,
	}
}

func (q *orderDepositRetryQueue) schedule(order *submittedOrder, now time.Time) error {
	defer q.publish()

	key := orderInboxKey(order)
	if key == "" {
		return errOrderDepositRetryKey
	}
	state := q.byKey[key]
	if state != nil && !state.readyAt.IsZero() {
		return nil
	}
	if state == nil {
		if len(q.byKey) >= q.capacity {
			return errOrderDepositRetryFull
		}
		state = &orderDepositRetry{startedAt: now}
		q.byKey[key] = state
	}
	state.order = order
	retryEnd, boundErr := orderDepositRetryEnd(order, state.startedAt)
	finalReadyAt := retryEnd.Add(-initialOrderDepositRetryBackoff)
	if !now.Before(finalReadyAt) {
		delete(q.byKey, key)
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

func (q *orderDepositRetryQueue) contains(order *submittedOrder) bool {
	_, ok := q.byKey[orderInboxKey(order)]
	return ok
}

func (q *orderDepositRetryQueue) nextReadyAt() (time.Time, bool) {
	state := q.next()
	if state == nil {
		return time.Time{}, false
	}
	return state.readyAt, true
}

func (q *orderDepositRetryQueue) popReady(now time.Time) (*submittedOrder, error) {
	defer q.publish()

	state := q.next()
	if state == nil || state.readyAt.After(now) {
		return nil, nil
	}
	state.readyAt = time.Time{}
	retryEnd, boundErr := orderDepositRetryEnd(state.order, state.startedAt)
	if !now.Before(retryEnd) {
		delete(q.byKey, orderInboxKey(state.order))
		return state.order, boundErr
	}
	return state.order, nil
}

func (q *orderDepositRetryQueue) next() *orderDepositRetry {
	var next *orderDepositRetry
	for _, state := range q.byKey {
		if state.readyAt.IsZero() {
			continue
		}
		if next == nil || state.readyAt.Before(next.readyAt) {
			next = state
		}
	}
	return next
}

func (q *orderDepositRetryQueue) finish(order *submittedOrder) {
	defer q.publish()

	delete(q.byKey, orderInboxKey(order))
}

func (q *orderDepositRetryQueue) len() int {
	return len(q.byKey)
}

func (q *orderDepositRetryQueue) clear() {
	defer q.publish()

	clear(q.byKey)
}

func (q *orderDepositRetryQueue) orderQueueSnapshot() orderQueueSnapshot {
	if observed := q.observed.Load(); observed != nil {
		return *observed
	}
	return orderQueueSnapshot{}
}

func (q *orderDepositRetryQueue) publish() {
	observation := &orderQueueSnapshot{}
	for _, state := range q.byKey {
		if !state.readyAt.IsZero() {
			observation.backlog++
			observation.nearestDeadline = earlierOrderDeadlineUnix(observation.nearestDeadline, state.order)
		}
	}
	q.observed.Store(observation)
}
