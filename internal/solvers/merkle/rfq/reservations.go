package rfq

import (
	"math/big"
	"sync"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type fillSpend struct {
	CapacityID liquidlane.CapacityID
	Amount     *big.Int
	IncludedAt uint64
	Deadline   int64 // Original signed chain deadline.
}

type spendLedger struct {
	mu         sync.Mutex
	spends     map[string]fillSpend
	watermarks map[liquidlane.CapacityID]uint64
	completed  map[string]int64
}

func newSpendLedger() *spendLedger {
	return &spendLedger{spends: make(map[string]fillSpend), watermarks: make(map[liquidlane.CapacityID]uint64), completed: make(map[string]int64)}
}

func (l *spendLedger) reserve(id string, spend fillSpend) error {
	if id == "" || spend.CapacityID == "" || spend.Amount == nil || spend.Amount.Sign() <= 0 || spend.Amount.BitLen() > 256 || spend.Deadline <= 0 {
		return errors.New("RFQ fill has no valid spend")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	spend.Amount = new(big.Int).Set(spend.Amount)
	l.spends[id] = spend
	return nil
}

func (l *spendLedger) release(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.spends, id)
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
	if spend.IncludedAt == 0 {
		return errors.New("RFQ fill has no inclusion block")
	}
	if err := l.reserve(id, spend); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
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
			if spend, found := l.spends[id]; found && spend.IncludedAt != 0 {
				l.watermarks[spend.CapacityID] = max(l.watermarks[spend.CapacityID], spend.IncludedAt)
				delete(l.spends, id)
			}
			delete(l.completed, id)
		}
	}
}

// Reservations subtracts pending spends and canonical spends absent from each
// capacity's oldest snapshot. An unrelated route remains available while a fill runs.
func (s *Solver) Reservations(inventory []liquidlane.Inventory) liquidlane.CapacityReservations {
	if s.spends == nil {
		return nil
	}
	blocks := make(map[liquidlane.CapacityID]uint64, len(inventory))
	maximums := make(liquidlane.CapacityReservations)
	for _, item := range inventory {
		capacityID := liquidlane.RouteCapacityID(item.Route)
		block, seen := blocks[capacityID]
		if !seen || item.BlockNumber < block {
			blocks[capacityID] = item.BlockNumber
		}
		if item.MaxAssets != nil && (maximums[capacityID] == nil || item.MaxAssets.Cmp(maximums[capacityID]) > 0) {
			maximums[capacityID] = item.MaxAssets
		}
	}
	s.spends.mu.Lock()
	defer s.spends.mu.Unlock()
	reserved := make(liquidlane.CapacityReservations)
	for id, spend := range s.spends.spends {
		block, known := blocks[spend.CapacityID]
		if spend.IncludedAt != 0 && known && block >= spend.IncludedAt {
			// Keep a watermark after pruning so later older snapshots cannot reuse spent cash.
			s.spends.watermarks[spend.CapacityID] = max(s.spends.watermarks[spend.CapacityID], spend.IncludedAt)
			delete(s.spends.spends, id)
			continue
		}
		reserved.Add(spend.CapacityID, spend.Amount)
	}
	for capacityID, block := range blocks {
		if block < s.spends.watermarks[capacityID] {
			reserved.Add(capacityID, maximums[capacityID])
		}
	}
	return reserved
}
