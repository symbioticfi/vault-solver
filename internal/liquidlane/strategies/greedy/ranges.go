package greedy

import (
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
		out = append(out, nonDominatedAlternatives(source.alternatives)...)
	}
	return out
}

func nonDominatedAlternatives(candidates []liquidlane.QuoteCandidate) []liquidlane.QuoteCandidate {
	out := make([]liquidlane.QuoteCandidate, 0, len(candidates))
	for index, candidate := range candidates {
		dominated := false
		for otherIndex, other := range candidates {
			if index != otherIndex && dominates(other, candidate) {
				dominated = true
				break
			}
		}
		if !dominated {
			out = append(out, candidate)
		}
	}
	return out
}

func dominates(left, right liquidlane.QuoteCandidate) bool {
	// A private alternative costs more gas than a direct one, so raw rate and
	// capacity alone cannot prove that it dominates the direct path.
	if left.DiscountID != nil && right.DiscountID == nil {
		return false
	}
	if left.Rate.Cmp(right.Rate) < 0 ||
		left.MaxAmountIn.Cmp(right.MaxAmountIn) < 0 ||
		left.MaxAmountOut.Cmp(right.MaxAmountOut) < 0 {
		return false
	}
	if right.ValidUntil.IsZero() {
		if !left.ValidUntil.IsZero() {
			return false
		}
	} else if !left.ValidUntil.IsZero() && left.ValidUntil.Before(right.ValidUntil) {
		return false
	}
	return better(left, right)
}
