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

// offerTracker remembers our outstanding offers per (adapter, auction) so we don't re-offer through
// the same adapter while one is live, and so we can tell when an auction is fully covered. Hydrated
// from the 3F API when an adapter becomes usable (restart-safe), updated in memory as offers are
// submitted; Run goroutine only, no locking.
type offerTracker struct {
	offers map[offerKey]offerState
}

func newOfferTracker() *offerTracker {
	return &offerTracker{offers: make(map[offerKey]offerState)}
}

// liveEntries returns the (adapter, auction) keys of every unexpired offer as of now, for the strategy
// to dedup against. Cheaper than probing each adapter/auction pair: it walks only the offers we hold.
func (t *offerTracker) liveEntries(now time.Time) []offerKey {
	keys := make([]offerKey, 0, len(t.offers))
	for k, st := range t.offers {
		if st.expiry.After(now) {
			keys = append(keys, k)
		}
	}
	return keys
}

// record stores the expiration and principal of an offer we hold through adapter for auctionID.
func (t *offerTracker) record(adapter common.Address, auctionID int64, expiration time.Time, principal *big.Int) {
	t.offers[offerKey{adapter, auctionID}] = offerState{expiry: expiration, principal: new(big.Int).Set(principal)}
}

// retainAdapters drops cached offers made by adapters that are no longer usable. In particular,
// rotating an adapter's offerSigner invalidates its outstanding signatures, so those offers must no
// longer reduce the amount covered by the active snapshot.
func (t *offerTracker) retainAdapters(active map[common.Address]struct{}) {
	for key := range t.offers {
		if _, ok := active[key.adapter]; !ok {
			delete(t.offers, key)
		}
	}
}

// liveCoverage sums the principal of our unexpired offers on auctionID across every adapter — how much
// of the auction's requested amount we already cover.
func (t *offerTracker) liveCoverage(auctionID int64, now time.Time) *big.Int {
	total := new(big.Int)
	for k, st := range t.offers {
		if k.auction == auctionID && st.expiry.After(now) {
			total.Add(total, st.principal)
		}
	}
	return total
}

// pruneExpired drops entries whose offer has already expired, keeping the map bounded over a long run.
func (t *offerTracker) pruneExpired(now time.Time) {
	for k, st := range t.offers {
		if !st.expiry.After(now) {
			delete(t.offers, k)
		}
	}
}

// parseUnixTime parses a uint256 unix-seconds string (as the API encodes expirations).
func parseUnixTime(s string) (time.Time, error) {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}
