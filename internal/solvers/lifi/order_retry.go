package lifi

import "sync/atomic"

type reservationRetry struct {
	order      *submittedOrder
	generation uint64
}

// reservationRetryQueue is mutated exclusively by the order worker; metrics may
// read an immutable published observation concurrently.
type reservationRetryQueue struct {
	observed atomic.Pointer[orderQueueSnapshot]
	items    []reservationRetry
	capacity int
}

func newReservationRetryQueue(capacity int) *reservationRetryQueue {
	if capacity <= 0 {
		panic("lifi: order retry capacity must be positive")
	}
	return &reservationRetryQueue{capacity: capacity}
}

func (q *reservationRetryQueue) enqueue(order *submittedOrder, generation uint64) error {
	defer q.publish()

	key := orderInboxKey(order)
	if key != "" {
		for _, item := range q.items {
			if orderInboxKey(item.order) == key {
				return nil
			}
		}
	}
	if len(q.items) >= q.capacity {
		return errOrderRetryFull
	}
	q.items = append(q.items, reservationRetry{order: order, generation: generation})
	return nil
}

func (q *reservationRetryQueue) popReady(generation uint64) *submittedOrder {
	defer q.publish()

	if len(q.items) == 0 || q.items[0].generation >= generation {
		return nil
	}
	item := q.items[0]
	q.items[0] = reservationRetry{}
	q.items = q.items[1:]
	if len(q.items) == 0 {
		q.items = nil
	}
	return item.order
}

func (q *reservationRetryQueue) len() int {
	return len(q.items)
}

func (q *reservationRetryQueue) clear() {
	defer q.publish()

	q.items = nil
}

func (q *reservationRetryQueue) orderQueueSnapshot() orderQueueSnapshot {
	if observed := q.observed.Load(); observed != nil {
		return *observed
	}
	return orderQueueSnapshot{}
}

func (q *reservationRetryQueue) publish() {
	state := &orderQueueSnapshot{backlog: len(q.items)}
	for _, item := range q.items {
		state.nearestDeadline = earlierOrderDeadlineUnix(state.nearestDeadline, item.order)
	}
	q.observed.Store(state)
}
