package rfq

import (
	"sync"
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

const (
	// strategyTTL bounds how long a quoted strategy is cached before eviction. A later award that
	// misses the cache is rebuilt from on-chain state by recoverStrategy, so this only caps memory;
	// it does not drop fillable orders.
	strategyTTL = 3 * time.Hour
	// terminalOrderTTL is how long terminal orders (and their attempt counts) are retained for
	// reconciliation/observability before eviction.
	terminalOrderTTL = 3 * time.Hour
)

// orderRecord is the local tracking state for one order. The executable payload (encodedOrder,
// signature, deadline) is fetched fresh from the backend at fill time, so it is not persisted here.
type orderRecord struct {
	OrderID   string
	QuoteID   string
	Status    orderStatus
	TxHash    common.Hash
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// queuedOrder is the input to upsertQueued, carrying the fields known when an order is first polled.
type queuedOrder struct {
	OrderID string
	QuoteID string
}

// store is the filler's in-memory operational state. The HTTP server and the poll loop touch it
// concurrently, so every accessor is mutex-guarded.
type store struct {
	mu         sync.Mutex
	strategies map[string]*strategyRecord // by quoteId
	orders     map[string]*orderRecord    // by orderId
	attempts   map[string]int             // by orderId
	now        func() time.Time
}

func newStore(now func() time.Time) *store {
	return &store{
		strategies: make(map[string]*strategyRecord),
		orders:     make(map[string]*orderRecord),
		attempts:   make(map[string]int),
		now:        now,
	}
}

/* ───────── strategies ───────── */

func (s *store) putStrategy(rec *strategyRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategies[rec.QuoteID] = rec
}

// strategy returns the cached strategy for quoteID, or nil. It returns the shared pointer (not a
// clone): a strategyRecord is immutable after putStrategy, so concurrent readers are safe. Do not
// mutate a returned record in place — copy it, or that invariant (and the lack of a data race) breaks.
func (s *store) strategy(quoteID string) *strategyRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.strategies[quoteID]
}

// sweep evicts stale entries so the in-memory maps don't grow without bound over a long run:
// strategies older than strategyTTL, and terminal orders (with their attempt counts) untouched for
// longer than terminalOrderTTL. Called from the poll loop.
func (s *store) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, rec := range s.strategies {
		if now.Sub(rec.CreatedAt) > strategyTTL {
			delete(s.strategies, id)
		}
	}
	for id, rec := range s.orders {
		if !rec.Status.active() && now.Sub(rec.UpdatedAt) > terminalOrderTTL {
			delete(s.orders, id)
			delete(s.attempts, id)
		}
	}
}

/* ───────── orders ───────── */

// upsertQueued creates a queued order if absent, or refreshes the existing record's poll fields
// without regressing a non-queued status.
func (s *store) upsertQueued(in queuedOrder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	rec, ok := s.orders[in.OrderID]
	if !ok {
		rec = &orderRecord{OrderID: in.OrderID, Status: statusQueued, CreatedAt: now}
		s.orders[in.OrderID] = rec
	}
	rec.QuoteID = orStr(in.QuoteID, rec.QuoteID)
	rec.UpdatedAt = now
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

// markStatus sets the status and optional txHash/lastError.
func (s *store) markStatus(orderID string, status orderStatus, txHash common.Hash, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[orderID]++
	return s.attempts[orderID]
}

func cloneOrder(rec *orderRecord) *orderRecord {
	if rec == nil {
		return nil
	}
	cp := *rec
	return &cp
}
