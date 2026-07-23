package defaultstrategy

import (
	"math/big"
	"sort"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func (s *Strategy) buildQuoteRanges(
	ladder []quoteRoute,
	pricing gasPricing,
) ([]types.QuoteRange, map[liquidlane.CandidateID]quoteCandidate) {
	used := make(map[liquidlane.CandidateID]quoteCandidate)
	transitions := quoteTransitionPoints(ladder)
	if len(transitions) == 0 {
		return nil, used
	}
	breakpoints := quoteBreakpoints(transitions[len(transitions)-1], s.minAmount, s.rangeCount)
	ranges := make([]types.QuoteRange, 0, len(breakpoints))
	lower := new(big.Int).Set(s.minAmount)
	for _, upper := range breakpoints {
		if upper.Cmp(lower) < 0 {
			continue
		}
		quoteRange, candidates, ok := s.priceQuoteRange(ladder, lower, upper, transitions, pricing)
		if ok {
			ranges = append(ranges, quoteRange)
			for _, candidate := range candidates {
				used[candidate.id()] = candidate
			}
		}
		lower = new(big.Int).Add(upper, big.NewInt(1))
	}
	return ranges, used
}

func (s *Strategy) priceQuoteRange(
	ladder []quoteRoute,
	lower *big.Int,
	upper *big.Int,
	transitions []*big.Int,
	pricing gasPricing,
) (types.QuoteRange, []quoteCandidate, bool) {
	amounts := quoteRangePoints(lower, upper, transitions)
	quotes := make([]exactInputQuote, len(amounts))
	gasCost := new(big.Int)
	for index, amount := range amounts {
		quote, ok := solveExactInputQuote(ladder, amount, pricing)
		if !ok {
			return types.QuoteRange{}, nil, false
		}
		quotes[index] = quote
		if quote.gasCost.Cmp(gasCost) > 0 {
			gasCost.Set(quote.gasCost)
		}
	}

	var rate *big.Int
	candidates := make([]quoteCandidate, 0, len(amounts)*len(ladder))
	for index, quote := range quotes {
		decimals := quote.candidates[0]
		pointRate := s.guaranteedQuoteRate(
			quote.grossAmountOut, amounts[index], gasCost,
			decimals.TokenInDecimals, decimals.TokenOutDecimals,
		)
		if rate == nil || pointRate.Cmp(rate) < 0 {
			rate = pointRate
		}
		candidates = append(candidates, quote.candidates...)
	}
	if rate == nil || rate.Sign() <= 0 {
		return types.QuoteRange{}, nil, false
	}
	return types.QuoteRange{
		MinAmount: new(big.Int).Set(lower), MaxAmount: new(big.Int).Set(upper),
		Quote: fixedPointDecimal(rate, rateScaleDigits),
	}, candidates, true
}

func quoteBreakpoints(maximum, minimum *big.Int, targetCount int) []*big.Int {
	if maximum == nil || maximum.Cmp(minimum) < 0 || targetCount <= 0 {
		return nil
	}
	selected := map[string]*big.Int{maximum.String(): new(big.Int).Set(maximum)}
	for len(selected) < targetCount {
		points := sortedAmounts(selected)
		var bestMid, bestLow, bestHigh *big.Int
		low := new(big.Int).Set(minimum)
		for _, high := range points {
			mid := geometricMidpoint(low, high)
			if mid != nil && (bestMid == nil ||
				new(big.Int).Mul(high, bestLow).Cmp(new(big.Int).Mul(bestHigh, low)) > 0) {
				bestMid, bestLow, bestHigh = mid, new(big.Int).Set(low), new(big.Int).Set(high)
			}
			low = high
		}
		if bestMid == nil {
			break
		}
		selected[bestMid.String()] = bestMid
	}
	return sortedAmounts(selected)
}

func quoteTransitionPoints(ladder []quoteRoute) []*big.Int {
	selected := make(map[string]*big.Int)
	// A candidate can become ineligible after any subset of other routes has filled to capacity.
	for mask := 0; mask < 1<<len(ladder); mask++ {
		prefix := new(big.Int)
		for index, route := range ladder {
			if mask&(1<<index) != 0 {
				prefix.Add(prefix, route.maxInput)
			}
		}
		for index, route := range ladder {
			if mask&(1<<index) != 0 {
				continue
			}
			for _, candidate := range route.alternatives {
				point := new(big.Int).Add(prefix, candidate.maxInput)
				selected[point.String()] = point
			}
		}
		if mask == (1<<len(ladder))-1 && prefix.Sign() > 0 {
			selected[prefix.String()] = prefix
		}
	}
	return sortedAmounts(selected)
}

func quoteRangePoints(lower, upper *big.Int, transitions []*big.Int) []*big.Int {
	selected := map[string]*big.Int{
		lower.String(): new(big.Int).Set(lower),
		upper.String(): new(big.Int).Set(upper),
	}
	for _, transition := range transitions {
		if transition.Cmp(lower) >= 0 && transition.Cmp(upper) <= 0 {
			selected[transition.String()] = new(big.Int).Set(transition)
		}
		after := new(big.Int).Add(transition, big.NewInt(1))
		if after.Cmp(lower) >= 0 && after.Cmp(upper) <= 0 {
			selected[after.String()] = after
		}
	}
	return sortedAmounts(selected)
}

func sortedAmounts(amounts map[string]*big.Int) []*big.Int {
	out := make([]*big.Int, 0, len(amounts))
	for _, amount := range amounts {
		out = append(out, amount)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cmp(out[j]) < 0 })
	return out
}

func geometricMidpoint(lower, upper *big.Int) *big.Int {
	if lower.Sign() <= 0 || upper.Cmp(lower) <= 0 {
		return nil
	}
	mid := new(big.Int).Sqrt(new(big.Int).Mul(lower, upper))
	if mid.Cmp(lower) <= 0 {
		mid.Add(lower, big.NewInt(1))
	}
	if mid.Cmp(upper) >= 0 {
		return nil
	}
	return mid
}

func (s *Strategy) guaranteedQuoteRate(
	grossAmountOut *big.Int,
	amountIn *big.Int,
	cost *big.Int,
	inDecimals int,
	outDecimals int,
) *big.Int {
	available := applyBpsDown(grossAmountOut, bpsDenominator-2*s.cfg.PriceBufferBps)
	available.Sub(available, cost)
	if available.Sign() <= 0 {
		return new(big.Int)
	}
	return liquidlane.RateForAmountOut(available, amountIn, inDecimals, outDecimals)
}

func quoteExpiry(deadline time.Time, buffer time.Duration, used map[liquidlane.CandidateID]quoteCandidate) int64 {
	expiry := deadline.Unix()
	for _, candidate := range used {
		if !candidate.ValidUntil.IsZero() {
			expiry = min(expiry, candidate.ValidUntil.Add(-buffer).Unix())
		}
	}
	return expiry
}
