package greedy

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

// FillTask contains the protocol-neutral facts needed to route an exact-input fill.
type FillTask struct {
	TokenIn  common.Address
	TokenOut common.Address
	AmountIn *big.Int

	Quotes       []liquidlane.FillQuote
	Reservations liquidlane.CapacityReservations
	ValidAfter   time.Time

	MaxRoutes           int
	PriceBufferBps      int
	InventoryReserveBps int
	InputPolicy         UncoveredInputPolicy
	GasPricing          *liquidstrategies.GasPricing
	Trace               liquidstrategies.DecisionTrace
}

// FillSolution is a routed fill before protocol-specific output requirements are applied.
type FillSolution struct {
	routes       []fillAllocation
	gasAmount    *big.Int
	maxAmountOut *big.Int
}

// MaxAmountOut is the largest protocol output requirement this allocation can satisfy.
func (a *FillSolution) MaxAmountOut() *big.Int {
	if a == nil {
		return new(big.Int)
	}
	return liquidlane.CloneBig(a.maxAmountOut)
}

// Finalize distributes the required protocol output and gas across the selected routes.
func (a *FillSolution) Finalize(requiredAmountOut *big.Int) []liquidstrategies.FillRoute {
	if a == nil || requiredAmountOut == nil || requiredAmountOut.Sign() <= 0 ||
		requiredAmountOut.Cmp(a.maxAmountOut) > 0 {
		return nil
	}
	minimumTotal := new(big.Int).Add(requiredAmountOut, a.gasAmount)
	targets := make([]*big.Int, len(a.routes))
	for index := range a.routes {
		targets[index] = a.routes[index].targetOutput
	}
	minimums := distributeMinimums(targets, minimumTotal)
	if minimums == nil {
		return nil
	}
	routes := make([]liquidstrategies.FillRoute, len(a.routes))
	for index, leg := range a.routes {
		routes[index] = liquidstrategies.FillRoute{
			CandidateID:       leg.candidate.id(),
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

// SolveFill selects current LiquidLane quotes, enforces shared capacity, and optionally prices execution gas.
func SolveFill(task FillTask) (*FillSolution, error) {
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
	if task.InputPolicy != RejectUncoveredInput && task.InputPolicy != AbsorbUncoveredInput {
		return nil, errors.New("invalid uncovered input policy")
	}
	if task.PriceBufferBps < 0 || task.PriceBufferBps >= bpsDenominator {
		return nil, errors.Errorf("priceBufferBps: must be in [0,%d)", bpsDenominator)
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
		task.PriceBufferBps,
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
	legs := make([]liquidstrategies.GasLeg, 0, len(allocation))
	for index, leg := range allocation {
		allocation[index].targetOutput = new(big.Int).Sub(
			leg.executableOutput,
			applyBpsUp(leg.executableOutput, task.PriceBufferBps),
		)
		if allocation[index].targetOutput.Sign() <= 0 {
			task.Trace.Decline(
				"fill", "buffer-exceeds-output",
				"leg", index,
				"routeId", leg.candidate.quote.ID,
				"executableAmountOut", leg.executableOutput.String(),
				"targetAmountOut", allocation[index].targetOutput.String(),
			)
			return nil, nil
		}
		targetTotal.Add(targetTotal, allocation[index].targetOutput)
		legs = append(legs, liquidstrategies.GasLeg{
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
	return &FillSolution{
		routes: allocation, gasAmount: gasAmount, maxAmountOut: maxAmountOut,
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

func buildFillCandidates(task FillTask) ([]fillCandidate, error) {
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
			candidate, task.AmountIn, capacity, task.PriceBufferBps,
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
	priceBufferBps int,
	inputPolicy UncoveredInputPolicy,
) []fillAllocation {
	routes := buildFillRoutes(candidates)
	capacityLimits := fillCapacityLimits(candidates)
	capacityUsed := make(map[liquidlane.CapacityID]*big.Int, len(capacityLimits))
	usedRoutes := make(map[liquidlane.RouteID]bool, maxRoutes)
	remaining := liquidlane.CloneBig(amountIn)
	allocation := make([]fillAllocation, 0, maxRoutes)

	for remaining.Sign() > 0 && len(allocation) < maxRoutes {
		var best *fillAllocation
		lastRoute := inputPolicy == RejectUncoveredInput && len(allocation) == maxRoutes-1
		for _, route := range routes {
			if usedRoutes[route.id] {
				continue
			}
			choice := fillRouteChoice(
				route, remaining, capacityLimits, capacityUsed, priceBufferBps,
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
		if inputPolicy == RejectUncoveredInput || len(allocation) == 0 {
			return nil
		}
		allocation[len(allocation)-1].amountIn.Add(allocation[len(allocation)-1].amountIn, remaining)
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
	priceBufferBps int,
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
		amount := maxInputWithinCapacity(candidate, remaining, candidateCapacity, priceBufferBps)
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
	return &fillAllocation{
		candidate:        *best,
		amountIn:         legAmount,
		executableOutput: scaledFillOutput(best.quote, legAmount),
		reservedOutput:   reservedCapacityOutput(*best, legAmount, priceBufferBps),
	}
}

func maxInputWithinCapacity(
	candidate fillCandidate,
	inputLimit *big.Int,
	capacity *big.Int,
	priceBufferBps int,
) *big.Int {
	quote := candidate.quote
	if inputLimit == nil || inputLimit.Sign() <= 0 || capacity == nil || capacity.Sign() <= 0 ||
		quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 ||
		quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
		return new(big.Int)
	}
	precision := big.NewInt(bpsDenominator)
	buffer := big.NewInt(int64(priceBufferBps))
	maxOutput := new(big.Int)
	if quote.DiscountID != nil {
		maxOutput.Mul(capacity, precision)
		maxOutput.Div(maxOutput, new(big.Int).Add(precision, buffer))
	} else {
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

func reservedCapacityOutput(
	candidate fillCandidate,
	amountIn *big.Int,
	priceBufferBps int,
) *big.Int {
	amountOut := scaledFillOutput(candidate.quote, amountIn)
	buffer := applyBpsUp(amountOut, priceBufferBps)
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
