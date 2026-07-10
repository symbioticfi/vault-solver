package defaultstrategy

import (
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type positionKey struct {
	market   common.Hash
	borrower common.Address
}

type decisionReservation struct {
	bidNative  *big.Int
	gasNative  *big.Int
	positions  []positionKey
	confirmed  bool
	resolvedAt time.Time
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

func (r *decisionReservations) reserve(auctionID string, priced pricedBundle) {
	if auctionID == "" {
		return
	}
	positions := make([]positionKey, 0, len(priced.selectedLegs))
	for _, leg := range priced.selectedLegs {
		positions = append(positions, positionKey{market: leg.MarketId, borrower: leg.Borrower})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byAuction == nil {
		r.byAuction = make(map[string]*decisionReservation)
	}
	r.byAuction[auctionID] = &decisionReservation{
		bidNative: cloneBig(priced.bidNative),
		gasNative: cloneBig(priced.gasNative),
		positions: positions,
	}
}

func (r *decisionReservations) reconcile(
	pending []types.PendingAuction,
	now time.Time,
	callbackUpdatedAt time.Time,
) reservedResources {
	pendingIDs := make(map[string]struct{}, len(pending))
	for _, auction := range pending {
		if auction.ID != "" && (auction.ExpiresAt.IsZero() || now.Before(auction.ExpiresAt)) {
			pendingIDs[auction.ID] = struct{}{}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	out := reservedResources{
		bidNative: new(big.Int),
		gasNative: new(big.Int),
		positions: make(map[positionKey]struct{}),
	}
	for auctionID, reservation := range r.byAuction {
		if _, ok := pendingIDs[auctionID]; ok {
			reservation.confirmed = true
			reservation.resolvedAt = time.Time{}
		} else {
			switch {
			case !reservation.confirmed:
				delete(r.byAuction, auctionID)
				continue
			case reservation.resolvedAt.IsZero():
				reservation.resolvedAt = now
			case callbackUpdatedAt.After(reservation.resolvedAt):
				delete(r.byAuction, auctionID)
				continue
			}
		}
		out.bidNative.Add(out.bidNative, orZero(reservation.bidNative))
		out.gasNative.Add(out.gasNative, orZero(reservation.gasNative))
		for _, position := range reservation.positions {
			out.positions[position] = struct{}{}
		}
	}
	return out
}

func filterReservedPositions(scored []scoredLeg, reserved map[positionKey]struct{}) []scoredLeg {
	if len(reserved) == 0 {
		return scored
	}
	filtered := scored[:0]
	for _, leg := range scored {
		key := positionKey{market: leg.MarketId, borrower: leg.Borrower}
		if _, found := reserved[key]; !found {
			filtered = append(filtered, leg)
		}
	}
	return filtered
}
