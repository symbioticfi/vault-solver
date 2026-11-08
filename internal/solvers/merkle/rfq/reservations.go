package rfq

import (
	"sync"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type fillSpend struct {
	CapacityID liquidlane.CapacityID
	IncludedAt uint64
	Deadline   int64 // Original signed chain deadline.
}

type spendLedger struct {
	mu         sync.Mutex
	active     bool
	watermarks map[liquidlane.CapacityID]uint64
	completed  map[string]int64
}

func newSpendLedger() *spendLedger {
	return &spendLedger{watermarks: make(map[liquidlane.CapacityID]uint64), completed: make(map[string]int64)}
}

func (l *spendLedger) setActive(active bool) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active = active
}

func (l *spendLedger) finalized(id string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, found := l.completed[id]
	return found
}

func (l *spendLedger) include(id string, spend fillSpend) error {
	if l == nil {
		return nil
	}
	if id == "" || spend.CapacityID == "" || spend.IncludedAt == 0 || spend.Deadline <= 0 {
		return errors.New("RFQ fill has no valid spend or inclusion block")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.watermarks[spend.CapacityID] = max(l.watermarks[spend.CapacityID], spend.IncludedAt)
	l.completed[id] = spend.Deadline
	return nil
}

func (l *spendLedger) expire(chainTime int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, deadline := range l.completed {
		if deadline <= chainTime {
			delete(l.completed, id)
		}
	}
}

// AcceptsSnapshots also fences the gap between shared-sender release and the
// fill loop recording its canonical inclusion. Quotes call it after the lane read.
func (s *Solver) AcceptsSnapshots(inventory []liquidlane.Inventory) bool {
	if s.spends == nil {
		return true
	}
	s.spends.mu.Lock()
	defer s.spends.mu.Unlock()
	if s.spends.active {
		return false
	}
	for _, item := range inventory {
		if item.BlockNumber < s.spends.watermarks[liquidlane.RouteCapacityID(item.Route)] {
			return false
		}
	}
	return true
}
