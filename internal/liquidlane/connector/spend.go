package connector

import (
	"math/big"
	"sync"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Spend is one fill's payments against physical account/asset capacities; a fill split across
// connectors spends from each funding account.
type Spend struct {
	Amounts    liquidlane.CapacityReservations
	IncludedAt uint64
	Deadline   int64 // Original signed chain deadline.
}

// Total is the fill's payment summed over its capacities.
func (s Spend) Total() *big.Int {
	total := new(big.Int)
	for _, amount := range s.Amounts {
		total.Add(total, amount)
	}
	return total
}

// SpendLedger keeps pending and canonical connector spends in RAM until upstream
// capacity snapshots reflect them. A nil ledger reserves nothing.
type SpendLedger struct {
	mu         sync.Mutex
	spends     map[string]Spend
	watermarks map[liquidlane.CapacityID]uint64
	completed  map[string]int64
}

func NewSpendLedger() *SpendLedger {
	return &SpendLedger{spends: make(map[string]Spend), watermarks: make(map[liquidlane.CapacityID]uint64), completed: make(map[string]int64)}
}

// Reserve records the spend of a fill before its submission.
func (l *SpendLedger) Reserve(id string, spend Spend) error {
	if l == nil {
		return nil
	}
	if id == "" || len(spend.Amounts) == 0 || spend.Deadline <= 0 {
		return errors.New("fill has no valid spend")
	}
	amounts := make(liquidlane.CapacityReservations, len(spend.Amounts))
	for capacityID, amount := range spend.Amounts {
		if capacityID == "" || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 256 {
			return errors.New("fill has no valid spend")
		}
		amounts[capacityID] = new(big.Int).Set(amount)
	}
	spend.Amounts = amounts
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spends[id] = spend
	return nil
}

// Release drops the spend of a fill that failed unsigned or reverted canonically.
func (l *SpendLedger) Release(id string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.spends, id)
}

// Finalized reports whether a fill was canonically included and its signed deadline has not passed.
func (l *SpendLedger) Finalized(id string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, found := l.completed[id]
	return found
}

// Include records the canonical inclusion block of a fill.
func (l *SpendLedger) Include(id string, spend Spend) error {
	if l == nil {
		return nil
	}
	if spend.IncludedAt == 0 {
		return errors.New("fill has no inclusion block")
	}
	if err := l.Reserve(id, spend); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.completed[id] = spend.Deadline
	return nil
}

// Expire compacts completed fills whose signed deadline passed, keeping an inclusion watermark.
func (l *SpendLedger) Expire(chainTime int64) {
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
func (l *SpendLedger) Reservations(inventory []liquidlane.Inventory) liquidlane.CapacityReservations {
	if l == nil {
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
	l.mu.Lock()
	defer l.mu.Unlock()
	reserved := make(liquidlane.CapacityReservations)
	for id, spend := range l.spends {
		for capacityID, amount := range spend.Amounts {
			block, known := blocks[capacityID]
			if spend.IncludedAt != 0 && known && block >= spend.IncludedAt {
				// Keep a watermark so later older snapshots cannot reuse spent cash.
				l.watermarks[capacityID] = max(l.watermarks[capacityID], spend.IncludedAt)
				delete(spend.Amounts, capacityID)
				continue
			}
			reserved.Add(capacityID, amount)
		}
		if len(spend.Amounts) == 0 {
			delete(l.spends, id)
		}
	}
	for capacityID, block := range blocks {
		if block < l.watermarks[capacityID] {
			reserved.Add(capacityID, maximums[capacityID])
		}
	}
	return reserved
}
