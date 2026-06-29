package bridgefacilitator

import (
	"math/big"
)

// sizeInputs are the bounds that constrain how much principal the bot may offer for one Request. The
// caps mirror the adapter's authoritative on-chain exposure limits (each 0 = disabled).
type sizeInputs struct {
	perRequestMax *big.Int // adapter perRequestMaxCollateral (0 = no limit)
	fundable      *big.Int // delegator-cap + vault-liquidity headroom, incl. the sleeve ceiling (chain read)

	openCount     int
	maxConcurrent int // adapter maxConcurrentLoans (0 = no limit)
}

// sizeOffer returns the max principal an adapter can fund for one Request (its capacity) and whether it
// can bid. fundable is a hard cap (it folds in the delegator's per-adapter limitOf / sleeve ceiling);
// per-Request and concurrency caps apply only when set (0 = disabled). Capacity is independent of the
// ask — selectOffers clamps it to the uncovered amount.
func sizeOffer(in sizeInputs) (*big.Int, bool) {
	if in.maxConcurrent > 0 && in.openCount >= in.maxConcurrent {
		return nil, false
	}

	amount := new(big.Int).Set(in.fundable)
	if in.perRequestMax != nil && in.perRequestMax.Sign() > 0 {
		amount = minBig(amount, in.perRequestMax)
	}
	if amount.Sign() <= 0 {
		return nil, false
	}
	return amount, true
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}
