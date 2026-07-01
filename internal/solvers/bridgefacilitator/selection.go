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
// principals sum to at most min(remaining, Σcapacity); one offer per adapter. An adapter is skipped when
// the clamped principal falls below its on-chain minAssetsPerRequest floor (the consume would revert),
// leaving that remainder for a later pass.
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
		if floor := c.off.st.minAssets; floor != nil && floor.Sign() > 0 && principal.Cmp(floor) < 0 {
			continue // uncovered remainder is below this adapter's on-chain minimum request size
		}
		offers = append(offers, adapterOffer{off: c.off, principal: principal})
		left.Sub(left, principal)
	}
	return offers
}
