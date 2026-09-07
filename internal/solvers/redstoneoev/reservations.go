package redstoneoev

import (
	"container/list"
	"math/big"
	"strings"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const reservationTTL = 5 * time.Minute

// The bid record is retained after releasing funding, so delayed/replayed result
// frames keep the original amount and cannot emit a second lifecycle transition.
type bidRecord struct {
	bidWei       *big.Int
	won, settled bool
	pending      *pendingBid
	history      *list.Element
}

type pendingBid struct {
	nonce         uint64
	sentAt, wonAt time.Time
}

type bidSettlementTransition struct {
	bidWei       *big.Int
	won, settled bool
}

func normalizeAuctionID(id string) string { return strings.TrimSpace(id) }

// All helpers below require resMu. Active bids are never evicted to make room
// for history. The bounded inactive list replaces the separate reservation copy.
func (s *Solver) bidRecord(id string, amount *big.Int) *bidRecord {
	if s.bids == nil {
		s.bids = make(map[string]*bidRecord)
	}
	record := s.bids[id]
	if record == nil {
		record = &bidRecord{}
		s.bids[id] = record
	}
	if record.bidWei == nil && amount != nil {
		record.bidWei = bigmath.Clone(amount)
	}
	return record
}

func (s *Solver) retainBidHistory(id string, record *bidRecord) {
	record.pending = nil
	if record.history == nil {
		record.history = s.bidHistory.PushBack(id)
	}
	for s.bidHistory.Len() > maxSeenAuctions {
		oldest := s.bidHistory.Front()
		delete(s.bids, oldest.Value.(string))
		s.bidHistory.Remove(oldest)
	}
}

func (s *Solver) reserve(nonce uint64, now time.Time, id string, amount *big.Int) {
	id = normalizeAuctionID(id)
	if id == "" {
		return
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	record := s.bidRecord(id, amount)
	if record.history != nil {
		s.bidHistory.Remove(record.history)
		record.history = nil
	}
	record.pending = &pendingBid{nonce: nonce, sentAt: now}
}

func (s *Solver) releaseReservationByAuction(id string) {
	id = normalizeAuctionID(id)
	s.resMu.Lock()
	defer s.resMu.Unlock()
	if record := s.bids[id]; record != nil {
		s.retainBidHistory(id, record)
	}
}

func (s *Solver) markReservationWon(id string, now time.Time) (*big.Int, bool) {
	id = normalizeAuctionID(id)
	if id == "" || now.IsZero() {
		return nil, false
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	record := s.bidRecord(id, nil)
	if pending := record.pending; pending != nil && pending.wonAt.IsZero() {
		pending.wonAt = now
	}
	changed := !record.won
	record.won = true
	if record.pending == nil {
		s.retainBidHistory(id, record)
	}
	return bigmath.Clone(record.bidWei), changed
}

func (s *Solver) settleReservationByAuction(id, key string) bidSettlementTransition {
	id, key = normalizeAuctionID(id), strings.TrimSpace(key)
	if key == "" {
		return bidSettlementTransition{}
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	record := s.bidRecord(key, nil)
	if source := s.bids[id]; source != nil && source != record {
		if record.bidWei == nil {
			record.bidWei = bigmath.Clone(source.bidWei)
		}
		record.won = record.won || source.won
		s.retainBidHistory(id, source)
	}
	transition := bidSettlementTransition{bidWei: bigmath.Clone(record.bidWei), won: !record.won, settled: !record.settled}
	record.won, record.settled = true, true
	s.retainBidHistory(key, record)
	return transition
}

// inFlightSnapshot returns owned strategy values; filtering and ordering happen
// at the decision boundary, so reservation bookkeeping retains even expired wins.
func (s *Solver) inFlightSnapshot() []types.PendingAuction {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	var pending []types.PendingAuction
	for id, record := range s.bids {
		if bid := record.pending; bid != nil {
			pending = append(pending, types.PendingAuction{ID: id, SentAt: bid.sentAt, Won: !bid.wonAt.IsZero()})
		}
	}
	return pending
}

func (s *Solver) wonReservationMetrics() (int, time.Duration) {
	return s.wonReservationMetricsAt(time.Now())
}

func (s *Solver) wonReservationMetricsAt(now time.Time) (int, time.Duration) {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	count, age := 0, time.Duration(0)
	for _, record := range s.bids {
		if pending := record.pending; pending != nil && !pending.wonAt.IsZero() {
			count++
			age = max(age, now.Sub(pending.wonAt))
		}
	}
	return count, age
}

func (s *Solver) pruneReservations(nonce uint64, now time.Time) {
	s.resMu.Lock()
	unresolved := 0
	for id, record := range s.bids {
		pending := record.pending
		if pending == nil {
			continue
		}
		anchor := pending.sentAt
		if !pending.wonAt.IsZero() {
			anchor = pending.wonAt
		}
		expired := now.Sub(anchor) > reservationTTL
		// Executor consumes nonce = chain+1, then advances chain nonce to that value.
		if pending.nonce <= nonce || expired {
			if pending.nonce > nonce && expired && !pending.wonAt.IsZero() {
				unresolved++
			}
			s.retainBidHistory(id, record)
		}
	}
	s.resMu.Unlock()
	s.metrics.unresolvedWins(unresolved)
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
