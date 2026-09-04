package policy

import "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"

func filterReservedPositions(scored []scoredLeg, claims []decision.PositionClaim) []scoredLeg {
	if len(claims) == 0 {
		return scored
	}
	reserved := make(map[decision.PositionClaim]struct{}, len(claims))
	for _, claim := range claims {
		reserved[claim] = struct{}{}
	}
	filtered := scored[:0]
	for _, leg := range scored {
		claim := decision.PositionClaim{MarketID: leg.MarketId, Borrower: leg.Borrower}
		if _, found := reserved[claim]; !found {
			filtered = append(filtered, leg)
		}
	}
	return filtered
}
