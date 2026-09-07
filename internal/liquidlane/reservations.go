package liquidlane

import (
	"math/big"
	"sync"
)

// CapacityReservations tracks unavailable output by physical vault capacity.
type CapacityReservations map[CapacityID]*big.Int

// Add accumulates a positive reservation without retaining the caller's big.Int.
func (reservations CapacityReservations) Add(capacityID CapacityID, amount *big.Int) {
	if capacityID == "" || amount == nil || amount.Sign() <= 0 {
		return
	}
	if reservations[capacityID] == nil {
		reservations[capacityID] = new(big.Int)
	}
	reservations[capacityID].Add(reservations[capacityID], amount)
}

// AddAll accumulates another reservation set.
func (reservations CapacityReservations) AddAll(additions CapacityReservations) {
	for capacityID, amount := range additions {
		reservations.Add(capacityID, amount)
	}
}

// CapacityLedger owns reservations for pending fills. Its zero value is ready to use.
type CapacityLedger struct {
	mu    sync.RWMutex
	byKey map[string]CapacityReservations
	total CapacityReservations
}

// Set stores one pending fill reservation. It reports whether the ledger changed.
func (ledger *CapacityLedger) Set(key string, reservations CapacityReservations) bool {
	owned, valid := cloneValidReservations(reservations)
	if key == "" || !valid {
		return false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.byKey == nil {
		ledger.byKey = make(map[string]CapacityReservations)
		ledger.total = make(CapacityReservations)
	}
	ledger.subtract(ledger.byKey[key])
	ledger.byKey[key] = owned
	ledger.total.AddAll(owned)
	return true
}

// Delete releases a fill and its contribution to the aggregate in one critical section.
func (ledger *CapacityLedger) Delete(key string) bool {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	owned, exists := ledger.byKey[key]
	if !exists {
		return false
	}
	ledger.subtract(owned)
	delete(ledger.byKey, key)
	return true
}

// subtract requires the write lock; each amount belongs to an existing ledger entry.
func (ledger *CapacityLedger) subtract(owned CapacityReservations) {
	for id, amount := range owned {
		remaining := ledger.total[id]
		remaining.Sub(remaining, amount)
		if remaining.Sign() == 0 {
			delete(ledger.total, id)
		}
	}
}

// Snapshot returns the aggregate reservation without exposing ledger state.
func (ledger *CapacityLedger) Snapshot() CapacityReservations {
	return ledger.SnapshotExcluding("")
}

// SnapshotExcluding returns the aggregate reservation without one pending fill.
// It lets an owner plan a replacement before releasing the old reservation.
func (ledger *CapacityLedger) SnapshotExcluding(excludedKey string) CapacityReservations {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	out := make(CapacityReservations, len(ledger.total))
	for id, amount := range ledger.total {
		value := new(big.Int).Set(amount)
		if excluded := ledger.byKey[excludedKey][id]; excluded != nil {
			value.Sub(value, excluded)
		}
		if value.Sign() > 0 {
			out[id] = value
		}
	}
	return out
}

// Len returns the number of pending fills in the ledger.
func (ledger *CapacityLedger) Len() int {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	return len(ledger.byKey)
}

func cloneValidReservations(reservations CapacityReservations) (CapacityReservations, bool) {
	if len(reservations) == 0 {
		return nil, false
	}
	out := make(CapacityReservations, len(reservations))
	for capacityID, amount := range reservations {
		if capacityID == "" || amount == nil || amount.Sign() <= 0 {
			return nil, false
		}
		out.Add(capacityID, amount)
	}
	return out, true
}
