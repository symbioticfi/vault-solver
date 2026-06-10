package bridgefacilitator

import (
	"math/big"
)

// sizeInputs are the bounds that constrain how much principal the bot may offer for one Request. The
// caps mirror the adapter's authoritative on-chain exposure limits (each 0 = disabled).
type sizeInputs struct {
	perRequestMax   *big.Int // adapter perRequestMaxCollateral (0 = no limit)
	fundable        *big.Int // delegator-cap + vault-liquidity headroom (chain read)
	amountRequested *big.Int // auction ask; nil if unknown
	sleeveMax       *big.Int // adapter totalMaxCollateral (0 = no limit)
	outstanding     *big.Int // live sleeve exposure (sum of open principals)

	openCount     int
	maxConcurrent int // adapter maxConcurrentLoans (0 = no limit)
}

// sizeOffer returns the principal to offer and whether to bid at all. `fundable` is always a hard cap —
// committing more would make the just-in-time allocation inside the consume callback revert. The
// per-Request, sleeve, and concurrency caps apply only when set (0 = disabled). Request authorization
// is enforced on-chain by the 3F whitelist at consume time, so the bot applies only these risk caps.
func sizeOffer(in sizeInputs) (*big.Int, bool) {
	if in.maxConcurrent > 0 && in.openCount >= in.maxConcurrent {
		return nil, false
	}

	amount := new(big.Int).Set(in.fundable)
	if in.perRequestMax != nil && in.perRequestMax.Sign() > 0 {
		amount = minBig(amount, in.perRequestMax)
	}
	if in.sleeveMax != nil && in.sleeveMax.Sign() > 0 {
		sleeveRoom := new(big.Int).Sub(in.sleeveMax, in.outstanding)
		if sleeveRoom.Sign() <= 0 {
			return nil, false
		}
		amount = minBig(amount, sleeveRoom)
	}
	if in.amountRequested != nil && in.amountRequested.Sign() > 0 {
		amount = minBig(amount, in.amountRequested)
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
