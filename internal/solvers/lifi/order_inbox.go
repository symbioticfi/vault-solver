package lifi

import (
	"context"
	"strings"
	"sync"

	"github.com/go-errors/errors"
)

// orderInbox keeps WebSocket delivery and REST recovery independent from slower on-chain planning.
// The feed and recovery sweep may produce concurrently; run is the only consumer.
type orderInbox struct {
	mu                sync.Mutex
	orders            []*submittedOrder
	queued            map[string]bool
	recoverySeen      map[string]bool
	recoverySeenOrder []string
	recoverySeenNext  int
	recoveryOverflow  bool
	recoveryRetry     map[string]*submittedOrder
	recoveryAttempts  map[string]int
	recoveryGen       uint64
	closed            bool
	capacity          int
	ready             chan struct{}
	space             chan struct{}
}

func newOrderInbox(capacity int) *orderInbox {
	if capacity <= 0 {
		panic("lifi: order inbox capacity must be positive")
	}
	return &orderInbox{
		queued: make(map[string]bool), capacity: capacity,
		ready: make(chan struct{}, 1), space: make(chan struct{}, 1),
	}
}

func (q *orderInbox) enqueue(order *submittedOrder) error {
	if order == nil {
		return nil
	}
	key := orderInboxKey(order)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errOrderInboxClosed
	}
	if key != "" && q.recoverySeen[key] {
		q.mu.Unlock()
		return nil
	}
	if key != "" && q.queued[key] {
		q.markRecoverySeen(key)
		q.mu.Unlock()
		return nil
	}
	if len(q.orders) >= q.capacity {
		if q.recoverySeen != nil {
			q.recoveryOverflow = true
		}
		q.mu.Unlock()
		return errOrderInboxFull
	}
	if order.processed == nil {
		q.recoveryGen++
	} else {
		order.recoveryGen = q.recoveryGen
	}
	q.orders = append(q.orders, order)
	if key != "" {
		q.queued[key] = true
		q.markRecoverySeen(key)
	}
	q.mu.Unlock()
	q.signalReady()
	return nil
}

func (q *orderInbox) closeInput() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.signalReady()
}

func (q *orderInbox) signalReady() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *orderInbox) markRecoverySeen(key string) {
	if key == "" || q.recoverySeen == nil || q.recoverySeen[key] {
		return
	}
	if len(q.recoverySeenOrder) < orderRecoverySeenCapacity {
		q.recoverySeenOrder = append(q.recoverySeenOrder, key)
	} else {
		evicted := q.recoverySeenOrder[q.recoverySeenNext]
		delete(q.recoverySeen, evicted)
		q.recoverySeenOrder[q.recoverySeenNext] = key
		q.recoverySeenNext = (q.recoverySeenNext + 1) % orderRecoverySeenCapacity
	}
	q.recoverySeen[key] = true
}

func (q *orderInbox) enqueueWait(ctx context.Context, order *submittedOrder) error {
	for {
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

func (q *orderInbox) beginRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.clearRecovery()
	q.recoverySeen = make(map[string]bool)
	q.recoveryRetry = make(map[string]*submittedOrder)
	q.recoveryAttempts = make(map[string]int)
}

func (q *orderInbox) endRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.clearRecovery()
}

func (q *orderInbox) clearRecovery() {
	q.recoverySeen = nil
	q.recoverySeenOrder = nil
	q.recoverySeenNext = 0
	q.recoveryOverflow = false
	q.recoveryRetry = nil
	q.recoveryAttempts = nil
	q.recoveryGen = 0
}

func (q *orderInbox) markRecoveryRetry(order *submittedOrder, attemptLimit int) {
	key := orderInboxKey(order)
	if key == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.recoverySeen == nil {
		return
	}
	// A zero limit deliberately preserves unbounded recovery for chain, RPC, and
	// pre-admission failures. Positive limits count failures by stable order key, so
	// the budget survives both recovery sweeps and reconstructed REST order values.
	if attemptLimit > 0 {
		q.recoveryAttempts[key]++
		if q.recoveryAttempts[key] >= attemptLimit {
			return
		}
	}
	q.recoveryRetry[key] = order
	q.recoveryGen++
}

func (q *orderInbox) takeRecoveryRetries() []*submittedOrder {
	q.mu.Lock()
	defer q.mu.Unlock()
	orders := make([]*submittedOrder, 0, len(q.recoveryRetry))
	retrying := make(map[string]bool, len(q.recoveryRetry))
	for key, order := range q.recoveryRetry {
		orders = append(orders, order)
		retrying[key] = true
		delete(q.recoverySeen, key)
	}
	q.recoveryRetry = make(map[string]*submittedOrder)
	seenOrder := make([]string, 0, len(q.recoverySeenOrder))
	for offset := range len(q.recoverySeenOrder) {
		index := (q.recoverySeenNext + offset) % len(q.recoverySeenOrder)
		key := q.recoverySeenOrder[index]
		if key != "" && !retrying[key] && q.recoverySeen[key] {
			seenOrder = append(seenOrder, key)
		}
	}
	q.recoverySeenOrder = seenOrder
	q.recoverySeenNext = 0
	return orders
}

func (q *orderInbox) tryEndRecovery(processedGen uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.recoveryOverflow || len(q.recoveryRetry) > 0 {
		q.recoveryOverflow = false
		return false
	}
	if q.recoveryGen != processedGen {
		return false
	}
	q.clearRecovery()
	return true
}

func (q *orderInbox) run(ctx context.Context, out chan<- *submittedOrder) error {
	defer close(out)
	for {
		q.mu.Lock()
		if len(q.orders) == 0 {
			if q.closed {
				q.mu.Unlock()
				return nil
			}
			q.mu.Unlock()
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
		q.mu.Unlock()
		select {
		case q.space <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- order:
		}
		if key := orderInboxKey(order); key != "" {
			q.mu.Lock()
			delete(q.queued, key)
			q.mu.Unlock()
		}
	}
}

func orderInboxKey(order *submittedOrder) string {
	if order.dedupeKey != "" {
		return order.dedupeKey
	}
	if order.OnChainOrderID != "" {
		return strings.ToLower(strings.TrimSpace(order.OnChainOrderID))
	}
	return strings.ToLower(strings.TrimSpace(order.OrderID))
}
