package bridgefacilitator

import (
	"math/big"
)

// sizeInputs are the bounds that constrain how much principal the bot may offer for one Request. The
// caps mirror the adapter's authoritative on-chain per-request limits (each 0 = disabled).
type sizeInputs struct {
	fundable  *big.Int // getMaxAssets() headroom, less this pass's commitments (chain read)
	maxAssets *big.Int // adapter maxAssetsPerRequest (0 = no per-request ceiling)
	minAssets *big.Int // adapter minAssetsPerRequest (0 = no per-request floor)

	openCount     int
	maxConcurrent int // MAX_REQUESTS (compile-time const, not 0=disabled)
}

// sizeOffer returns the max principal an adapter can fund for one Request (its capacity) and whether it
// can bid. fundable is a hard cap (getMaxAssets already folds in the delegator cap + vault liquidity);
// the per-request ceiling applies only when set (0 = disabled). Capacity below the per-request floor
// can't fund a valid request, so the adapter can't bid. Capacity is independent of the ask —
// selectOffers clamps it to the uncovered amount (and re-checks the floor after clamping).
func sizeOffer(in sizeInputs) (*big.Int, bool) {
	if in.maxConcurrent > 0 && in.openCount >= in.maxConcurrent {
		return nil, false
	}

	amount := new(big.Int).Set(in.fundable)
	if in.maxAssets != nil && in.maxAssets.Sign() > 0 {
		amount = minBig(amount, in.maxAssets)
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
