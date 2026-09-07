package bridgefacilitator

import (
	"math/big"
	"strconv"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
)

type offerKey struct {
	adapter common.Address
	auction int64
}
type offerState struct {
	expiry    time.Time
	principal *big.Int
}

type offerSnapshot struct {
	entries  []offerKey
	coverage map[int64]*big.Int
}

// The Run goroutine replaces one adapter's authoritative API snapshot at a time.
// A failed read leaves that adapter's previous coverage until its offers expire.
type offerTracker struct {
	adapters map[common.Address]map[int64]offerState
}

func newOfferTracker() *offerTracker {
	return &offerTracker{adapters: make(map[common.Address]map[int64]offerState)}
}

func (t *offerTracker) reconcileAdapter(adapter common.Address, offers map[int64]offerState) {
	if len(offers) == 0 {
		delete(t.adapters, adapter)
		return
	}
	snapshot := make(map[int64]offerState, len(offers))
	for id, offer := range offers {
		snapshot[id] = offerState{expiry: offer.expiry, principal: bigmath.Clone(offer.principal)}
	}
	t.adapters[adapter] = snapshot
}

// snapshot collects deduplication keys and auction coverage in the same pass.
// Coverage owns its amounts; no strategy can mutate the authoritative tracker.
func (t *offerTracker) snapshot(now time.Time) offerSnapshot {
	out := offerSnapshot{coverage: make(map[int64]*big.Int)}
	for adapter, offers := range t.adapters {
		for auction, offer := range offers {
			if !offer.expiry.After(now) {
				continue
			}
			out.entries = append(out.entries, offerKey{adapter: adapter, auction: auction})
			if offer.principal != nil {
				if out.coverage[auction] == nil {
					out.coverage[auction] = new(big.Int)
				}
				out.coverage[auction].Add(out.coverage[auction], offer.principal)
			}
		}
	}
	return out
}

func (t *offerTracker) retainAdapters(active map[common.Address]struct{}) {
	for adapter := range t.adapters {
		if _, ok := active[adapter]; !ok {
			delete(t.adapters, adapter)
		}
	}
}

func (t *offerTracker) pruneExpired(now time.Time) {
	for adapter, offers := range t.adapters {
		for id, offer := range offers {
			if !offer.expiry.After(now) {
				delete(offers, id)
			}
		}
		if len(offers) == 0 {
			delete(t.adapters, adapter)
		}
	}
}

func parseUnixTime(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0), nil
}
