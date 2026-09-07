package defaultstrategy

import (
	"math/big"
	"sync"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type positionKey struct {
	market   common.Hash
	borrower common.Address
}

type reservationPhase uint8

const (
	reservationProposed reservationPhase = iota
	reservationPending
	reservationAwaitingBalance
)

type decisionReservation struct {
	bidNative, gasNative *big.Int
	positions            []positionKey
	phase                reservationPhase
	balanceAfter         time.Time
}

type reservedResources struct {
	bidNative *big.Int
	gasNative *big.Int
	positions map[positionKey]struct{}
}

// decisionReservations keeps callback and Executor headroom in the strategy that produced the decision.
// Solver PendingAuctions supplies lifecycle only; a fresh callback balance releases resolved reservations.
type decisionReservations struct {
	mu        sync.Mutex
	byAuction map[string]*decisionReservation
}

func (r *decisionReservations) reserve(id string, priced pricedBundle) {
	if id == "" {
		return
	}
	entry := &decisionReservation{bidNative: bigmath.Clone(priced.bidNative), gasNative: bigmath.Clone(priced.gasNative), positions: make([]positionKey, len(priced.selectedLegs))}
	for i, leg := range priced.selectedLegs {
		entry.positions[i] = positionKey{market: leg.MarketId, borrower: leg.Borrower}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byAuction == nil {
		r.byAuction = make(map[string]*decisionReservation)
	}
	r.byAuction[id] = entry
}

func (r *decisionReservations) reconcile(pending []types.PendingAuction, now, balanceUpdated time.Time) reservedResources {
	live := make(map[string]bool, len(pending))
	for _, auction := range pending {
		if auction.ID != "" && (auction.ExpiresAt.IsZero() || now.Before(auction.ExpiresAt)) {
			live[auction.ID] = true
		}
	}
	result := reservedResources{bidNative: new(big.Int), gasNative: new(big.Int), positions: make(map[positionKey]struct{})}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, entry := range r.byAuction {
		if live[id] {
			entry.phase, entry.balanceAfter = reservationPending, time.Time{}
		} else {
			switch entry.phase {
			case reservationProposed:
				delete(r.byAuction, id)
				continue
			case reservationPending:
				entry.phase, entry.balanceAfter = reservationAwaitingBalance, now
			case reservationAwaitingBalance:
				if balanceUpdated.After(entry.balanceAfter) {
					delete(r.byAuction, id)
					continue
				}
			}
		}
		if entry.bidNative != nil {
			result.bidNative.Add(result.bidNative, entry.bidNative)
		}
		if entry.gasNative != nil {
			result.gasNative.Add(result.gasNative, entry.gasNative)
		}
		for _, position := range entry.positions {
			result.positions[position] = struct{}{}
		}
	}
	return result
}

func filterReservedPositions(scored []scoredLeg, reserved map[positionKey]struct{}) []scoredLeg {
	if len(reserved) == 0 {
		return scored
	}
	keep := 0
	for _, leg := range scored {
		if _, held := reserved[positionKey{market: leg.MarketId, borrower: leg.Borrower}]; !held {
			scored[keep] = leg
			keep++
		}
	}
	clear(scored[keep:])
	return scored[:keep]
}
