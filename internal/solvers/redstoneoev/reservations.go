package redstoneoev

// reservations.go holds the in-flight-bid headroom reservation subsystem and the auction-id dedup ring.

import (
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// positionKey identifies a borrower position (one Morpho market + borrower) — the unit a bid liquidates.
type positionKey struct {
	market   common.Hash
	borrower common.Address
}

// reservedBid is one sent-but-not-yet-resolved bid's commitment against cached headroom: payBid native,
// predicted Executor-deposit gas debit, signed nonce, send time, and the positions it liquidates.
type reservedBid struct {
	bidNative  *big.Int
	gasNative  *big.Int
	nonce      uint64
	at         time.Time
	positions  []positionKey
	auctionID  string
	auctionKey common.Hash
	gasUnits   uint64
	gasRoutes  string
}

// reservationTTL is only a fallback for missed auction/liquidation result frames. Normal release is
// event-driven: a lost auction-result or our liquidation-result frees the bid immediately, while a won bid
// without a result stays pinned long enough for delayed settlement/nonce reconciliation.
const reservationTTL = 5 * time.Minute

type inFlightState struct {
	positions map[positionKey]bool
	bidNative *big.Int
	gasNative *big.Int
}

// inFlightSnapshot returns, in ONE pass under resMu, everything buildBid needs about sent-but-unresolved
// bids: the (market,borrower) set, the reserved callback payBid native, and the reserved Executor-deposit
// gas debit.
func (s *Solver) inFlightSnapshot() inFlightState {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	out := inFlightState{bidNative: new(big.Int), gasNative: new(big.Int)}
	if len(s.res) > 0 {
		out.positions = make(map[positionKey]bool, len(s.res))
	}
	for _, r := range s.res {
		out.bidNative.Add(out.bidNative, orZero(r.bidNative))
		out.gasNative.Add(out.gasNative, orZero(r.gasNative))
		for _, p := range r.positions {
			out.positions[p] = true
		}
	}
	return out
}

// reserve records the headroom a just-sent bid commits: bid native, predicted gas debit from
// the Executor deposit, and the positions it liquidates.
func (s *Solver) reserve(bidNative, gasNative *big.Int, nonce uint64, now time.Time, positions []positionKey, auctionID string, auctionKey common.Hash, gas gasPrediction) {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = append(s.res, reservedBid{
		bidNative:  orZero(bidNative),
		gasNative:  orZero(gasNative),
		nonce:      nonce,
		at:         now,
		positions:  positions,
		auctionID:  auctionID,
		auctionKey: auctionKey,
		gasUnits:   gas.Units,
		gasRoutes:  gasRoutesString(gas.Routes),
	})
}

func (s *Solver) reservationByAuction(id string) (reservedBid, bool) {
	if id == "" {
		return reservedBid{}, false
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	for _, r := range s.res {
		if r.auctionID == id {
			return r, true
		}
	}
	return reservedBid{}, false
}

func (s *Solver) releaseReservationByAuction(id string) {
	if id == "" {
		return
	}
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = slices.DeleteFunc(s.res, func(r reservedBid) bool { return r.auctionID == id })
}

// pruneReservations frees a reservation once its bid resolves: when nonce <= the on-chain nonce (the bid
// won and settled — a pending bid is signed with nonce = on-chain + 1, so settlement sets the on-chain nonce
// to exactly the consumed bid's, and `<=` releases precisely then), or once it has aged past reservationTTL.
// Still-pending bids stay pinned.
func (s *Solver) pruneReservations(onChainNonce uint64, now time.Time) {
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.res = slices.DeleteFunc(s.res, func(r reservedBid) bool {
		return r.resolved(onChainNonce, now)
	})
}

func (r reservedBid) resolved(onChainNonce uint64, now time.Time) bool {
	return r.nonce <= onChainNonce || now.Sub(r.at) > reservationTTL
}

// maxSeenAuctions bounds the de-dup set (insertion-ordered eviction); ample for the auction cadence.
const maxSeenAuctions = 1024

// seenAuctions is a bounded, insertion-ordered de-dup set for auction ids: a re-subscribe on reconnect can
// replay a frame, and bidding twice for one auction burns a second nonce + reserves a second headroom.
// Touched only by the single WS read goroutine (handleMessage), so it needs no lock.
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
