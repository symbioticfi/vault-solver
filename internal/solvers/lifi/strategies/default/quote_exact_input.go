package defaultstrategy

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type exactInputQuote struct {
	grossAmountOut *big.Int
	gasCost        *big.Int
	candidates     []quoteCandidate
}

// solveExactInputQuote greedily prices one concrete input amount across physical routes.
func solveExactInputQuote(ladder []quoteRoute, amountIn *big.Int, pricing gasPricing) (exactInputQuote, bool) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return exactInputQuote{}, false
	}

	remaining := new(big.Int).Set(amountIn)
	usedRoutes := make(map[liquidlane.RouteID]bool, len(ladder))
	candidates := make([]quoteCandidate, 0, len(ladder))
	gasLegs := make([]gasLeg, 0, len(ladder))
	grossAmountOut := new(big.Int)

	for remaining.Sign() > 0 {
		candidate, legAmount, ok := bestQuoteLeg(ladder, usedRoutes, remaining)
		if !ok {
			return exactInputQuote{}, false
		}
		amountOut := liquidlane.AmountOutForRate(
			legAmount,
			candidate.MaxRate,
			candidate.TokenInDecimals,
			candidate.TokenOutDecimals,
		)
		if amountOut.Sign() <= 0 {
			return exactInputQuote{}, false
		}

		candidates = append(candidates, candidate)
		gasLegs = append(gasLegs, gasLeg{
			route: candidate.Route, amountOut: amountOut, private: candidate.DiscountID != nil,
		})
		grossAmountOut.Add(grossAmountOut, amountOut)
		usedRoutes[candidate.ID] = true
		remaining.Sub(remaining, legAmount)
	}

	return exactInputQuote{
		grossAmountOut: grossAmountOut,
		gasCost:        pricing.cost(gasLegs),
		candidates:     candidates,
	}, true
}

func bestQuoteLeg(
	ladder []quoteRoute,
	used map[liquidlane.RouteID]bool,
	remaining *big.Int,
) (quoteCandidate, *big.Int, bool) {
	var best quoteCandidate
	var bestAmount *big.Int
	found := false
	for _, route := range ladder {
		if used[route.id] {
			continue
		}
		legAmount := minBig(remaining, route.maxInput)
		for _, candidate := range route.alternatives {
			if candidate.maxInput.Cmp(legAmount) < 0 {
				continue
			}
			if !found || quoteCandidateBetter(candidate, best) {
				best = candidate
				bestAmount = legAmount
				found = true
			}
		}
	}
	return best, bestAmount, found
}
