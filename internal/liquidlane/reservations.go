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
	mu       sync.RWMutex
	byKey    map[string]CapacityReservations
	revision uint64
}

// Set stores one pending fill reservation. It reports whether the ledger changed.
func (ledger *CapacityLedger) Set(key string, reservations CapacityReservations) bool {
	return ledger.set(key, reservations, nil)
}

func (ledger *CapacityLedger) set(key string, reservations CapacityReservations, revision *uint64) bool {
	normalized, ok := cloneValidReservations(reservations)
	if key == "" || !ok {
		return false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if revision != nil && ledger.revision != *revision {
		return false
	}
	if ledger.byKey == nil {
		ledger.byKey = make(map[string]CapacityReservations)
	}
	ledger.byKey[key] = normalized
	ledger.revision++
	return true
}

// Delete releases one pending fill reservation. It reports whether the ledger changed.
func (ledger *CapacityLedger) Delete(key string) bool {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, ok := ledger.byKey[key]; !ok {
		return false
	}
	delete(ledger.byKey, key)
	ledger.revision++
	return true
}

// Snapshot returns the aggregate reservation without exposing ledger state.
func (ledger *CapacityLedger) Snapshot() CapacityReservations {
	return ledger.SnapshotExcluding("")
}

// SnapshotExcluding returns the aggregate reservation without one pending fill.
// It lets an owner plan a replacement before releasing the old reservation.
func (ledger *CapacityLedger) SnapshotExcluding(excludedKey string) CapacityReservations {
	return ledger.SnapshotIf(func(key string, _ CapacityID) bool { return key != excludedKey })
}

// SnapshotIf aggregates only the reservations keep admits, decided per pending fill and capacity.
func (ledger *CapacityLedger) SnapshotIf(keep func(key string, capacityID CapacityID) bool) CapacityReservations {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	out := make(CapacityReservations)
	for key, reservations := range ledger.byKey {
		for capacityID, amount := range reservations {
			if keep(key, capacityID) {
				out.Add(capacityID, amount)
			}
		}
	}
	return out
}

// Has reports whether one pending fill holds a reservation.
func (ledger *CapacityLedger) Has(key string) bool {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	_, ok := ledger.byKey[key]
	return ok
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

// Revision changes whenever an order acquires, replaces or releases capacity.
// Quotes recheck it after calculation; fill planners use SetAt to reject stale plans.
func (ledger *CapacityLedger) Revision() uint64 {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	return ledger.revision
}

// SetAt installs a plan only if no reservation changed since its input was read.
func (ledger *CapacityLedger) SetAt(key string, reservations CapacityReservations, revision uint64) bool {
	return ledger.set(key, reservations, &revision)
}
