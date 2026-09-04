package lifi

import (
	"context"
	"strings"
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
	errOrderRetryKey            = errors.New("order capacity retry requires a stable order key")
)

type orderPhase uint8

const (
	orderIdle orderPhase = iota
	orderQueued
	orderRecoveryRetry
	orderDepositWait
	orderCapacityWait
)

// orderRecord is the sole mutable lifecycle for one stable LI.FI order identity. phase is exclusive:
// an order cannot simultaneously be queued, recovering, waiting for deposit, and waiting for capacity.
type orderRecord struct {
	order *submittedOrder

	phase            orderPhase
	recoveryAttempts int

	depositStarted time.Time
	depositReady   time.Time
	depositBackoff time.Duration
}

// orderBook owns delivery deduplication, recovery, deposit propagation, and capacity waiting.
// Concurrent feed/recovery producers and the single worker access it only through these methods.
type orderBook struct {
	mu sync.Mutex

	orders  chan *submittedOrder
	records map[string]*orderRecord
	space   chan struct{}

	capacityOrder []string
	recovering    bool
	closed        bool
}

func newOrderBook(capacity int) *orderBook {
	if capacity <= 0 {
		panic("lifi: order book capacity must be positive")
	}
	return &orderBook{
		orders: make(chan *submittedOrder, capacity), records: make(map[string]*orderRecord),
		space: make(chan struct{}, 1),
	}
}

func (b *orderBook) enqueue(order *submittedOrder) error {
	if order == nil {
		return nil
	}
	key := orderKey(order)
	b.mu.Lock()
	switch {
	case b.closed:
		b.mu.Unlock()
		return errOrderBookClosed
	case key != "" && b.records[key] != nil && b.records[key].phase != orderIdle:
		b.mu.Unlock()
		return nil
	case len(b.orders) == cap(b.orders):
		b.mu.Unlock()
		return errOrderBookFull
	}
	if key != "" {
		record := b.recordLocked(key, order)
		record.phase = orderQueued
	}
	b.orders <- order
	b.mu.Unlock()
	return nil
}

func (b *orderBook) enqueueWait(ctx context.Context, order *submittedOrder) error {
	for {
		if err := b.enqueue(order); !errors.Is(err, errOrderBookFull) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.space:
		}
	}
}

func (b *orderBook) waitUntilProcessed(ctx context.Context) error {
	barrier := &submittedOrder{processed: make(chan struct{})}
	if err := b.enqueueWait(ctx, barrier); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-barrier.processed:
		return nil
	}
}

func (b *orderBook) accepted(order *submittedOrder) {
	b.mu.Lock()
	if key := orderKey(order); key != "" {
		if record := b.records[key]; record != nil && record.phase == orderQueued {
			record.phase = orderIdle
			b.removeIdleLocked(key, record)
		}
	}
	b.mu.Unlock()
	b.signalSpace()
}

func (b *orderBook) closeInput() {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.orders)
	}
	b.mu.Unlock()
}

func (b *orderBook) beginRecovery() {
	b.mu.Lock()
	b.recovering = true
	for key, record := range b.records {
		if record.phase == orderRecoveryRetry {
			record.phase = orderIdle
		}
		record.recoveryAttempts = 0
		b.removeIdleLocked(key, record)
	}
	b.mu.Unlock()
}

func (b *orderBook) endRecovery() {
	b.mu.Lock()
	b.recovering = false
	for key, record := range b.records {
		if record.phase == orderRecoveryRetry {
			record.phase = orderIdle
		}
		record.recoveryAttempts = 0
		b.removeIdleLocked(key, record)
	}
	b.mu.Unlock()
}

func (b *orderBook) markRecoveryRetry(order *submittedOrder, limit int) {
	key := orderKey(order)
	if key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.recovering {
		return
	}
	record := b.recordLocked(key, order)
	record.recoveryAttempts++
	if limit == 0 || record.recoveryAttempts < limit {
		record.phase = orderRecoveryRetry
	}
	b.removeIdleLocked(key, record)
}

func (b *orderBook) takeRecoveryRetries() []*submittedOrder {
	b.mu.Lock()
	defer b.mu.Unlock()
	orders := make([]*submittedOrder, 0)
	for key, record := range b.records {
		if record.phase != orderRecoveryRetry {
			continue
		}
		record.phase = orderIdle
		orders = append(orders, record.order)
		b.removeIdleLocked(key, record)
	}
	return orders
}

func (b *orderBook) tryEndRecovery() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, record := range b.records {
		if record.phase == orderRecoveryRetry {
			return false
		}
	}
	b.recovering = false
	for key, record := range b.records {
		record.recoveryAttempts = 0
		b.removeIdleLocked(key, record)
	}
	return true
}

func (b *orderBook) containsDeposit(order *submittedOrder) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	record := b.records[orderKey(order)]
	return record != nil && record.phase == orderDepositWait
}

func (b *orderBook) scheduleDeposit(order *submittedOrder, now time.Time) error {
	key := orderKey(order)
	if key == "" {
		return errOrderDepositRetryKey
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	record := b.recordLocked(key, order)
	if record.phase == orderDepositWait {
		return nil
	}
	if record.phase != orderIdle {
		return nil
	}
	if record.depositStarted.IsZero() {
		if b.countPhaseLocked(orderDepositWait) >= orderDepositRetryCapacity {
			b.removeIdleLocked(key, record)
			return errOrderDepositRetryFull
		}
		record.depositStarted = now
		record.phase = orderDepositWait
	}
	end, boundErr := orderDepositRetryEnd(order, record.depositStarted)
	lastTry := end.Add(-initialOrderDepositRetryBackoff)
	if !now.Before(lastTry) {
		b.finishDepositLocked(key, record)
		return boundErr
	}
	if record.depositBackoff == 0 {
		record.depositBackoff = initialOrderDepositRetryBackoff
	} else {
		record.depositBackoff = min(2*record.depositBackoff, maximumOrderDepositRetryBackoff)
	}
	record.depositReady = minTime(now.Add(record.depositBackoff), lastTry)
	return nil
}

func orderDepositRetryEnd(order *submittedOrder, started time.Time) (time.Time, error) {
	end := started.Add(maximumOrderDepositRetryWindow)
	if deadline := orderDeadline(order); !deadline.IsZero() && !deadline.After(end) {
		return deadline, errOrderDepositRetryExpired
	}
	return end, errOrderDepositRetryWindow
}

func (b *orderBook) popDepositReady(now time.Time) (*submittedOrder, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key, record := b.nextDepositLocked()
	if record == nil || record.depositReady.After(now) {
		return nil, nil
	}
	record.depositReady = time.Time{}
	end, boundErr := orderDepositRetryEnd(record.order, record.depositStarted)
	if !now.Before(end) {
		order := record.order
		b.finishDepositLocked(key, record)
		return order, boundErr
	}
	return record.order, nil
}

func (b *orderBook) nextDepositLocked() (string, *orderRecord) {
	var nextKey string
	var next *orderRecord
	for key, record := range b.records {
		if record.phase != orderDepositWait || record.depositReady.IsZero() {
			continue
		}
		if next == nil || record.depositReady.Before(next.depositReady) ||
			record.depositReady.Equal(next.depositReady) && key < nextKey {
			nextKey, next = key, record
		}
	}
	return nextKey, next
}

func (b *orderBook) finishDeposit(order *submittedOrder) {
	key := orderKey(order)
	b.mu.Lock()
	if record := b.records[key]; record != nil {
		b.finishDepositLocked(key, record)
	}
	b.mu.Unlock()
}

func (b *orderBook) finishDepositLocked(key string, record *orderRecord) {
	if record.phase != orderDepositWait || record.depositStarted.IsZero() {
		return
	}
	record.depositStarted, record.depositReady, record.depositBackoff = time.Time{}, time.Time{}, 0
	record.phase = orderIdle
	b.removeIdleLocked(key, record)
}

func (b *orderBook) enqueueCapacity(order *submittedOrder) (bool, error) {
	key := orderKey(order)
	if key == "" {
		return false, errOrderRetryKey
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	record := b.recordLocked(key, order)
	if record.phase == orderCapacityWait {
		return false, nil
	}
	if record.phase != orderIdle {
		return false, nil
	}
	if b.countPhaseLocked(orderCapacityWait) >= orderRetryCapacity {
		b.removeIdleLocked(key, record)
		return false, errOrderRetryFull
	}
	record.phase = orderCapacityWait
	b.capacityOrder = append(b.capacityOrder, key)
	return true, nil
}

func (b *orderBook) popCapacity() *submittedOrder {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.capacityOrder) > 0 {
		key := b.capacityOrder[0]
		b.capacityOrder = b.capacityOrder[1:]
		record := b.records[key]
		if record == nil || record.phase != orderCapacityWait {
			continue
		}
		record.phase = orderIdle
		order := record.order
		b.removeIdleLocked(key, record)
		return order
	}
	return nil
}

func (b *orderBook) retryCounts() (deposit, capacity int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.countPhaseLocked(orderDepositWait), b.countPhaseLocked(orderCapacityWait)
}

func (b *orderBook) metricsSnapshot() orderQueueMetrics {
	b.mu.Lock()
	defer b.mu.Unlock()
	snapshot := orderQueueMetrics{queueInbox: {count: len(b.orders)}}
	for _, record := range b.records {
		var value *orderQueueMetric
		switch record.phase {
		case orderIdle:
		case orderQueued:
			value = &snapshot[queueInbox]
		case orderRecoveryRetry:
			value = &snapshot[queueRecoveryRetry]
		case orderCapacityWait:
			value = &snapshot[queueCapacityRetry]
		case orderDepositWait:
			value = &snapshot[queueDepositRetry]
		}
		if value == nil {
			continue
		}
		if record.phase != orderQueued {
			value.count++
		}
		if deadline := orderDeadline(record.order).Unix(); deadline > 0 &&
			(value.deadline == 0 || deadline < value.deadline) {
			value.deadline = deadline
		}
	}
	return snapshot
}

func (b *orderBook) clear() {
	b.mu.Lock()
	b.records = make(map[string]*orderRecord)
	b.capacityOrder = nil
	b.mu.Unlock()
}

func (b *orderBook) countPhaseLocked(phase orderPhase) int {
	count := 0
	for _, record := range b.records {
		if record.phase == phase {
			count++
		}
	}
	return count
}

func (b *orderBook) recordLocked(key string, order *submittedOrder) *orderRecord {
	record := b.records[key]
	if record == nil {
		record = new(orderRecord)
		b.records[key] = record
	}
	record.order = order
	return record
}

func (b *orderBook) removeIdleLocked(key string, record *orderRecord) {
	// Recovery attempt budgets must survive REST sweeps and reconstructed order values.
	if b.recovering && record.recoveryAttempts > 0 {
		return
	}
	if record.phase == orderIdle && record.depositStarted.IsZero() {
		delete(b.records, key)
	}
}

func (b *orderBook) signalSpace() {
	select {
	case b.space <- struct{}{}:
	default:
	}
}

func orderKey(order *submittedOrder) string {
	if order == nil {
		return ""
	}
	if order.dedupeKey != "" {
		return order.dedupeKey
	}
	if order.OnChainOrderID != "" {
		return strings.ToLower(strings.TrimSpace(order.OnChainOrderID))
	}
	return strings.ToLower(strings.TrimSpace(order.OrderID))
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
