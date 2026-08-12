package lifi

import (
	"sort"
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
	order          *submittedOrder
	key            string
	backoffStep    int
	startedAt      time.Time
	readyAt        time.Time
	endAt          time.Time
	endErr         error
	finalScheduled bool
}

// orderDepositRetryQueue is owned exclusively by the order worker. A state remains
// indexed while its order is being retried, so a concurrent feed replay cannot reset
// the backoff/window state between status reads.
type orderDepositRetryQueue struct {
	items    []*orderDepositRetry
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
		state = &orderDepositRetry{key: key, startedAt: now}
		q.byKey[key] = state
	}
	state.order = order
	if state.finalScheduled {
		delete(q.byKey, key)
		return state.endErr
	}

	retryEnd, boundErr := orderDepositRetryEnd(order, state.startedAt)
	state.endAt = retryEnd
	state.endErr = boundErr
	if !now.Before(retryEnd) {
		delete(q.byKey, key)
		return boundErr
	}
	finalReadyAt := retryEnd.Add(-initialOrderDepositRetryBackoff)
	if !now.Before(finalReadyAt) {
		delete(q.byKey, key)
		return boundErr
	}

	state.backoffStep++
	readyAt := now.Add(orderDepositRetryBackoff(state.backoffStep))
	if !readyAt.Before(finalReadyAt) {
		readyAt = finalReadyAt
		state.finalScheduled = true
	}
	state.readyAt = readyAt
	index := sort.Search(len(q.items), func(index int) bool {
		return !q.items[index].readyAt.Before(readyAt)
	})
	q.items = append(q.items, nil)
	copy(q.items[index+1:], q.items[index:])
	q.items[index] = state
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

func orderDepositRetryBackoff(step int) time.Duration {
	backoff := initialOrderDepositRetryBackoff
	for range max(0, step-1) {
		backoff = min(2*backoff, maximumOrderDepositRetryBackoff)
	}
	return backoff
}

func orderDepositRetryDelay(readyAt, now time.Time) time.Duration {
	return max(readyAt.Sub(now), 0)
}

func (q *orderDepositRetryQueue) contains(order *submittedOrder) bool {
	_, ok := q.byKey[orderInboxKey(order)]
	return ok
}

func (q *orderDepositRetryQueue) nextReadyAt() (time.Time, bool) {
	if len(q.items) == 0 {
		return time.Time{}, false
	}
	return q.items[0].readyAt, true
}

func (q *orderDepositRetryQueue) popReady(now time.Time) (*submittedOrder, error) {
	if len(q.items) == 0 || q.items[0].readyAt.After(now) {
		return nil, nil
	}
	state := q.items[0]
	q.items[0] = nil
	q.items = q.items[1:]
	if len(q.items) == 0 {
		q.items = nil
	}
	state.readyAt = time.Time{}
	if !now.Before(state.endAt) {
		delete(q.byKey, state.key)
		return state.order, state.endErr
	}
	return state.order, nil
}

func (q *orderDepositRetryQueue) finish(order *submittedOrder) {
	key := orderInboxKey(order)
	state := q.byKey[key]
	if state == nil {
		return
	}
	if !state.readyAt.IsZero() {
		for index, item := range q.items {
			if item != state {
				continue
			}
			copy(q.items[index:], q.items[index+1:])
			q.items[len(q.items)-1] = nil
			q.items = q.items[:len(q.items)-1]
			break
		}
	}
	delete(q.byKey, key)
}

func (q *orderDepositRetryQueue) len() int {
	return len(q.items)
}

func (q *orderDepositRetryQueue) clear() {
	q.items = nil
	clear(q.byKey)
}
