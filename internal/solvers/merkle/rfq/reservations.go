package rfq

import (
	"math/big"
	"sync"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type fillSpend struct {
	Amounts    liquidlane.CapacityReservations
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
	if id == "" || len(spend.Amounts) == 0 || spend.Deadline <= 0 {
		return errors.New("RFQ fill has no valid spend")
	}
	amounts := make(liquidlane.CapacityReservations, len(spend.Amounts))
	for capacityID, amount := range spend.Amounts {
		if capacityID == "" || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 256 {
			return errors.New("RFQ fill has no valid spend")
		}
		amounts[capacityID] = new(big.Int).Set(amount)
	}
	spend.Amounts = amounts
	l.mu.Lock()
	defer l.mu.Unlock()
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
				for capacityID := range spend.Amounts {
					l.watermarks[capacityID] = max(l.watermarks[capacityID], spend.IncludedAt)
				}
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
		for capacityID, amount := range spend.Amounts {
			block, known := blocks[capacityID]
			if spend.IncludedAt != 0 && known && block >= spend.IncludedAt {
				// Keep a watermark so later older snapshots cannot reuse spent cash.
				s.spends.watermarks[capacityID] = max(s.spends.watermarks[capacityID], spend.IncludedAt)
				delete(spend.Amounts, capacityID)
				continue
			}
			reserved.Add(capacityID, amount)
		}
		if len(spend.Amounts) == 0 {
			delete(s.spends.spends, id)
		}
	}
	for capacityID, block := range blocks {
		if block < s.spends.watermarks[capacityID] {
			reserved.Add(capacityID, maximums[capacityID])
		}
	}
	return reserved
}
