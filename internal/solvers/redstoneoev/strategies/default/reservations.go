package defaultstrategy

import (
	"sync"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type decisionReservation struct {
	confirmed  bool
	resolvedAt time.Time
}

// decisionReservations blocks another bundle while an auction is pending or awaits a balance refresh.
// Solver PendingAuctions supplies lifecycle only; the strategy owns the post-resolution refresh gate.
// The mutex protects reconciliation and reservation updates; bid decisions are serialized by the solver.
type decisionReservations struct {
	mu        sync.Mutex
	byAuction map[string]*decisionReservation
}

func (r *decisionReservations) reserve(auctionID string) {
	if auctionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byAuction == nil {
		r.byAuction = make(map[string]*decisionReservation)
	}
	r.byAuction[auctionID] = &decisionReservation{}
}

func (r *decisionReservations) reconcile(
	pending []types.PendingAuction,
	now time.Time,
	callbackUpdatedAt time.Time,
) bool {
	pendingIDs := make(map[string]struct{}, len(pending))
	for _, auction := range pending {
		if auction.ID != "" && (auction.ExpiresAt.IsZero() || now.Before(auction.ExpiresAt)) {
			pendingIDs[auction.ID] = struct{}{}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
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
	}
	return len(r.byAuction) > 0
}
