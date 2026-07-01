package bridgefacilitator

import (
	"math/big"
)

// sizeInputs are the bounds that constrain how much principal the bot may offer for one Request. The
// caps mirror the adapter's authoritative on-chain per-request limits.
type sizeInputs struct {
	fundable  *big.Int // getMaxAssets() headroom, less this pass's commitments (chain read)
	maxAssets *big.Int // maxAssetsPerRequest — always-active ceiling (0 = reject-all)
	minAssets *big.Int // minAssetsPerRequest (0 = no floor)

	openCount     int
	maxConcurrent int // MAX_REQUESTS (compile-time const)
}

// sizeOffer returns the max principal an adapter can fund for one Request (its capacity), or false if it
// can't bid. maxAssets is always an active ceiling — the contract reverts TooLargeRequest on principal >
// maxAssetsPerRequest, so 0 is reject-all, not "no cap"; minAssets is a floor (vacuous at 0). Capacity is
// independent of the ask; selectOffers clamps it and re-checks the floor.
func sizeOffer(in sizeInputs) (*big.Int, bool) {
	if in.maxConcurrent > 0 && in.openCount >= in.maxConcurrent {
		return nil, false
	}

	amount := new(big.Int).Set(in.fundable)
	if in.maxAssets != nil {
		amount = minBig(amount, in.maxAssets) // always-active ceiling; 0 ⇒ no bid
	}
	if amount.Sign() <= 0 {
		return nil, false
	}
	if in.minAssets != nil && amount.Cmp(in.minAssets) < 0 {
		return nil, false // capacity below the on-chain minimum request size
	}
	return amount, true
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}
