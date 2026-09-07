package lifi

import (
	"container/list"
	"context"
	"strings"
	"sync"

	"github.com/go-errors/errors"
)

// A key has one record across WebSocket delivery, REST recovery and retries.
// All fields are guarded by orderInbox.mu; only run sends to the worker.
type inboxEntry struct {
	queued   bool
	seen     *list.Element
	retry    *submittedOrder
	attempts int
}

type orderInbox struct {
	mu               sync.Mutex
	orders           []*submittedOrder
	delivering       *submittedOrder
	entries          map[string]*inboxEntry
	seen             list.List
	recovering       bool
	recoveryOverflow bool
	recoveryGen      uint64
	closed           bool
	capacity         int
	ready            chan struct{}
	space            chan struct{}
}

func newOrderInbox(capacity int) *orderInbox {
	if capacity <= 0 {
		panic("lifi: order inbox capacity must be positive")
	}
	return &orderInbox{capacity: capacity, entries: make(map[string]*inboxEntry),
		ready: make(chan struct{}, 1), space: make(chan struct{}, 1)}
}

func signal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (q *orderInbox) entry(key string) *inboxEntry {
	state := q.entries[key]
	if state == nil {
		state = &inboxEntry{}
		q.entries[key] = state
	}
	return state
}

func (q *orderInbox) forgetIdle(key string) {
	state := q.entries[key]
	if state != nil && !state.queued && state.seen == nil && state.retry == nil && state.attempts == 0 {
		delete(q.entries, key)
	}
}

func (q *orderInbox) markRecoverySeen(key string) {
	if key == "" || !q.recovering {
		return
	}
	state := q.entry(key)
	if state.seen != nil {
		return
	}
	if q.seen.Len() == orderRecoverySeenCapacity {
		oldest := q.seen.Front()
		oldKey := oldest.Value.(string)
		q.entries[oldKey].seen = nil
		q.seen.Remove(oldest)
		q.forgetIdle(oldKey)
	}
	state.seen = q.seen.PushBack(key)
}

func (q *orderInbox) enqueue(order *submittedOrder) error {
	if order == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errOrderInboxClosed
	}
	key := orderInboxKey(order)
	if state := q.entries[key]; key != "" && state != nil {
		if state.seen != nil {
			return nil
		}
		if state.queued {
			q.markRecoverySeen(key)
			return nil
		}
	}
	if len(q.orders) >= q.capacity {
		q.recoveryOverflow = q.recoveryOverflow || q.recovering
		return errOrderInboxFull
	}
	if order.processed == nil {
		q.recoveryGen++
	} else {
		order.recoveryGen = q.recoveryGen
	}
	q.orders = append(q.orders, order)
	if key != "" {
		q.entry(key).queued = true
		q.markRecoverySeen(key)
	}
	signal(q.ready)
	return nil
}

func (q *orderInbox) enqueueWait(ctx context.Context, order *submittedOrder) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := q.enqueue(order)
		if !errors.Is(err, errOrderInboxFull) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.space:
		}
	}
}

func (q *orderInbox) closeInput() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	signal(q.ready)
	signal(q.space)
}

func (q *orderInbox) waitUntilProcessed(ctx context.Context) (uint64, error) {
	barrier := &submittedOrder{processed: make(chan struct{})}
	if err := q.enqueueWait(ctx, barrier); err != nil {
		return 0, err
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-barrier.processed:
		return barrier.recoveryGen, nil
	}
}

func (q *orderInbox) resetRecovery(active bool) {
	q.recovering, q.recoveryOverflow, q.recoveryGen = active, false, 0
	q.seen.Init()
	for key, state := range q.entries {
		state.seen, state.retry, state.attempts = nil, nil, 0
		q.forgetIdle(key)
	}
}

func (q *orderInbox) beginRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.resetRecovery(true)
}

func (q *orderInbox) endRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.resetRecovery(false)
}

func (q *orderInbox) markRecoveryRetry(order *submittedOrder, limit int) {
	key := orderInboxKey(order)
	if key == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.recovering {
		return
	}
	state := q.entry(key)
	// Zero is reserved for transport/pre-admission failures: these must not consume
	// the bounded strategy failure budget while delivery readiness is withheld.
	if limit > 0 {
		state.attempts++
		if state.attempts >= limit {
			return
		}
	}
	state.retry = order
	q.recoveryGen++
}

func (q *orderInbox) takeRecoveryRetries() []*submittedOrder {
	q.mu.Lock()
	defer q.mu.Unlock()
	var orders []*submittedOrder
	for key, state := range q.entries {
		if state.retry == nil {
			continue
		}
		orders = append(orders, state.retry)
		state.retry = nil
		if state.seen != nil {
			q.seen.Remove(state.seen)
			state.seen = nil
		}
		q.forgetIdle(key)
	}
	return orders
}

func (q *orderInbox) tryEndRecovery(generation uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	overflow := q.recoveryOverflow
	q.recoveryOverflow = false
	if overflow || q.recoveryGen != generation {
		return false
	}
	for _, state := range q.entries {
		if state.retry != nil {
			return false
		}
	}
	q.resetRecovery(false)
	return true
}

func (q *orderInbox) run(ctx context.Context, out chan<- *submittedOrder) error {
	defer close(out)
	for {
		q.mu.Lock()
		if len(q.orders) == 0 {
			closed := q.closed
			q.mu.Unlock()
			if closed {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-q.ready:
				continue
			}
		}
		order := q.orders[0]
		q.orders[0] = nil
		q.orders = q.orders[1:]
		if len(q.orders) == 0 {
			q.orders = nil
		}
		q.delivering = order
		q.mu.Unlock()
		signal(q.space)
		var err error
		select {
		case out <- order:
		case <-ctx.Done():
			err = ctx.Err()
		}
		q.mu.Lock()
		q.delivering = nil
		if key := orderInboxKey(order); key != "" {
			q.entry(key).queued = false
			q.forgetIdle(key)
		}
		q.mu.Unlock()
		if err != nil {
			return err
		}
	}
}

func addQueuedOrder(snapshot *orderQueueSnapshot, order *submittedOrder) {
	if order == nil || order.processed != nil {
		return
	}
	snapshot.backlog++
	snapshot.nearestDeadline = earlierOrderDeadlineUnix(snapshot.nearestDeadline, order)
}

func (q *orderInbox) orderQueueSnapshot() orderQueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	var snapshot orderQueueSnapshot
	addQueuedOrder(&snapshot, q.delivering)
	for _, order := range q.orders {
		addQueuedOrder(&snapshot, order)
	}
	return snapshot
}

func (q *orderInbox) recoveryRetryQueueSnapshot() orderQueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	var snapshot orderQueueSnapshot
	for _, state := range q.entries {
		addQueuedOrder(&snapshot, state.retry)
	}
	return snapshot
}

func orderInboxKey(order *submittedOrder) string {
	if order == nil {
		return ""
	}
	if order.dedupeKey != "" {
		return order.dedupeKey
	}
	key := order.OnChainOrderID
	if key == "" {
		key = order.OrderID
	}
	return strings.ToLower(strings.TrimSpace(key))
}
