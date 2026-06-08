package bridgefacilitator

import (
	"strconv"
	"time"
)

// offerTracker remembers, per auction, when our currently-outstanding offer expires, so we don't
// re-offer while a live offer exists. It is rebuilt from the 3F API at startup (restart-safe) and
// updated in memory as offers are submitted. Accessed only from the Run goroutine; no locking.
type offerTracker struct {
	expiry map[int64]time.Time // auctionID -> our offer's expiration
}

func newOfferTracker() *offerTracker {
	return &offerTracker{expiry: make(map[int64]time.Time)}
}

// hasLive reports whether we hold an unexpired offer for auctionID as of now.
func (t *offerTracker) hasLive(auctionID int64, now time.Time) bool {
	exp, ok := t.expiry[auctionID]
	return ok && exp.After(now)
}

// record stores the expiration of an offer we hold for auctionID.
func (t *offerTracker) record(auctionID int64, expiration time.Time) {
	t.expiry[auctionID] = expiration
}

// parseUnixTime parses a uint256 unix-seconds string (as the API encodes expirations).
func parseUnixTime(s string) (time.Time, error) {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}
