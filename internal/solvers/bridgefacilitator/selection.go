package bridgefacilitator

import "math/big"

// adapterSizing pairs an adapter candidate with the principal it can fund for one auction.
type adapterSizing struct {
	target    Target
	principal *big.Int
}

// selectBestAdapter returns the candidate that can fund the largest principal (which, at the auction's
// fixed rate, maximizes expected return). Ties keep the earlier candidate; ok is false when empty.
func selectBestAdapter(candidates []adapterSizing) (adapterSizing, bool) {
	var best adapterSizing
	found := false
	for _, c := range candidates {
		if !found || c.principal.Cmp(best.principal) > 0 {
			best, found = c, true
		}
	}
	return best, found
}
