package rfq

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type orderStatus string

const (
	statusQueued     orderStatus = "queued"
	statusSubmitted  orderStatus = "submitted"
	statusFilled     orderStatus = "filled"
	statusExpired    orderStatus = "expired"
	statusFailed     orderStatus = "failed"
	terminalOrderTTL             = 3 * time.Hour
)

func (status orderStatus) active() bool {
	return status == statusQueued || status == statusSubmitted
}

func canTransition(from, to orderStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case "":
		return to == statusQueued
	case statusQueued:
		return to == statusSubmitted || to == statusFilled || to == statusExpired || to == statusFailed
	case statusSubmitted:
		return to == statusFilled || to == statusExpired || to == statusFailed
	case statusFailed:
		return to == statusQueued
	case statusFilled, statusExpired:
		return false
	default:
		return false
	}
}

type orderRecord struct {
	OrderID   string
	Status    orderStatus
	TxHash    common.Hash
	LastError string
	UpdatedAt time.Time
	attempts  int
}

// store is owned exclusively by the execution loop; it deliberately has no locks or observer API.
type store struct {
	orders map[string]*orderRecord
	now    func() time.Time
}

func newStore(now func() time.Time) *store {
	return &store{orders: make(map[string]*orderRecord), now: now}
}

func (s *store) upsertQueued(id string) bool {
	record := s.orders[id]
	created := record == nil
	if record == nil {
		record = &orderRecord{OrderID: id, Status: statusQueued}
		s.orders[id] = record
	} else if record.Status == statusFailed {
		record.Status, record.LastError = statusQueued, ""
	}
	record.UpdatedAt = s.now()
	return created
}

func (s *store) activeOrders() []*orderRecord {
	active := make([]*orderRecord, 0, len(s.orders))
	for _, record := range s.orders {
		if record.Status.active() {
			active = append(active, record)
		}
	}
	return active
}

func (s *store) markStatus(id string, status orderStatus, hash common.Hash, message string) bool {
	record := s.orders[id]
	if record == nil || !canTransition(record.Status, status) {
		return false
	}
	record.Status, record.LastError, record.UpdatedAt = status, message, s.now()
	if hash != (common.Hash{}) {
		record.TxHash = hash
	}
	return true
}

func (s *store) recordAttempt(id string) int {
	if record := s.orders[id]; record != nil {
		record.attempts++
		return record.attempts
	}
	return 0
}

func (s *store) sweep() {
	now := s.now()
	for id, record := range s.orders {
		if !record.Status.active() && now.Sub(record.UpdatedAt) > terminalOrderTTL {
			delete(s.orders, id)
		}
	}
}
