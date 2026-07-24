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
}

// Set stores one pending fill reservation. It reports whether the ledger changed.
func (ledger *CapacityLedger) Set(key string, reservations CapacityReservations) bool {
	normalized, ok := cloneValidReservations(reservations)
	if key == "" || !ok {
		return false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.byKey == nil {
		ledger.byKey = make(map[string]CapacityReservations)
	}
	ledger.byKey[key] = normalized
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
	return true
}

// Snapshot returns the aggregate reservation without exposing ledger state.
func (ledger *CapacityLedger) Snapshot() CapacityReservations {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	out := make(CapacityReservations)
	for _, reservations := range ledger.byKey {
		out.AddAll(reservations)
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
