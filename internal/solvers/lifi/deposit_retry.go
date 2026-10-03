package lifi

import (
	"sync"
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
	errOrderNonceRetryExpired   = errors.New("order deadline elapsed during nonce reconciliation")
)

type orderDepositRetry struct {
	order     *submittedOrder
	backoff   time.Duration
	startedAt time.Time
	deadline  time.Time
	readyAt   time.Time
}

// orderDepositRetryQueue supplies timed retries for deposit propagation and uncertain fills.
// It is mutated exclusively by the order worker; metrics may
// take read-only snapshots concurrently. A state remains indexed while its order is
// being retried, so a concurrent feed replay cannot reset the backoff/window state
// between status reads.
type orderDepositRetryQueue struct {
	mu       sync.RWMutex
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
	return q.scheduleBefore(order, now, time.Time{})
}

// scheduleBefore reuses the replay-coalescing timer for an uncertain fill. Its bound is the
// wall-clock order deadline captured at admission, rather than the deposit-propagation window.
func (q *orderDepositRetryQueue) scheduleBefore(order *submittedOrder, now, deadline time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()

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
	if !deadline.IsZero() {
		state.deadline = earlierDeadline(state.deadline, deadline)
	}
	retryEnd, boundErr := state.retryEnd()
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

func (s *orderDepositRetry) retryEnd() (time.Time, error) {
	if !s.deadline.IsZero() {
		return s.deadline, errOrderNonceRetryExpired
	}
	return orderDepositRetryEnd(s.order, s.startedAt)
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
	q.mu.RLock()
	defer q.mu.RUnlock()

	_, ok := q.byKey[orderInboxKey(order)]
	return ok
}

func (q *orderDepositRetryQueue) nextReadyAt() (time.Time, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	state := q.nextLocked()
	if state == nil {
		return time.Time{}, false
	}
	return state.readyAt, true
}

func (q *orderDepositRetryQueue) popReady(now time.Time) (*submittedOrder, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	state := q.nextLocked()
	if state == nil || state.readyAt.After(now) {
		return nil, nil
	}
	state.readyAt = time.Time{}
	retryEnd, boundErr := state.retryEnd()
	if !now.Before(retryEnd) {
		delete(q.byKey, orderInboxKey(state.order))
		return state.order, boundErr
	}
	return state.order, nil
}

func (q *orderDepositRetryQueue) nextLocked() *orderDepositRetry {
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
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.byKey, orderInboxKey(order))
}

func (q *orderDepositRetryQueue) len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.byKey)
}

func (q *orderDepositRetryQueue) clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	clear(q.byKey)
}

func (q *orderDepositRetryQueue) orderQueueSnapshot() orderQueueSnapshot {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var snapshot orderQueueSnapshot
	for _, state := range q.byKey {
		if state.readyAt.IsZero() {
			continue
		}
		snapshot.backlog++
		snapshot.nearestDeadline = earlierOrderDeadlineUnix(snapshot.nearestDeadline, state.order)
	}
	return snapshot
}
