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

// offerState is one outstanding offer: when it expires, its signed terms, its request contract, and
// whether 3F has yet to accept it (status SUBMITTED, so a re-post with the same nonce replaces it) or
// reports it funded (status SUCCEEDED, dropped once the chain confirms the loan).
type offerState struct {
	expiry         time.Time
	principal      *big.Int
	expectedReturn *big.Int
	nonce          *big.Int
	request        common.Address
	open           bool
	funded         bool
}

// offerTracker remembers our outstanding offers per (adapter, auction) so the strategy knows what each
// adapter already holds on an auction (dedup or top-up), and so we can tell how much of an auction we
// cover. It is a snapshot
// of the 3F API's live offers, rebuilt from the API before every offer pass (reconcileAdapter); Run
// goroutine only, no locking.
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

// reconcileAdapter replaces all of this adapter's cached offers with the API's current live set. The
// 1-2 minute poll is authoritative — it always reflects our own just-submitted offers as well as any
// made out of band — so anything not in `live` is gone and dropped.
func (t *offerTracker) reconcileAdapter(adapter common.Address, live map[int64]offerState) {
	for k := range t.offers {
		if k.adapter == adapter {
			delete(t.offers, k)
		}
	}
	for auctionID, st := range live {
		t.offers[offerKey{adapter, auctionID}] = offerState{
			expiry: st.expiry, principal: cloneBig(st.principal), expectedReturn: cloneBig(st.expectedReturn),
			nonce: cloneBig(st.nonce), request: st.request, open: st.open, funded: st.funded,
		}
	}
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
// of the auction's requested amount we already cover. nil means a live commitment has an unknown amount.
func (t *offerTracker) liveCoverage(auctionID int64, now time.Time) *big.Int {
	total := new(big.Int)
	for k, st := range t.offers {
		if k.auction == auctionID && st.expiry.After(now) {
			if st.principal == nil {
				return nil
			}
			total.Add(total, st.principal)
		}
	}
	return total
}

// dropFunded forgets the adapter's SUCCEEDED offers whose request is among the adapter's on-chain
// requests: that loan is funded, so getMaxAssets and requestsLength already account for its principal
// and slot, and keeping the offer live would withhold them a second time. A SUCCEEDED offer the chain
// does not list stays live.
func (t *offerTracker) dropFunded(adapter common.Address, requests []common.Address) {
	onChain := make(map[common.Address]bool, len(requests))
	for _, r := range requests {
		onChain[r] = true
	}
	for k, st := range t.offers {
		if k.adapter == adapter && st.funded && onChain[st.request] {
			delete(t.offers, k)
		}
	}
}

// liveNonce is the nonce of our unexpired offer through adapter on auctionID, nil when there is none.
// A top-up re-signs with it: the Request contract consumes at most one offer per nonce, so the old
// and new signatures can never both fund.
func (t *offerTracker) liveNonce(adapter common.Address, auctionID int64, now time.Time) *big.Int {
	if st, ok := t.offers[offerKey{adapter, auctionID}]; ok && st.expiry.After(now) {
		return cloneBig(st.nonce)
	}
	return nil
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
