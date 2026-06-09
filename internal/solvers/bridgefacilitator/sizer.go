package bridgefacilitator

import (
	"math/big"
)

// sizeInputs are the bounds that constrain how much principal the bot may offer for one Request.
type sizeInputs struct {
	perRequestMax   *big.Int // curator per-Request cap
	fundable        *big.Int // remaining room under the delegator's per-adapter cap (chain read)
	amountRequested *big.Int // auction ask; nil if unknown
	sleeveMax       *big.Int // curator total-sleeve cap
	outstanding     *big.Int // live sleeve exposure (sum of open principals)

	openCount     int
	maxConcurrent int
}

// sizeOffer returns the principal to offer and whether to bid at all. It bids only when within
// concurrency + sleeve headroom, then caps the amount to the binding minimum of every limit.
// Committing more than `fundable` would make the just-in-time allocation inside the consume
// callback revert, so it is a hard cap. Request authorization is enforced on-chain by the 3F
// whitelist at consume time, so the bot applies only its own risk caps here.
func sizeOffer(in sizeInputs) (*big.Int, bool) {
	if in.openCount >= in.maxConcurrent {
		return nil, false
	}
	sleeveRoom := new(big.Int).Sub(in.sleeveMax, in.outstanding)
	if sleeveRoom.Sign() <= 0 {
		return nil, false
	}

	amount := minBig(in.perRequestMax, in.fundable)
	amount = minBig(amount, sleeveRoom)
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
