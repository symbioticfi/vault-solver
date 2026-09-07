package planning

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// BestRouteCandidates keeps the highest-ranked physical routes and every
// alternative that can outperform or outlive another alternative for the same
// route.
func BestRouteCandidates(candidates []liquidlane.QuoteCandidate, maxRoutes int) []liquidlane.QuoteCandidate {
	if maxRoutes <= 0 {
		return nil
	}
	sources := buildSources(candidates)
	if len(sources) > maxRoutes {
		sources = sources[:maxRoutes]
	}
	out := make([]liquidlane.QuoteCandidate, 0, len(candidates))
	for _, source := range sources {
		out = append(out, nonDominatedAlternatives(source.Alternatives)...)
	}
	return out
}

func nonDominatedAlternatives(candidates []liquidlane.QuoteCandidate) []liquidlane.QuoteCandidate {
	frontier := make([]liquidlane.QuoteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		hidden := false
		for _, kept := range frontier {
			if dominates(kept, candidate) {
				hidden = true
				break
			}
		}
		if hidden {
			continue
		}
		survivors := frontier[:0]
		for _, kept := range frontier {
			if !dominates(candidate, kept) {
				survivors = append(survivors, kept)
			}
		}
		clear(frontier[len(survivors):])
		frontier = append(survivors, candidate)
	}
	return frontier
}

func dominates(left, right liquidlane.QuoteCandidate) bool {
	if left.DiscountID != nil && right.DiscountID == nil {
		return false
	} // Private execution has additional gas.
	dimensions := [][2]*big.Int{{left.Rate, right.Rate}, {left.MaxAmountIn, right.MaxAmountIn}, {left.MaxAmountOut, right.MaxAmountOut}}
	for _, pair := range dimensions {
		if pair[0].Cmp(pair[1]) < 0 {
			return false
		}
	}
	if !left.ValidUntil.IsZero() && (right.ValidUntil.IsZero() || left.ValidUntil.Before(right.ValidUntil)) {
		return false
	}
	return better(left, right)
}
