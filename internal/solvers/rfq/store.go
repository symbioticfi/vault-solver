package rfq

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// orderStatus is the local order lifecycle. queued → submitting → submitted → {filled|expired|failed}.
type orderStatus string

const (
	statusQueued     orderStatus = "queued"
	statusSubmitting orderStatus = "submitting"
	statusSubmitted  orderStatus = "submitted"
	statusFilled     orderStatus = "filled"
	statusExpired    orderStatus = "expired"
	statusFailed     orderStatus = "failed"
)

func (s orderStatus) active() bool {
	return s == statusQueued || s == statusSubmitting || s == statusSubmitted
}

// terminalOrderTTL is how long terminal orders are retained for reconciliation and observability.
const terminalOrderTTL = 3 * time.Hour

// orderRecord is the local tracking state for one order. The executable payload (encodedOrder,
// signature, deadline) is fetched fresh from the backend at fill time, so it is not persisted here.
type orderRecord struct {
	OrderID   string
	Status    orderStatus
	TxHash    common.Hash
	LastError string
	UpdatedAt time.Time
	attempts  int
}

// queuedOrder is the input to upsertQueued, carrying the fields known when an order is first polled.
type queuedOrder struct {
	OrderID string
}

// store is the filler's in-memory operational state. The execution goroutine owns all live access;
// accessors that expose records return clones so callers cannot mutate stored state.
type store struct {
	orders map[string]*orderRecord // by orderId
	now    func() time.Time
}

func newStore(now func() time.Time) *store {
	return &store{orders: make(map[string]*orderRecord), now: now}
}

// sweep evicts terminal orders (with their attempt counts) untouched for longer than terminalOrderTTL.
func (s *store) sweep() {
	now := s.now()
	for id, rec := range s.orders {
		if !rec.Status.active() && now.Sub(rec.UpdatedAt) > terminalOrderTTL {
			delete(s.orders, id)
		}
	}
}

/* ───────── orders ───────── */

// upsertQueued creates a queued order if absent, or refreshes the existing record's poll fields. A
// still-open order that previously failed a fill is re-armed to queued for another attempt (mirrors the
// TS filler, whose status precedence excludes `failed`): upsertQueued is only called for orders the
// backend still lists as open, so a transient failure (e.g. a fill that lost a race) gets retried while
// the order is live, and a deterministic one just re-fails cheaply via the pre-submit guards
// (deadline / strategy-binding / filler checks fail before any tx is sent). Active and terminal states
// (submitting / submitted / filled / expired) are left untouched so we never regress them.
func (s *store) upsertQueued(in queuedOrder) {
	now := s.now()
	rec, ok := s.orders[in.OrderID]
	if !ok {
		rec = &orderRecord{OrderID: in.OrderID, Status: statusQueued}
		s.orders[in.OrderID] = rec
	}
	if rec.Status == statusFailed {
		rec.Status = statusQueued
		rec.LastError = ""
	}
	rec.UpdatedAt = now
}

// activeOrders returns copies of orders in a non-terminal state.
func (s *store) activeOrders() []*orderRecord {
	out := make([]*orderRecord, 0, len(s.orders))
	for _, rec := range s.orders {
		if rec.Status.active() {
			out = append(out, cloneOrder(rec))
		}
	}
	return out
}

// markStatus sets the status and optional txHash/lastError.
func (s *store) markStatus(orderID string, status orderStatus, txHash common.Hash, lastErr string) {
	rec, ok := s.orders[orderID]
	if !ok {
		return
	}
	rec.Status = status
	if txHash != (common.Hash{}) {
		rec.TxHash = txHash
	}
	rec.LastError = lastErr
	rec.UpdatedAt = s.now()
}

// recordAttempt increments and returns the attempt count for an order.
func (s *store) recordAttempt(orderID string) int {
	rec := s.orders[orderID]
	if rec == nil {
		return 0
	}
	rec.attempts++
	return rec.attempts
}

func cloneOrder(rec *orderRecord) *orderRecord {
	if rec == nil {
		return nil
	}
	cp := *rec
	return &cp
}
