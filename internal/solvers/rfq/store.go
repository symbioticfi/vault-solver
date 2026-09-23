package rfq

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

// orderStatus is the local order lifecycle. Confirmed cancellation can enter retry_waiting before
// another open-order poll returns it to queued; other signed failures are terminal.
type orderStatus string

const (
	statusQueued       orderStatus = "queued"
	statusSubmitting   orderStatus = "submitting"
	statusSubmitted    orderStatus = "submitted"
	statusFilled       orderStatus = "filled"
	statusExpired      orderStatus = "expired"
	statusFailed       orderStatus = "failed"
	statusRetryWaiting orderStatus = "retry_waiting"
)

func (s orderStatus) active() bool {
	return s == statusQueued || s == statusSubmitting || s == statusSubmitted || s == statusRetryWaiting
}

// awaitsSubmission reports a won order the submitter still has to send.
func (s orderStatus) awaitsSubmission() bool {
	return s == statusQueued || s == statusSubmitting
}

const (
	// terminalOrderTTL is how long terminal orders (and their attempt counts) are retained for
	// reconciliation/observability before eviction.
	terminalOrderTTL = 3 * time.Hour
)

// orderRecord is the local tracking state for one order. The executable payload is fetched fresh
// from the backend at fill time; only a translated deadline is retained to bound unsigned retries.
type orderRecord struct {
	OrderID             string
	QuoteID             string
	Status              orderStatus
	TxHash              common.Hash
	LastError           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CancellationRetries int
	RetryAt             time.Time
	RetryDeadline       time.Time
	// IncludedAt is the block a confirmed fill landed in; zero until then. A snapshot read at or
	// after it already reflects the fill, so the reservation is not subtracted from it.
	IncludedAt uint64
}

// queuedOrder is the input to upsertQueued, carrying the fields known when an order is first polled.
type queuedOrder struct {
	OrderID string
	QuoteID string
}

// store is the filler's in-memory operational state. The HTTP server and the poll loop touch it
// concurrently, so every accessor is mutex-guarded.
type store struct {
	mu sync.Mutex
	// reservations hold the liquidity of won orders until they leave the active set. Writes happen
	// under mu so a reservation can never outlive its order; quotes read the ledger's own lock.
	reservations liquidlane.CapacityLedger
	orders       map[string]*orderRecord // by orderId
	attempts     map[string]int          // by orderId
	now          func() time.Time
}

func newStore(now func() time.Time) *store {
	return &store{
		orders:   make(map[string]*orderRecord),
		attempts: make(map[string]int),
		now:      now,
	}
}

// sweep evicts stale entries so the in-memory maps don't grow without bound over a long run:
// terminal orders (with their attempt counts) untouched for longer than terminalOrderTTL.
func (s *store) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, rec := range s.orders {
		if !rec.Status.active() && now.Sub(rec.UpdatedAt) > terminalOrderTTL {
			delete(s.orders, id)
			delete(s.attempts, id)
		}
	}
}

/* ───────── orders ───────── */

// upsertQueued re-arms unsigned failures and explicitly scheduled cancellation retries. A retry
// requires both the backoff and another open-order poll. Other signed failures stay terminal.
func (s *store) upsertQueued(in queuedOrder) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	rec, ok := s.orders[in.OrderID]
	inserted := !ok
	if !ok {
		rec = &orderRecord{OrderID: in.OrderID, Status: statusQueued, CreatedAt: now}
		s.orders[in.OrderID] = rec
	}
	if rec.Status == statusFailed && rec.TxHash == (common.Hash{}) {
		rec.Status = statusQueued
		rec.LastError = ""
	}
	if rec.Status == statusRetryWaiting && !now.Before(rec.RetryDeadline) {
		rec.Status = statusExpired
		s.reservations.Delete(in.OrderID)
	}
	if rec.Status == statusRetryWaiting && !now.Before(rec.RetryAt) {
		rec.Status = statusQueued
		rec.TxHash = common.Hash{} // the previous nonce was consumed by a confirmed cancellation
		rec.LastError = ""
		rec.RetryAt = time.Time{}
	}
	rec.QuoteID = parse.OrDefault(in.QuoteID, rec.QuoteID)
	rec.UpdatedAt = now
	return inserted
}

func (s *store) order(orderID string) *orderRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneOrder(s.orders[orderID])
}

// activeOrders returns copies of orders in a non-terminal state.
func (s *store) activeOrders() []*orderRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*orderRecord, 0, len(s.orders))
	for _, rec := range s.orders {
		if rec.Status.active() {
			out = append(out, cloneOrder(rec))
		}
	}
	return out
}

// ordersAwaitingSubmission returns won orders still to be sent, oldest award first.
func (s *store) ordersAwaitingSubmission() []*orderRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*orderRecord, 0, len(s.orders))
	for _, rec := range s.orders {
		if rec.Status.awaitsSubmission() {
			out = append(out, cloneOrder(rec))
		}
	}
	slices.SortFunc(out, func(a, b *orderRecord) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.OrderID, b.OrderID))
	})
	return out
}

func (s *store) activeOrderMetrics() (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	count := 0
	var oldest time.Duration
	for _, rec := range s.orders {
		if !rec.Status.active() {
			continue
		}
		count++
		oldest = max(oldest, now.Sub(rec.CreatedAt))
	}
	return count, oldest
}

// markStatus sets the status and optional txHash/lastError.
func (s *store) markStatus(orderID string, status orderStatus, txHash common.Hash, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.orders[orderID]
	if !ok {
		return
	}
	rec.Status = status
	if !status.active() {
		s.reservations.Delete(orderID)
	}
	if txHash != (common.Hash{}) {
		rec.TxHash = txHash
	}
	rec.LastError = lastErr
	rec.UpdatedAt = s.now()
}

// reserve replaces an active order's reservation. It refuses an order that has already left the
// active set, so a plan finishing after a terminal transition cannot leak capacity.
func (s *store) reserve(orderID string, reservations liquidlane.CapacityReservations) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.orders[orderID]
	if !ok || !rec.Status.active() {
		return false
	}
	return s.reservations.Set(orderID, reservations)
}

// reserved reports whether an order already holds a reservation.
func (s *store) reserved(orderID string) bool {
	return s.reservations.Has(orderID)
}

// pendingReservations is the liquidity won, unfinished orders hold against inventory, without one
// order so it can plan its own replacement. A reservation is skipped for a capacity whose every
// snapshot was read at or after the order's confirmed inclusion, since that snapshot already reflects
// the fill; an unknown snapshot block keeps the subtraction.
func (s *store) pendingReservations(excludedOrderID string, inventory []solverInventory) liquidlane.CapacityReservations {
	blocks := snapshotBlocks(inventory)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reservations.SnapshotIf(func(orderID string, capacityID liquidlane.CapacityID) bool {
		if orderID == excludedOrderID {
			return false
		}
		rec := s.orders[orderID]
		if rec == nil || rec.IncludedAt == 0 {
			return true
		}
		block, known := blocks[capacityID]
		return !known || block == 0 || block < rec.IncludedAt
	})
}

// snapshotBlocks is the oldest reported snapshot block per capacity; zero when any item's is unknown.
func snapshotBlocks(inventory []solverInventory) map[liquidlane.CapacityID]uint64 {
	blocks := make(map[liquidlane.CapacityID]uint64, len(inventory))
	for _, item := range inventory {
		capacityID := liquidlane.RouteCapacityID(item.Route)
		block, seen := blocks[capacityID]
		switch {
		case !seen:
			blocks[capacityID] = item.BlockNumber
		case block == 0:
			// unknown stays unknown
		case item.BlockNumber == 0 || item.BlockNumber < block:
			blocks[capacityID] = item.BlockNumber
		}
	}
	return blocks
}

// markIncluded records the block a confirmed fill landed in.
func (s *store) markIncluded(orderID string, block uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec := s.orders[orderID]; rec != nil {
		rec.IncludedAt = block
	}
}

// boundUnsignedWork sets the deadline after which unsigned preparation of an order expires locally.
// A bound already recorded (from a cancellation retry) is kept.
func (s *store) boundUnsignedWork(orderID string, deadline time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec := s.orders[orderID]; rec != nil && rec.RetryDeadline.IsZero() {
		rec.RetryDeadline = deadline
	}
}

// recordAttempt increments and returns the attempt count for an order.
func (s *store) recordAttempt(orderID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[orderID]++
	return s.attempts[orderID]
}

// scheduleCancellationRetry is called only after a successful, confirmed cancellation receipt.
// The consumed nonce is safe to leave behind, but the retry budget survives re-queuing the order.
func (s *store) scheduleCancellationRetry(
	orderID string, limit int, retryAt, deadline time.Time, txHash common.Hash, lastErr string,
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.orders[orderID]
	if rec == nil || rec.CancellationRetries >= limit {
		return false
	}
	rec.CancellationRetries++
	rec.Status = statusRetryWaiting
	rec.TxHash = txHash
	rec.LastError = lastErr
	rec.UpdatedAt = s.now()
	rec.RetryAt = retryAt
	rec.RetryDeadline = deadline
	return true
}

func cloneOrder(rec *orderRecord) *orderRecord {
	if rec == nil {
		return nil
	}
	cp := *rec
	return &cp
}
