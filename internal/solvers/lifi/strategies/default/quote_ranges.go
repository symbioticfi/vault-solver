package defaultstrategy

import (
	"math/big"
	"sort"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func (s *Strategy) buildQuoteRanges(
	candidates []liquidlane.QuoteCandidate,
	maxRoutes int,
	pricing liquidstrategies.GasPricing,
) ([]types.QuoteRange, map[liquidlane.CandidateID]liquidlane.QuoteCandidate, error) {
	candidates = liquidgreedy.BestRouteCandidates(candidates, maxRoutes)
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	maximum, routeCount, privateRouteCount := quoteBounds(candidates)
	if maximum.Sign() <= 0 {
		return nil, nil, nil
	}
	maxGasCost := pricing.MaxCost(routeCount, privateRouteCount)
	breakpoints := quoteBreakpoints(maximum, s.minAmount, s.rangeCount)
	used := make(map[liquidlane.CandidateID]liquidlane.QuoteCandidate)
	ranges := make([]types.QuoteRange, 0, len(breakpoints))
	lower := new(big.Int).Set(s.minAmount)
	for _, upper := range breakpoints {
		if upper.Cmp(lower) < 0 {
			continue
		}
		quoteRange, err := s.priceQuoteRange(
			candidates, maxRoutes, lower, upper, maxGasCost, routeCount, pricing,
		)
		if err != nil {
			return nil, nil, err
		}
		if quoteRange != nil {
			ranges = append(ranges, *quoteRange)
		}
		lower = new(big.Int).Add(upper, big.NewInt(1))
	}
	if len(ranges) > 0 {
		for _, candidate := range candidates {
			used[candidate.ID] = candidate
		}
	}
	return ranges, used, nil
}

func (s *Strategy) priceQuoteRange(
	candidates []liquidlane.QuoteCandidate,
	maxRoutes int,
	lower *big.Int,
	upper *big.Int,
	maxGasCost *big.Int,
	routeCount int,
	pricing liquidstrategies.GasPricing,
) (*types.QuoteRange, error) {
	quoteAt := func(amount *big.Int) (*liquidgreedy.QuoteSolution, error) {
		return liquidgreedy.SolveQuote(liquidgreedy.QuoteTask{
			ExactInput:      amount,
			Candidates:      candidates,
			MaxRoutes:       maxRoutes,
			MinInput:        s.minAmount,
			OutputBufferBps: 2 * s.cfg.PriceBufferBps,
			InputPolicy:     liquidgreedy.RejectUncoveredInput,
			GasPricing:      &pricing,
		})
	}
	lowerQuote, err := quoteAt(lower)
	if err != nil || lowerQuote == nil {
		return nil, err
	}
	upperQuote, err := quoteAt(upper)
	if err != nil || upperQuote == nil {
		return nil, err
	}

	inDecimals := candidates[0].Route.TokenInDecimals
	outDecimals := candidates[0].Route.TokenOutDecimals
	rate := liquidlane.RateForAmountOut(lowerQuote.AmountOut, lower, inDecimals, outDecimals)
	upperRate := liquidlane.RateForAmountOut(upperQuote.AmountOut, upper, inDecimals, outDecimals)
	if upperRate.Cmp(rate) < 0 {
		rate = upperRate
	}
	floorRate := candidateFloorRate(
		candidates,
		lower,
		upper,
		maxGasCost,
		routeCount,
		2*s.cfg.PriceBufferBps,
		inDecimals,
		outDecimals,
	)
	if floorRate.Cmp(rate) < 0 {
		rate = floorRate
	}
	if rate.Sign() <= 0 {
		return nil, nil
	}
	return &types.QuoteRange{
		MinAmount: new(big.Int).Set(lower),
		MaxAmount: new(big.Int).Set(upper),
		Quote:     fixedPointDecimal(rate, rateScaleDigits),
	}, nil
}

func quoteBounds(
	candidates []liquidlane.QuoteCandidate,
) (maximum *big.Int, routeCount int, privateRouteCount int) {
	type route struct {
		maxInput *big.Int
		private  bool
	}
	byRoute := make(map[liquidlane.RouteID]route)
	for _, candidate := range candidates {
		item := byRoute[candidate.Route.ID]
		if item.maxInput == nil || candidate.MaxAmountIn.Cmp(item.maxInput) > 0 {
			item.maxInput = candidate.MaxAmountIn
		}
		item.private = item.private || candidate.DiscountID != nil
		byRoute[candidate.Route.ID] = item
	}
	total := new(big.Int)
	private := 0
	for _, item := range byRoute {
		total.Add(total, item.maxInput)
		if item.private {
			private++
		}
	}
	return total, len(byRoute), private
}

func candidateFloorRate(
	candidates []liquidlane.QuoteCandidate,
	minimumInput *big.Int,
	maximumInput *big.Int,
	gasCost *big.Int,
	routeCount int,
	outputBufferBps int,
	inDecimals int,
	outDecimals int,
) *big.Int {
	if len(candidates) == 0 || routeCount <= 0 ||
		minimumInput == nil || minimumInput.Sign() <= 0 ||
		maximumInput == nil || maximumInput.Cmp(minimumInput) < 0 {
		return new(big.Int)
	}
	byRoute := make(map[liquidlane.RouteID][]liquidlane.QuoteCandidate)
	for _, candidate := range candidates {
		byRoute[candidate.Route.ID] = append(byRoute[candidate.Route.ID], candidate)
	}
	var rate *big.Int
	for _, alternatives := range byRoute {
		maxInput := new(big.Int)
		for _, candidate := range alternatives {
			if candidate.MaxAmountIn.Cmp(maxInput) > 0 {
				maxInput.Set(candidate.MaxAmountIn)
			}
		}
		legLimit := minBig(maximumInput, maxInput)
		var best *big.Int
		for _, candidate := range alternatives {
			if candidate.MaxAmountIn.Cmp(legLimit) >= 0 && (best == nil || candidate.Rate.Cmp(best) > 0) {
				best = candidate.Rate
			}
		}
		if best != nil && (rate == nil || best.Cmp(rate) < 0) {
			rate = liquidlane.CloneBig(best)
		}
	}
	if rate == nil || rate.Sign() <= 0 {
		return new(big.Int)
	}
	rate.Mul(rate, big.NewInt(int64(bpsDenominator-outputBufferBps)))
	rate.Div(rate, big.NewInt(bpsDenominator))

	// Every complete greedy plan uses at most routeCount candidates whose
	// effective rates are no lower than rate. Summing their floors loses at
	// most routeCount-1 output units; a non-zero output buffer can lose one more.
	loss := new(big.Int)
	if gasCost != nil {
		loss.Set(gasCost)
	}
	loss.Add(loss, big.NewInt(int64(routeCount-1)))
	if outputBufferBps > 0 {
		loss.Add(loss, big.NewInt(1))
	}
	if loss.Sign() == 0 {
		return rate
	}
	lossRate := liquidlane.RateForAmountOut(loss, minimumInput, inDecimals, outDecimals)
	lossRate.Add(lossRate, big.NewInt(1))
	rate.Sub(rate, lossRate)
	if rate.Sign() <= 0 {
		return new(big.Int)
	}
	return rate
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

func quoteExpiry(
	deadline time.Time,
	buffer time.Duration,
	used map[liquidlane.CandidateID]liquidlane.QuoteCandidate,
) int64 {
	expiry := deadline.Unix()
	for _, candidate := range used {
		if !candidate.ValidUntil.IsZero() {
			expiry = min(expiry, candidate.ValidUntil.Add(-buffer).Unix())
		}
	}
	return expiry
}
