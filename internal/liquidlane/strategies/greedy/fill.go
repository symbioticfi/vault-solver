package greedy

import (
	"math/big"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

// SolveFill selects current LiquidLane quotes, enforces shared capacity, and optionally prices execution gas.
func SolveFill(task strategies.FillTask) (*strategies.FillSolution, error) {
	if task.AmountIn == nil || task.AmountIn.Sign() <= 0 {
		return nil, errors.New("amountIn: must be positive")
	}
	if task.MaxRoutes <= 0 || len(task.Quotes) == 0 {
		task.Trace.Decline(
			"fill", "no-quotes",
			"quotes", len(task.Quotes),
			"maxRoutes", task.MaxRoutes,
		)
		return nil, nil
	}
	if task.InputPolicy != strategies.RejectUncoveredInput && task.InputPolicy != strategies.AbsorbUncoveredInput {
		return nil, errors.New("invalid uncovered input policy")
	}
	if task.PrivateCapacityBufferBps < 0 || task.PrivateCapacityBufferBps >= bpsDenominator {
		return nil, errors.New("private capacity buffer must be in [0,10000)")
	}
	if task.InventoryReserveBps < 0 || task.InventoryReserveBps >= bpsDenominator {
		return nil, errors.Errorf("inventoryReserveBps: must be in [0,%d)", bpsDenominator)
	}
	candidates, err := buildFillCandidates(task)
	if err != nil || len(candidates) == 0 {
		if err == nil {
			task.Trace.Decline(
				"fill", "no-candidates",
				"quotes", len(task.Quotes),
				"reservations", len(task.Reservations),
				"validAfter", task.ValidAfter,
			)
		}
		return nil, err
	}
	allocation := greedyFillAllocation(
		candidates,
		task.AmountIn,
		min(task.MaxRoutes, len(candidates)),
		task.PrivateCapacityBufferBps,
		task.InputPolicy,
	)
	if len(allocation) == 0 {
		task.Trace.Decline(
			"fill", insufficientCapacityReason,
			"amountIn", task.AmountIn.String(),
			"candidates", len(candidates),
			"maxRoutes", task.MaxRoutes,
		)
		return nil, nil
	}
	targetTotal := new(big.Int)
	legs := make([]strategies.GasLeg, 0, len(allocation))
	for _, leg := range allocation {
		// The quote-time price margin can be consumed while honoring the awarded output.
		targetTotal.Add(targetTotal, leg.executableOutput)
		legs = append(legs, strategies.GasLeg{
			Route:     leg.candidate.quote.Route,
			AmountOut: leg.executableOutput,
			Private:   leg.candidate.quote.DiscountID != nil,
		})
	}
	gasAmount := new(big.Int)
	if task.GasPricing != nil {
		gasAmount = task.GasPricing.Cost(legs)
	}
	maxAmountOut := new(big.Int).Sub(targetTotal, gasAmount)
	if maxAmountOut.Sign() <= 0 {
		task.Trace.Decline(
			"fill", "gas-exceeds-output",
			"targetAmountOut", targetTotal.String(),
			"gasCost", gasAmount.String(),
			"maxAmountOut", maxAmountOut.String(),
		)
		return nil, nil
	}
	task.Trace.Log(
		"liquidlane fill selected",
		"amountIn", task.AmountIn.String(),
		"targetAmountOut", targetTotal.String(),
		"gasCost", gasAmount.String(),
		"maxAmountOut", maxAmountOut.String(),
		"routes", len(allocation),
	)
	routes := make([]strategies.FillRoute, len(allocation))
	for index, leg := range allocation {
		routes[index] = strategies.FillRoute{
			CandidateID: leg.candidate.id(), RouteID: leg.candidate.quote.ID,
			CapacityID: liquidlane.RouteCapacityID(leg.candidate.quote.Route),
			Adapter:    leg.candidate.quote.Adapter, AmountIn: leg.amountIn,
			ExpectedAmountOut: leg.executableOutput, ReservedAmountOut: leg.reservedOutput,
			DiscountID: leg.candidate.quote.DiscountID,
		}
	}
	return strategies.NewFillSolution(routes, gasAmount), nil
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
}

func (candidate fillCandidate) id() liquidlane.CandidateID {
	return liquidlane.NewCandidateID(candidate.quote.Route, candidate.quote.DiscountID)
}

func buildFillCandidates(task strategies.FillTask) ([]fillCandidate, error) {
	seen := make(map[liquidlane.CandidateID]bool, len(task.Quotes))
	candidates := make([]fillCandidate, 0, len(task.Quotes))
	for _, quote := range task.Quotes {
		if quote.TokenIn != task.TokenIn || quote.TokenOut != task.TokenOut {
			continue
		}
		if !quote.ValidUntil.IsZero() && !quote.ValidUntil.After(task.ValidAfter) {
			continue
		}
		if quote.AmountIn == nil || quote.AmountIn.Cmp(task.AmountIn) != 0 {
			return nil, errors.Errorf("fill quote %s amountIn does not match order", quote.ID)
		}
		if quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0 ||
			quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
			continue
		}
		capacityID := liquidlane.RouteCapacityID(quote.Route)
		capacity := AvailableCapacity(quote.MaxAssets, task.InventoryReserveBps)
		if reserved := task.Reservations[capacityID]; reserved != nil && reserved.Sign() > 0 {
			capacity.Sub(capacity, reserved)
		}
		if capacity.Sign() <= 0 {
			continue
		}
		candidate := fillCandidate{quote: quote, capacity: capacity}
		candidate.maxInput = maxInputWithinCapacity(
			candidate, task.AmountIn, capacity, task.PrivateCapacityBufferBps,
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

func greedyFillAllocation(
	candidates []fillCandidate,
	amountIn *big.Int,
	maxRoutes int,
	privateBufferBps int,
	inputPolicy strategies.UncoveredInputPolicy,
) []fillAllocation {
	routes := buildFillRoutes(candidates)
	capacityLimits := fillCapacityLimits(candidates)
	capacityUsed := make(map[liquidlane.CapacityID]*big.Int, len(capacityLimits))
	usedRoutes := make(map[liquidlane.RouteID]bool, maxRoutes)
	remaining := liquidlane.CloneBig(amountIn)
	allocation := make([]fillAllocation, 0, maxRoutes)

	for remaining.Sign() > 0 && len(allocation) < maxRoutes {
		var best *fillAllocation
		lastRoute := inputPolicy == strategies.RejectUncoveredInput && len(allocation) == maxRoutes-1
		for _, route := range routes {
			if usedRoutes[route.id] {
				continue
			}
			choice := fillRouteChoice(
				route, remaining, capacityLimits, capacityUsed, privateBufferBps,
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
		if inputPolicy == strategies.RejectUncoveredInput || len(allocation) == 0 {
			return nil
		}
		// Discount calldata has no output cap, so it must never absorb excess input.
		direct := -1
		for i := range allocation {
			if allocation[i].candidate.quote.DiscountID == nil {
				direct = i
			}
		}
		if direct < 0 {
			return nil
		}
		allocation[direct].amountIn.Add(allocation[direct].amountIn, remaining)
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
		routes = append(routes, fillRoute{id: routeID, alternatives: alternatives})
	}
	return routes
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

func fillRouteChoice(
	route fillRoute,
	remaining *big.Int,
	capacityLimits map[liquidlane.CapacityID]*big.Int,
	capacityUsed map[liquidlane.CapacityID]*big.Int,
	privateBufferBps int,
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
		amount := maxInputWithinCapacity(candidate, remaining, candidateCapacity, privateBufferBps)
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
		reservedOutput:   reservedFillOutput(best.quote, legAmount, privateBufferBps),
	}
}

func maxInputWithinCapacity(
	candidate fillCandidate,
	inputLimit *big.Int,
	capacity *big.Int,
	privateBufferBps int,
) *big.Int {
	quote := candidate.quote
	if inputLimit == nil || inputLimit.Sign() <= 0 || capacity == nil || capacity.Sign() <= 0 ||
		quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 ||
		quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
		return new(big.Int)
	}
	maxOutput := liquidlane.CloneBig(capacity)
	if quote.DiscountID != nil {
		maxOutput.Mul(capacity, big.NewInt(bpsDenominator))
		maxOutput.Div(maxOutput, big.NewInt(int64(bpsDenominator+privateBufferBps)))
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

func scaledFillOutput(quote liquidlane.FillQuote, amountIn *big.Int) *big.Int {
	if quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 ||
		quote.MaxAmountOut == nil || amountIn == nil {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(quote.MaxAmountOut, amountIn), quote.AmountIn)
}

func reservedFillOutput(quote liquidlane.FillQuote, amountIn *big.Int, privateBufferBps int) *big.Int {
	output := scaledFillOutput(quote, amountIn)
	if quote.DiscountID != nil {
		output.Add(output, applyBpsUp(output, privateBufferBps))
	}
	return output
}
