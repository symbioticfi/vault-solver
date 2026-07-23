package defaultstrategy

import (
	"math/big"
	"sort"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

type fillSolution struct {
	allocations  []fillAllocation
	gasAmount    *big.Int
	maxAmountOut *big.Int
}

func (solution *fillSolution) buildRoutes(requiredAmountOut *big.Int) []types.FillRoute {
	if solution == nil || requiredAmountOut == nil || requiredAmountOut.Sign() <= 0 ||
		requiredAmountOut.Cmp(solution.maxAmountOut) > 0 {
		return nil
	}
	minimumTotal := new(big.Int).Add(requiredAmountOut, solution.gasAmount)
	targets := make([]*big.Int, len(solution.allocations))
	for index := range solution.allocations {
		targets[index] = solution.allocations[index].targetOutput
	}
	minimums := distributeMinimums(targets, minimumTotal)
	if minimums == nil {
		return nil
	}
	routes := make([]types.FillRoute, len(solution.allocations))
	for index, leg := range solution.allocations {
		routes[index] = types.FillRoute{
			RouteID:           leg.candidate.quote.ID,
			CapacityID:        liquidlane.RouteCapacityID(leg.candidate.quote.Route),
			Adapter:           leg.candidate.quote.Adapter,
			AmountIn:          liquidlane.CloneBig(leg.amountIn),
			ExpectedAmountOut: liquidlane.CloneBig(leg.targetOutput),
			MinAmountOut:      minimums[index],
			ReservedAmountOut: liquidlane.CloneBig(leg.reservedOutput),
			DiscountID:        liquidlane.CloneHash(leg.candidate.quote.DiscountID),
		}
	}
	return routes
}

func (s *Strategy) solveGreedyFill(
	input types.FillInput,
	validAfter time.Time,
	maxRoutes int,
) (*fillSolution, error) {
	candidates, err := s.buildFillCandidates(input, validAfter)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	allocation := s.greedyFillAllocation(
		candidates,
		input.AmountIn,
		min(maxRoutes, len(candidates)),
	)
	if len(allocation) == 0 {
		return nil, nil
	}
	pricing, err := newGasPricing(
		input.MaxFeePerGas,
		input.TokenOut,
		input.GasPrices,
		input.GasSnapshot,
		s.cfg.InventoryReserveBps,
	)
	if err != nil {
		return nil, err
	}
	targetTotal := new(big.Int)
	legs := make([]gasLeg, 0, len(allocation))
	for index, leg := range allocation {
		allocation[index].targetOutput = new(big.Int).Sub(
			leg.executableOutput,
			applyBpsUp(leg.executableOutput, s.cfg.PriceBufferBps),
		)
		if allocation[index].targetOutput.Sign() <= 0 {
			return nil, nil
		}
		targetTotal.Add(targetTotal, allocation[index].targetOutput)
		legs = append(legs, gasLeg{
			route:     leg.candidate.quote.Route,
			amountOut: leg.executableOutput,
			private:   leg.candidate.quote.DiscountID != nil,
		})
	}
	gasAmount := pricing.cost(legs)
	maxAmountOut := new(big.Int).Sub(targetTotal, gasAmount)
	if maxAmountOut.Sign() <= 0 {
		return nil, nil
	}
	return &fillSolution{
		allocations: allocation, gasAmount: gasAmount, maxAmountOut: maxAmountOut,
	}, nil
}

type fillCandidate struct {
	quote    liquidlane.FillQuote
	capacity *big.Int
	maxInput *big.Int
}

type fillRoute struct {
	id           liquidlane.RouteID
	alternatives []fillCandidate
}

type fillAllocation struct {
	candidate        fillCandidate
	amountIn         *big.Int
	executableOutput *big.Int
	reservedOutput   *big.Int
	targetOutput     *big.Int
}

func (candidate fillCandidate) id() liquidlane.CandidateID {
	return liquidlane.NewCandidateID(candidate.quote.Route, candidate.quote.DiscountID)
}

func (s *Strategy) buildFillCandidates(input types.FillInput, validAfter time.Time) ([]fillCandidate, error) {
	seen := make(map[liquidlane.CandidateID]bool, len(input.Quotes))
	candidates := make([]fillCandidate, 0, len(input.Quotes))
	for _, quote := range input.Quotes {
		if quote.TokenIn != input.TokenIn || quote.TokenOut != input.TokenOut {
			continue
		}
		if !quote.ValidUntil.IsZero() && !quote.ValidUntil.After(validAfter) {
			continue
		}
		if quote.AmountIn == nil || quote.AmountIn.Cmp(input.AmountIn) != 0 {
			return nil, errors.Errorf("fill quote %s amountIn does not match order", quote.ID)
		}
		if quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0 ||
			quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
			continue
		}
		capacityID := liquidlane.RouteCapacityID(quote.Route)
		capacity := s.availableCapacity(quote.MaxAssets)
		if reserved := input.Reservations[capacityID]; reserved != nil && reserved.Sign() > 0 {
			capacity.Sub(capacity, reserved)
		}
		if capacity.Sign() <= 0 {
			continue
		}
		candidate := fillCandidate{quote: quote, capacity: capacity}
		candidate.maxInput = s.maxInputWithinCapacity(
			candidate, input.AmountIn, capacity,
		)
		candidateID := candidate.id()
		if candidate.maxInput.Sign() <= 0 || seen[candidateID] {
			continue
		}
		seen[candidateID] = true
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (s *Strategy) greedyFillAllocation(
	candidates []fillCandidate,
	amountIn *big.Int,
	maxRoutes int,
) []fillAllocation {
	routes := buildFillRoutes(candidates)
	capacityLimits := fillCapacityLimits(candidates)
	capacityUsed := make(map[liquidlane.CapacityID]*big.Int, len(capacityLimits))
	usedRoutes := make(map[liquidlane.RouteID]bool, maxRoutes)
	remaining := liquidlane.CloneBig(amountIn)
	allocation := make([]fillAllocation, 0, maxRoutes)

	for remaining.Sign() > 0 && len(allocation) < maxRoutes {
		var best *fillAllocation
		lastRoute := len(allocation) == maxRoutes-1
		for _, route := range routes {
			if usedRoutes[route.id] {
				continue
			}
			choice := s.fillRouteChoice(
				route, remaining, capacityLimits, capacityUsed,
			)
			if choice != nil && lastRoute && choice.amountIn.Cmp(remaining) < 0 {
				continue
			}
			if choice != nil && (best == nil || fillAllocationBetter(*choice, *best)) {
				best = choice
			}
		}
		if best == nil {
			break
		}
		allocation = append(allocation, *best)
		usedRoutes[best.candidate.quote.ID] = true
		capacityID := liquidlane.RouteCapacityID(best.candidate.quote.Route)
		if capacityUsed[capacityID] == nil {
			capacityUsed[capacityID] = new(big.Int)
		}
		capacityUsed[capacityID].Add(capacityUsed[capacityID], best.reservedOutput)
		remaining.Sub(remaining, best.amountIn)
	}
	if remaining.Sign() > 0 {
		return nil
	}
	return allocation
}

func buildFillRoutes(candidates []fillCandidate) []fillRoute {
	byRoute := make(map[liquidlane.RouteID][]fillCandidate)
	for _, candidate := range candidates {
		byRoute[candidate.quote.ID] = append(byRoute[candidate.quote.ID], candidate)
	}
	routes := make([]fillRoute, 0, len(byRoute))
	for routeID, alternatives := range byRoute {
		routes = append(routes, fillRoute{id: routeID, alternatives: bestFillAlternatives(alternatives)})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].id < routes[j].id })
	return routes
}

func bestFillAlternatives(candidates []fillCandidate) []fillCandidate {
	var direct, private fillCandidate
	var hasDirect, hasPrivate bool
	for _, candidate := range candidates {
		if candidate.quote.DiscountID == nil {
			if !hasDirect || fillCandidateBetter(candidate, direct) {
				direct, hasDirect = candidate, true
			}
		} else if !hasPrivate || fillCandidateBetter(candidate, private) {
			private, hasPrivate = candidate, true
		}
	}
	alternatives := make([]fillCandidate, 0, 2)
	if hasDirect {
		alternatives = append(alternatives, direct)
	}
	if hasPrivate {
		alternatives = append(alternatives, private)
	}
	return alternatives
}

func fillCandidateBetter(left, right fillCandidate) bool {
	if comparison := compareFillRate(left.quote, right.quote); comparison != 0 {
		return comparison > 0
	}
	if comparison := left.maxInput.Cmp(right.maxInput); comparison != 0 {
		return comparison > 0
	}
	leftDirect := left.quote.DiscountID == nil
	rightDirect := right.quote.DiscountID == nil
	if leftDirect != rightDirect {
		return leftDirect
	}
	return left.id() < right.id()
}

func fillAllocationBetter(left, right fillAllocation) bool {
	if comparison := compareFillRate(left.candidate.quote, right.candidate.quote); comparison != 0 {
		return comparison > 0
	}
	if comparison := left.amountIn.Cmp(right.amountIn); comparison != 0 {
		return comparison > 0
	}
	leftDirect := left.candidate.quote.DiscountID == nil
	rightDirect := right.candidate.quote.DiscountID == nil
	if leftDirect != rightDirect {
		return leftDirect
	}
	return left.candidate.id() < right.candidate.id()
}

func compareFillRate(left, right liquidlane.FillQuote) int {
	leftRate := new(big.Int).Mul(left.MaxAmountOut, right.AmountIn)
	rightRate := new(big.Int).Mul(right.MaxAmountOut, left.AmountIn)
	return leftRate.Cmp(rightRate)
}

func fillCapacityLimits(candidates []fillCandidate) map[liquidlane.CapacityID]*big.Int {
	limits := make(map[liquidlane.CapacityID]*big.Int)
	for _, candidate := range candidates {
		capacityID := liquidlane.RouteCapacityID(candidate.quote.Route)
		if limit := limits[capacityID]; limit == nil || candidate.capacity.Cmp(limit) > 0 {
			limits[capacityID] = liquidlane.CloneBig(candidate.capacity)
		}
	}
	return limits
}

func (s *Strategy) fillRouteChoice(
	route fillRoute,
	remaining *big.Int,
	capacityLimits map[liquidlane.CapacityID]*big.Int,
	capacityUsed map[liquidlane.CapacityID]*big.Int,
) *fillAllocation {
	capacityID := liquidlane.RouteCapacityID(route.alternatives[0].quote.Route)
	capacityLeft := liquidlane.CloneBig(capacityLimits[capacityID])
	if used := capacityUsed[capacityID]; used != nil {
		capacityLeft.Sub(capacityLeft, used)
	}
	if capacityLeft.Sign() <= 0 {
		return nil
	}

	available := make([]*big.Int, len(route.alternatives))
	legAmount := new(big.Int)
	for index, candidate := range route.alternatives {
		candidateCapacity := minBig(capacityLeft, candidate.capacity)
		amount := s.maxInputWithinCapacity(candidate, remaining, candidateCapacity)
		if amount.Cmp(candidate.maxInput) > 0 {
			amount.Set(candidate.maxInput)
		}
		available[index] = amount
		if amount.Cmp(legAmount) > 0 {
			legAmount.Set(amount)
		}
	}
	if legAmount.Sign() <= 0 {
		return nil
	}

	var best *fillCandidate
	for index, candidate := range route.alternatives {
		if available[index].Cmp(legAmount) < 0 {
			continue
		}
		if best == nil || fillCandidateBetter(candidate, *best) {
			selected := candidate
			best = &selected
		}
	}
	if best == nil {
		return nil
	}
	return &fillAllocation{
		candidate:        *best,
		amountIn:         legAmount,
		executableOutput: scaledFillOutput(best.quote, legAmount),
		reservedOutput:   s.reservedCapacityOutput(*best, legAmount),
	}
}

func (s *Strategy) maxInputWithinCapacity(
	candidate fillCandidate,
	inputLimit *big.Int,
	capacity *big.Int,
) *big.Int {
	quote := candidate.quote
	if inputLimit == nil || inputLimit.Sign() <= 0 || capacity == nil || capacity.Sign() <= 0 ||
		quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 ||
		quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
		return new(big.Int)
	}
	precision := big.NewInt(bpsDenominator)
	buffer := big.NewInt(int64(s.cfg.PriceBufferBps))
	maxOutput := new(big.Int)
	if quote.DiscountID != nil {
		maxOutput.Mul(capacity, precision)
		maxOutput.Div(maxOutput, new(big.Int).Add(precision, buffer))
	} else {
		// reserved = floor(output * (1 - buffer)); invert the floor exactly.
		maxOutput.Add(capacity, big.NewInt(1))
		maxOutput.Mul(maxOutput, precision)
		maxOutput.Sub(maxOutput, big.NewInt(1))
		maxOutput.Div(maxOutput, new(big.Int).Sub(precision, buffer))
	}
	maxInput := new(big.Int).Add(maxOutput, big.NewInt(1))
	maxInput.Mul(maxInput, quote.AmountIn)
	maxInput.Sub(maxInput, big.NewInt(1))
	maxInput.Div(maxInput, quote.MaxAmountOut)
	if maxInput.Cmp(inputLimit) > 0 {
		maxInput.Set(inputLimit)
	}
	return maxInput
}

func (s *Strategy) reservedCapacityOutput(candidate fillCandidate, amountIn *big.Int) *big.Int {
	amountOut := scaledFillOutput(candidate.quote, amountIn)
	buffer := applyBpsUp(amountOut, s.cfg.PriceBufferBps)
	if candidate.quote.DiscountID != nil {
		return amountOut.Add(amountOut, buffer)
	}
	return amountOut.Sub(amountOut, buffer)
}

func distributeMinimums(targets []*big.Int, total *big.Int) []*big.Int {
	if len(targets) == 0 || total == nil || total.Sign() <= 0 ||
		total.Cmp(big.NewInt(int64(len(targets)))) < 0 {
		return nil
	}
	capacity := new(big.Int)
	for _, target := range targets {
		if target == nil || target.Sign() <= 0 {
			return nil
		}
		capacity.Add(capacity, target)
	}
	if total.Cmp(capacity) > 0 {
		return nil
	}
	remaining := new(big.Int).Sub(total, big.NewInt(int64(len(targets))))
	remainingCapacity := new(big.Int).Sub(capacity, big.NewInt(int64(len(targets))))
	minimums := make([]*big.Int, len(targets))
	for index, target := range targets {
		available := new(big.Int).Sub(target, big.NewInt(1))
		allocation := new(big.Int)
		if index == len(targets)-1 {
			allocation.Set(remaining)
		} else if remainingCapacity.Sign() > 0 {
			allocation.Mul(remaining, available)
			allocation.Div(allocation, remainingCapacity)
		}
		if allocation.Cmp(available) > 0 {
			return nil
		}
		minimums[index] = allocation.Add(allocation, big.NewInt(1))
		remaining.Sub(remaining, new(big.Int).Sub(minimums[index], big.NewInt(1)))
		remainingCapacity.Sub(remainingCapacity, available)
	}
	if remaining.Sign() != 0 {
		return nil
	}
	return minimums
}

func scaledFillOutput(quote liquidlane.FillQuote, amountIn *big.Int) *big.Int {
	if quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 ||
		quote.MaxAmountOut == nil || amountIn == nil {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(quote.MaxAmountOut, amountIn), quote.AmountIn)
}
