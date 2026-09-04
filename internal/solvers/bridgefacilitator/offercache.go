package bridgefacilitator

import (
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// offerKey identifies our offer on a given auction made on behalf of a given adapter (the maker).
// Dedup is per-adapter: two adapters may each hold a live offer on the same auction.
type offerKey struct {
	adapter common.Address
	auction int64
}

// offerState is one outstanding offer: when it expires and the principal it covers.
type offerState struct {
	expiry    time.Time
	principal *big.Int
}

func (state offerState) liveAt(now time.Time) bool {
	return state.expiry.After(now)
}

func (state offerState) clone() offerState {
	return offerState{expiry: state.expiry, principal: new(big.Int).Set(state.principal)}
}

// offerTracker remembers our outstanding offers per (adapter, auction) so we don't re-offer through the
// same adapter while one is live, and so we can tell when an auction is fully covered. It is a snapshot
// of the 3F API's live offers, rebuilt from the API before every offer pass (reconcileAdapter); Run
// goroutine only, no locking.
type offerTracker struct {
	offers map[offerKey]offerState
}

func newOfferTracker() *offerTracker {
	return &offerTracker{offers: make(map[offerKey]offerState)}
}

func (t *offerTracker) count() int { return len(t.offers) }

// liveEntries returns the (adapter, auction) keys of every unexpired offer as of now, for the strategy
// to dedup against. Cheaper than probing each adapter/auction pair: it walks only the offers we hold.
func (t *offerTracker) liveEntries(now time.Time) []offerKey {
	keys := make([]offerKey, 0, len(t.offers))
	for key, state := range t.offers {
		if state.liveAt(now) {
			keys = append(keys, key)
		}
	}
	return keys
}

// reconcileAdapter replaces this adapter's cache from the authoritative API response, including
// offers submitted by this or another process.
func (t *offerTracker) reconcileAdapter(adapter common.Address, live map[int64]offerState) {
	t.remove(func(key offerKey) bool { return key.adapter == adapter })
	for auctionID, state := range live {
		t.offers[offerKey{adapter, auctionID}] = state.clone()
	}
}

// retainAdapters drops cached offers made by adapters that are no longer usable. In particular,
// rotating an adapter's offerSigner invalidates its outstanding signatures, so those offers must no
// longer reduce the amount covered by the active snapshot.
func (t *offerTracker) retainAdapters(active map[common.Address]struct{}) {
	t.remove(func(key offerKey) bool {
		_, keep := active[key.adapter]
		return !keep
	})
}

func (t *offerTracker) remove(matches func(offerKey) bool) {
	for key := range t.offers {
		if matches(key) {
			delete(t.offers, key)
		}
	}
}

// liveCoverage sums the principal of our unexpired offers on auctionID across every adapter — how much
// of the auction's requested amount we already cover.
func (t *offerTracker) liveCoverage(auctionID int64, now time.Time) *big.Int {
	total := new(big.Int)
	for key, state := range t.offers {
		if key.auction == auctionID && state.liveAt(now) {
			total.Add(total, state.principal)
		}
	}
	return total
}

// parseUnixTime parses a uint256 unix-seconds string (as the API encodes expirations).
func parseUnixTime(s string) (time.Time, error) {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}
