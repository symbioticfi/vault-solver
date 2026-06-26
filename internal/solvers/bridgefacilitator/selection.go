package bridgefacilitator

import (
	"math/big"
	"sort"
)

// adapterSizing is one eligible adapter and the maximum principal it can currently fund for an auction
// (its exposure/liquidity capacity, independent of the auction's requested amount).
type adapterSizing struct {
	off      *adapterOffering
	capacity *big.Int
}

// adapterOffer is a selected offer: the adapter and the principal it will be asked to fund.
type adapterOffer struct {
	off       *adapterOffering
	principal *big.Int
}

// selectOffers chooses, in one shot, the offers that cover `remaining` of an auction. It ranks the
// candidates by capacity (largest first) and assigns each the principal it will offer —
// min(capacity, still-uncovered) — until the amount is covered or candidates run out. The returned
// principals sum to min(remaining, Σcapacity); one offer per adapter. A future min-amount exposure
// param would be enforced here.
func selectOffers(candidates []adapterSizing, remaining *big.Int) []adapterOffer {
	ranked := append([]adapterSizing(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].capacity.Cmp(ranked[j].capacity) > 0
	})

	left := new(big.Int).Set(remaining)
	offers := make([]adapterOffer, 0, len(ranked))
	for _, c := range ranked {
		if left.Sign() <= 0 {
			break
		}
		principal := new(big.Int).Set(c.capacity)
		if principal.Cmp(left) > 0 {
			principal.Set(left)
		}
		offers = append(offers, adapterOffer{off: c.off, principal: principal})
		left.Sub(left, principal)
	}
	return offers
}
