package rfq

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type orderStatus string

const (
	statusQueued     orderStatus = "queued"
	statusSubmitting orderStatus = "submitting"
	statusSubmitted  orderStatus = "submitted"
	statusFilled     orderStatus = "filled"
	statusExpired    orderStatus = "expired"
	statusFailed     orderStatus = "failed"
	terminalOrderTTL             = 3 * time.Hour
)

func (s orderStatus) active() bool {
	return s == statusQueued || s == statusSubmitting || s == statusSubmitted
}

// The poll loop owns order transitions. Metrics readers receive value snapshots;
// no executable or signed payload survives a poll cycle.
type orderRecord struct {
	OrderID   string
	Status    orderStatus
	TxHash    common.Hash
	LastError string
	Attempts  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type store struct {
	mu     sync.Mutex
	orders map[string]orderRecord
	now    func() time.Time
}

func newStore(now func() time.Time) *store {
	return &store{orders: make(map[string]orderRecord), now: now}
}

func (s *store) upsertQueued(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, exists := s.orders[id]
	if !exists {
		rec = orderRecord{OrderID: id, Status: statusQueued, CreatedAt: s.now()}
	}
	// Only a fresh open-order listing authorizes retry after failure. Included
	// orders remain submitted until backend reconciliation establishes a terminal state.
	if rec.Status == statusFailed {
		rec.Status, rec.LastError = statusQueued, ""
	}
	rec.UpdatedAt = s.now()
	s.orders[id] = rec
	return !exists
}

func (s *store) activeOrders() []orderRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]orderRecord, 0, len(s.orders))
	for _, rec := range s.orders {
		if rec.Status.active() {
			out = append(out, rec)
		}
	}
	// A map's random order must not decide who gets the serialized transaction lane.
	slices.SortFunc(out, func(a, b orderRecord) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.OrderID, b.OrderID)
	})
	return out
}

func (s *store) activeOrderMetrics() (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count, oldest := 0, time.Duration(0)
	now := s.now()
	for _, rec := range s.orders {
		if rec.Status.active() {
			count++
			oldest = max(oldest, now.Sub(rec.CreatedAt))
		}
	}
	return count, oldest
}

func (s *store) markStatus(id string, status orderStatus, hash common.Hash, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.orders[id]
	if !ok {
		return
	}
	rec.Status, rec.LastError, rec.UpdatedAt = status, message, s.now()
	if hash != (common.Hash{}) {
		rec.TxHash = hash
	}
	s.orders[id] = rec
}

func (s *store) recordAttempt(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.orders[id]
	if !ok {
		return 0
	}
	rec.Attempts++
	s.orders[id] = rec
	return rec.Attempts
}

func (s *store) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, rec := range s.orders {
		if !rec.Status.active() && now.Sub(rec.UpdatedAt) > terminalOrderTTL {
			delete(s.orders, id)
		}
	}
}
