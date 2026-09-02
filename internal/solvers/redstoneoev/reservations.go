package redstoneoev

// reservations.go holds the in-flight auction lifecycle state and auction-id dedup ring.

import (
	"math/big"
	"slices"
	"strings"
	"time"
)

// reservedBid is one enqueued-but-not-yet-resolved bid. The solver tracks lifecycle only: strategies own
// callback funding/gas decisions, while this state lets the solver pass pending auction ids back to the
// strategy and release reservations by nonce/result frames.
type reservedBid struct {
	nonce     uint64
	at        time.Time
	auctionID string
	bidWei    *big.Int
	won       bool
	wonAt     time.Time // first matched local win transition; never advanced by replayed results
}

type bidLifecycleRecord struct {
	bidWei  *big.Int
	won     bool
	settled bool
}

type bidSettlementTransition struct {
	bidWei  *big.Int
	won     bool
	settled bool
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
	auctionID = normalizeAuctionID(auctionID)
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = append(s.res, reservedBid{
		nonce:     nonce,
		at:        now,
		auctionID: auctionID,
		bidWei:    cloneBig(bidWei),
	})
	s.ensureBidLifecycleLocked(auctionID, bidWei)
}

func (s *Solver) releaseReservationByAuction(id string) {
	id = normalizeAuctionID(id)
	if id == "" {
		return
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = slices.DeleteFunc(s.res, func(r reservedBid) bool { return r.auctionID == id })
}

func (s *Solver) markReservationWon(id string, wonAt time.Time) (*big.Int, bool) {
	id = normalizeAuctionID(id)
	if id == "" || wonAt.IsZero() {
		return nil, false
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	var (
		bidWei            *big.Int
		reservationWasWon bool
	)
	for i := range s.res {
		if s.res[i].auctionID != id {
			continue
		}
		bidWei = s.res[i].bidWei
		reservationWasWon = s.res[i].won
		if !reservationWasWon {
			s.res[i].won = true
			s.res[i].wonAt = wonAt
		}
		break
	}
	record := s.ensureBidLifecycleLocked(id, bidWei)
	if record == nil {
		return cloneBig(bidWei), false
	}
	if reservationWasWon {
		record.won = true
	}
	if record.won {
		return cloneBig(record.bidWei), false
	}
	record.won = true
	return cloneBig(record.bidWei), true
}

func (s *Solver) settleReservationByAuction(id, lifecycleKey string) bidSettlementTransition {
	id = normalizeAuctionID(id)
	lifecycleKey = strings.TrimSpace(lifecycleKey)
	if lifecycleKey == "" {
		return bidSettlementTransition{}
	}

	s.resMu.Lock()
	defer s.resMu.Unlock()
	var (
		reservation      reservedBid
		reservationFound bool
	)
	s.res = slices.DeleteFunc(s.res, func(candidate reservedBid) bool {
		if id == "" || candidate.auctionID != id {
			return false
		}
		reservation = candidate
		reservationFound = true
		return true
	})
	record := s.ensureBidLifecycleLocked(lifecycleKey, reservation.bidWei)
	if reservationFound && reservation.won {
		record.won = true
	}
	transition := bidSettlementTransition{bidWei: cloneBig(record.bidWei)}
	if !record.won {
		record.won = true
		transition.won = true
	}
	if !record.settled {
		record.settled = true
		transition.settled = true
	}
	return transition
}

func (s *Solver) ensureBidLifecycleLocked(id string, bidWei *big.Int) *bidLifecycleRecord {
	if id == "" {
		return nil
	}
	if s.bidLifecycle == nil {
		s.bidLifecycle = make(map[string]*bidLifecycleRecord)
	}
	if record := s.bidLifecycle[id]; record != nil {
		if record.bidWei == nil && bidWei != nil {
			record.bidWei = cloneBig(bidWei)
		}
		return record
	}
	if len(s.bidLifecycleOrder) >= maxSeenAuctions {
		evictionIndex := slices.IndexFunc(s.bidLifecycleOrder, func(existingID string) bool {
			record := s.bidLifecycle[existingID]
			return record == nil || record.settled
		})
		evictionIndex = max(0, evictionIndex)
		evicted := s.bidLifecycleOrder[evictionIndex]
		delete(s.bidLifecycle, evicted)
		s.bidLifecycleOrder = slices.Delete(s.bidLifecycleOrder, evictionIndex, evictionIndex+1)
	}
	record := &bidLifecycleRecord{bidWei: cloneBig(bidWei)}
	s.bidLifecycle[id] = record
	s.bidLifecycleOrder = append(s.bidLifecycleOrder, id)
	return record
}

func normalizeAuctionID(id string) string {
	return strings.TrimSpace(id)
}

func (s *Solver) wonReservationMetrics() (int, time.Duration) {
	return s.wonReservationMetricsAt(time.Now())
}

func (s *Solver) wonReservationMetricsAt(now time.Time) (int, time.Duration) {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	count := 0
	var oldestAge time.Duration
	for _, r := range s.res {
		if !r.won || r.wonAt.IsZero() {
			continue
		}
		count++
		oldestAge = max(oldestAge, now.Sub(r.wonAt))
	}
	return count, oldestAge
}

// pruneReservations frees a reservation once its bid resolves: when nonce <= the on-chain nonce (the bid
// won and settled — a pending bid is signed with nonce = on-chain + 1, so settlement sets the on-chain nonce
// to exactly the consumed bid's, and `<=` releases precisely then), or once it has aged past reservationTTL.
// A won reservation's fallback TTL starts at its observed win so a late win cannot expire immediately.
// Still-pending bids stay pinned.
func (s *Solver) pruneReservations(onChainNonce uint64, now time.Time) {
	s.resMu.Lock()
	unresolvedWins := 0
	s.res = slices.DeleteFunc(s.res, func(r reservedBid) bool {
		if r.nonce <= onChainNonce {
			return true
		}
		anchor := r.at
		if r.won && !r.wonAt.IsZero() {
			anchor = r.wonAt
		}
		expired := now.Sub(anchor) > reservationTTL
		if expired && r.won {
			unresolvedWins++
		}
		return expired
	})
	s.resMu.Unlock()
	s.metrics.unresolvedWins(unresolvedWins)
}

// maxSeenAuctions bounds both lifecycle replay protection and auction-ingress de-duplication.
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
