package planning

import (
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
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
	GasPricing          *GasPricing
	Trace               DecisionTrace
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
	return bigmath.Clone(a.maxAmountOut)
}

// Finalize distributes the required protocol output and gas across the selected routes.
func (a *FillSolution) Finalize(requiredAmountOut *big.Int) []FillRoute {
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
	routes := make([]FillRoute, len(a.routes))
	for index, leg := range a.routes {
		routes[index] = FillRoute{
			CandidateID:       leg.candidate.id(),
			RouteID:           leg.candidate.quote.ID,
			CapacityID:        liquidlane.RouteCapacityID(leg.candidate.quote.Route),
			Adapter:           leg.candidate.quote.Adapter,
			AmountIn:          bigmath.Clone(leg.amountIn),
			ExpectedAmountOut: bigmath.Clone(leg.targetOutput),
			MinAmountOut:      minimums[index],
			ReservedAmountOut: bigmath.Clone(leg.reservedOutput),
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
	legs := make([]GasLeg, 0, len(allocation))
	for index := range allocation {
		leg := &allocation[index]
		leg.targetOutput = new(big.Int).Sub(
			leg.executableOutput,
			applyBpsUp(leg.executableOutput, task.PriceBufferBps),
		)
		if leg.targetOutput.Sign() <= 0 {
			task.Trace.Decline(
				"fill", "buffer-exceeds-output",
				"leg", index,
				"routeId", leg.candidate.quote.ID,
				"executableAmountOut", leg.executableOutput.String(),
				"targetAmountOut", leg.targetOutput.String(),
			)
			return nil, nil
		}
		targetTotal.Add(targetTotal, leg.targetOutput)
		legs = append(legs, GasLeg{
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
	var result []fillCandidate
	seen := make(map[liquidlane.CandidateID]bool)
	for _, quote := range task.Quotes {
		if quote.TokenIn != task.TokenIn || quote.TokenOut != task.TokenOut || (!quote.ValidUntil.IsZero() && !quote.ValidUntil.After(task.ValidAfter)) {
			continue
		}
		if quote.AmountIn == nil || quote.AmountIn.Cmp(task.AmountIn) != 0 {
			return nil, errors.Errorf("fill quote %s amountIn does not match order", quote.ID)
		}
		candidate := fillCandidate{quote: quote}
		if seen[candidate.id()] || quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0 || quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
			continue
		}
		candidate.capacity = AvailableCapacity(quote.MaxAssets, task.InventoryReserveBps)
		if held := task.Reservations[liquidlane.RouteCapacityID(quote.Route)]; held != nil && held.Sign() > 0 {
			candidate.capacity.Sub(candidate.capacity, held)
		}
		candidate.maxInput = maxInputWithinCapacity(candidate, task.AmountIn, candidate.capacity, task.PriceBufferBps)
		if candidate.maxInput.Sign() > 0 {
			seen[candidate.id()] = true
			result = append(result, candidate)
		}
	}
	return result, nil
}

// greedyFillAllocation spends each capacity domain once across all its routes.
// Within a physical route, first maximize the usable input, then choose its best alternative.
func greedyFillAllocation(candidates []fillCandidate, amountIn *big.Int, maxRoutes, priceBufferBps int, policy UncoveredInputPolicy) []fillAllocation {
	budget := make(liquidlane.CapacityReservations)
	routes := make(map[liquidlane.RouteID][]int)
	for index, c := range candidates {
		routes[c.quote.ID] = append(routes[c.quote.ID], index)
		id := liquidlane.RouteCapacityID(c.quote.Route)
		if budget[id] == nil || c.capacity.Cmp(budget[id]) > 0 {
			budget[id] = bigmath.Clone(c.capacity)
		}
	}
	remaining := bigmath.Clone(amountIn)
	selected := make([]fillAllocation, 0, min(maxRoutes, len(routes)))
	for remaining.Sign() > 0 && len(selected) < maxRoutes {
		var best fillAllocation
		for _, alternatives := range routes {
			var option fillAllocation
			for _, index := range alternatives {
				c := candidates[index]
				capacity := bigmath.Min(c.capacity, budget[liquidlane.RouteCapacityID(c.quote.Route)])
				input := maxInputWithinCapacity(c, bigmath.Min(remaining, c.maxInput), capacity, priceBufferBps)
				if input.Sign() <= 0 {
					continue
				}
				candidate := fillAllocation{candidate: c, amountIn: input}
				if option.amountIn == nil || input.Cmp(option.amountIn) > 0 || (input.Cmp(option.amountIn) == 0 && betterFill(candidate, option)) {
					option = candidate
				}
			}
			if option.amountIn == nil || policy == RejectUncoveredInput && len(selected) == maxRoutes-1 && option.amountIn.Cmp(remaining) < 0 {
				continue
			}
			if best.amountIn == nil || betterFill(option, best) {
				best = option
			}
		}
		if best.amountIn == nil {
			break
		}
		best.executableOutput = scaledFillOutput(best.candidate.quote, best.amountIn)
		best.reservedOutput = reservedCapacityOutput(best.candidate, best.amountIn, priceBufferBps)
		selected = append(selected, best)
		delete(routes, best.candidate.quote.ID)
		left := budget[liquidlane.RouteCapacityID(best.candidate.quote.Route)]
		left.Sub(left, best.reservedOutput)
		remaining.Sub(remaining, best.amountIn)
	}
	if remaining.Sign() > 0 {
		if policy == RejectUncoveredInput || len(selected) == 0 {
			return nil
		}
		selected[len(selected)-1].amountIn.Add(selected[len(selected)-1].amountIn, remaining)
	}
	return selected
}

// Candidate construction validates every quote against the same requested input,
// so comparing output amounts is exactly the rate comparison without cross-products.
func betterFill(a, b fillAllocation) bool {
	if order := a.candidate.quote.MaxAmountOut.Cmp(b.candidate.quote.MaxAmountOut); order != 0 {
		return order > 0
	}
	if order := a.amountIn.Cmp(b.amountIn); order != 0 {
		return order > 0
	}
	if (a.candidate.quote.DiscountID == nil) != (b.candidate.quote.DiscountID == nil) {
		return a.candidate.quote.DiscountID == nil
	}
	return a.candidate.id() < b.candidate.id()
}

func maxInputWithinCapacity(candidate fillCandidate, limit, capacity *big.Int, bufferBps int) *big.Int {
	quote := candidate.quote
	for _, value := range []*big.Int{limit, capacity, quote.AmountIn, quote.MaxAmountOut} {
		if value == nil || value.Sign() <= 0 {
			return new(big.Int)
		}
	}
	var payable *big.Int
	precision := big.NewInt(bpsDenominator)
	if quote.DiscountID == nil {
		// Invert floor(output*(1-buffer)): ceil((capacity+1)/(1-buffer))-1.
		payable = liquidlane.MulDivUp(new(big.Int).Add(capacity, big.NewInt(1)), precision, big.NewInt(int64(bpsDenominator-bufferBps)))
		payable.Sub(payable, big.NewInt(1))
	} else {
		// Private capacity reserves output plus its rounded-up buffer.
		payable = new(big.Int).Quo(new(big.Int).Mul(capacity, precision), big.NewInt(int64(bpsDenominator+bufferBps)))
	}
	maximum := liquidlane.MulDivUp(payable.Add(payable, big.NewInt(1)), quote.AmountIn, quote.MaxAmountOut)
	maximum.Sub(maximum, big.NewInt(1))
	if maximum.Cmp(limit) > 0 {
		maximum.Set(limit)
	}
	return maximum
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
	if len(targets) == 0 || total == nil {
		return nil
	}
	// Reserve one base unit per leg, then distribute the remainder in proportion
	// to each leg's unused target. Recomputing the remaining denominator carries
	// rounding dust forward and makes the final sum exact.
	remaining := new(big.Int).Sub(total, big.NewInt(int64(len(targets))))
	if remaining.Sign() < 0 {
		return nil
	}
	weightSum := big.NewInt(-int64(len(targets)))
	for _, target := range targets {
		if target == nil || target.Sign() <= 0 {
			return nil
		}
		weightSum.Add(weightSum, target)
	}
	if remaining.Cmp(weightSum) > 0 {
		return nil
	}
	result := make([]*big.Int, len(targets))
	for index, target := range targets {
		result[index] = big.NewInt(1)
		if remaining.Sign() == 0 {
			continue
		}
		weight := new(big.Int).Sub(target, big.NewInt(1))
		share := new(big.Int).Mul(remaining, weight)
		share.Quo(share, weightSum)
		result[index].Add(result[index], share)
		remaining.Sub(remaining, share)
		weightSum.Sub(weightSum, weight)
	}
	return result
}

func scaledFillOutput(quote liquidlane.FillQuote, amountIn *big.Int) *big.Int {
	if quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 ||
		quote.MaxAmountOut == nil || amountIn == nil {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(quote.MaxAmountOut, amountIn), quote.AmountIn)
}
