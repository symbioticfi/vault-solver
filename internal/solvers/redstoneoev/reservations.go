package redstoneoev

// reservations.go holds the in-flight auction lifecycle state and auction-id dedup ring.

import (
	"math/big"
	"slices"
	"time"
)

// reservedBid is one sent-but-not-yet-resolved bid. The solver tracks lifecycle only: strategies own
// callback funding/gas decisions, while this state lets the solver pass pending auction ids back to the
// strategy and release reservations by nonce/result frames.
type reservedBid struct {
	nonce     uint64
	at        time.Time
	auctionID string
	bidWei    *big.Int
	won       bool
}

// reservationTTL is only a fallback for missed auction/liquidation result frames. Normal release is
// event-driven: a lost auction-result or our liquidation-result frees the bid immediately, while a won bid
// without a result stays pinned long enough for delayed settlement/nonce reconciliation.
const reservationTTL = 5 * time.Minute

type inFlightState struct {
	pending []pendingAuction
}

type pendingAuction struct {
	ID     string
	SentAt time.Time
	Won    bool
}

// inFlightSnapshot returns the bounded pending-auction state strategies use for de-duping/risk control.
func (s *Solver) inFlightSnapshot() inFlightState {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	var out inFlightState
	if len(s.res) > 0 {
		out.pending = make([]pendingAuction, 0, len(s.res))
	}
	for _, r := range s.res {
		out.pending = append(out.pending, pendingAuction{ID: r.auctionID, SentAt: r.at, Won: r.won})
	}
	return out
}

func (s *Solver) reserve(
	nonce uint64,
	now time.Time,
	auctionID string,
	bidWei *big.Int,
) {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = append(s.res, reservedBid{
		nonce:     nonce,
		at:        now,
		auctionID: auctionID,
		bidWei:    cloneBig(bidWei),
	})
}

func (s *Solver) releaseReservationByAuction(id string) (reservation reservedBid, released bool) {
	if id == "" {
		return reservedBid{}, false
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = slices.DeleteFunc(s.res, func(r reservedBid) bool {
		if r.auctionID != id {
			return false
		}
		reservation = r
		released = true
		return true
	})
	return reservation, released
}

func (s *Solver) markReservationWon(id string) (*big.Int, bool) {
	if id == "" {
		return nil, false
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	for i := range s.res {
		if s.res[i].auctionID == id && !s.res[i].won {
			s.res[i].won = true
			return cloneBig(s.res[i].bidWei), true
		}
	}
	return nil, false
}

func (s *Solver) wonReservationCount() int {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	count := 0
	for _, r := range s.res {
		if r.won {
			count++
		}
	}
	return count
}

// pruneReservations frees a reservation once its bid resolves: when nonce <= the on-chain nonce (the bid
// won and settled — a pending bid is signed with nonce = on-chain + 1, so settlement sets the on-chain nonce
// to exactly the consumed bid's, and `<=` releases precisely then), or once it has aged past reservationTTL.
// Still-pending bids stay pinned.
func (s *Solver) pruneReservations(onChainNonce uint64, now time.Time) {
	s.resMu.Lock()
	unresolvedWins := 0
	s.res = slices.DeleteFunc(s.res, func(r reservedBid) bool {
		if r.nonce <= onChainNonce {
			return true
		}
		expired := now.Sub(r.at) > reservationTTL
		if expired && r.won {
			unresolvedWins++
		}
		return expired
	})
	s.resMu.Unlock()
	s.metrics.unresolvedWins(unresolvedWins)
}

// maxSeenAuctions bounds the de-dup set (insertion-ordered eviction); ample for the auction cadence.
const maxSeenAuctions = 1024

// seenAuctions is a bounded, insertion-ordered de-dup set for auction ids: a re-subscribe on reconnect can
// replay a frame, and bidding twice for one auction burns a second nonce for the same opportunity.
// Touched only while parsing auction frames before bid work is dispatched, so it needs no lock.
type seenAuctions struct {
	set   map[string]struct{}
	order []string
	cap   int
}

func newSeenAuctions(capacity int) *seenAuctions {
	return &seenAuctions{set: make(map[string]struct{}, capacity), cap: capacity}
}

// seen reports whether id was already processed; if not, it records it (evicting the oldest past cap).
func (s *seenAuctions) seen(id string) bool {
	if _, ok := s.set[id]; ok {
		return true
	}
	if len(s.order) >= s.cap {
		delete(s.set, s.order[0])
		s.order = s.order[1:]
	}
	s.set[id] = struct{}{}
	s.order = append(s.order, id)
	return false
}
