package lifi

import "sync"

type reservationRetry struct {
	order      *submittedOrder
	generation uint64
}

// reservationRetryQueue is mutated exclusively by the order worker; metrics may
// take read-only snapshots concurrently.
type reservationRetryQueue struct {
	mu       sync.RWMutex
	items    []reservationRetry
	queued   map[string]bool
	capacity int
}

func newReservationRetryQueue(capacity int) *reservationRetryQueue {
	if capacity <= 0 {
		panic("lifi: order retry capacity must be positive")
	}
	return &reservationRetryQueue{queued: make(map[string]bool), capacity: capacity}
}

func (q *reservationRetryQueue) enqueue(order *submittedOrder, generation uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := orderInboxKey(order)
	if key != "" && q.queued[key] {
		return nil
	}
	if len(q.items) >= q.capacity {
		return errOrderRetryFull
	}
	q.items = append(q.items, reservationRetry{order: order, generation: generation})
	if key != "" {
		q.queued[key] = true
	}
	return nil
}

func (q *reservationRetryQueue) popReady(generation uint64) *submittedOrder {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 || q.items[0].generation >= generation {
		return nil
	}
	item := q.items[0]
	q.items[0] = reservationRetry{}
	q.items = q.items[1:]
	if len(q.items) == 0 {
		q.items = nil
	}
	delete(q.queued, orderInboxKey(item.order))
	return item.order
}

func (q *reservationRetryQueue) len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.items)
}

func (q *reservationRetryQueue) clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = nil
	clear(q.queued)
}

func (q *reservationRetryQueue) orderQueueSnapshot() orderQueueSnapshot {
	q.mu.RLock()
	defer q.mu.RUnlock()

	snapshot := orderQueueSnapshot{backlog: len(q.items)}
	for _, item := range q.items {
		snapshot.nearestDeadline = earlierOrderDeadlineUnix(snapshot.nearestDeadline, item.order)
	}
	return snapshot
}
