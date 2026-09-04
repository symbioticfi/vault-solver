// Package greedy contains the greedy LiquidLane quote and fill strategy shared
// by protocol-specific solver strategies.
package planning

import (
	"math/big"
	"sort"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Allocation is the selected amount for one candidate.
type Allocation struct {
	Candidate liquidlane.QuoteCandidate
	AmountIn  *big.Int
	AmountOut *big.Int
}

// allocation records one deterministic routing pass. Remaining is expressed
// in input units for exact-input work and output units for exact-output work.
type allocation struct {
	Allocations    []Allocation
	TotalAmountIn  *big.Int
	TotalAmountOut *big.Int
	Remaining      *big.Int
}

// routeBook groups alternatives by physical route. A route may be used only
// once, while its direct and private alternatives compete at the chosen size.
type routeBook struct {
	routes []pricedRoute
}

func newRouteBook(candidates []liquidlane.QuoteCandidate) routeBook {
	return routeBook{routes: indexRoutes(candidates)}
}

func (book routeBook) allocateInput(
	amountIn *big.Int,
	maxRoutes int,
	requireComplete bool,
) allocation {
	result := newAllocation(amountIn)
	if amountIn == nil || amountIn.Sign() <= 0 || maxRoutes <= 0 {
		return result
	}

	result.Allocations = make([]Allocation, 0, min(maxRoutes, len(book.routes)))
	used := make(map[liquidlane.RouteID]struct{}, min(maxRoutes, len(book.routes)))
	for result.Remaining.Sign() > 0 && len(result.Allocations) < maxRoutes {
		mustFinish := requireComplete && len(result.Allocations) == maxRoutes-1
		candidate, amount, ok := chooseInputLeg(book.routes, used, result.Remaining, mustFinish)
		if !ok {
			break
		}
		amountOut := output(candidate, amount)
		if amountOut.Sign() <= 0 {
			used[candidate.Route.ID] = struct{}{}
			continue
		}
		result.Allocations = append(result.Allocations, Allocation{
			Candidate: candidate,
			AmountIn:  liquidlane.CloneBig(amount),
			AmountOut: amountOut,
		})
		result.TotalAmountIn.Add(result.TotalAmountIn, amount)
		result.TotalAmountOut.Add(result.TotalAmountOut, amountOut)
		result.Remaining.Sub(result.Remaining, amount)
		used[candidate.Route.ID] = struct{}{}
	}
	return result
}

// allocateExactOutput greedily buys amountOut from the best physical routes.
// Output rounding may intentionally produce a surplus.
func (book routeBook) allocateOutput(targetOutput *big.Int, maxRoutes int) allocation {
	result := newAllocation(targetOutput)
	if targetOutput == nil || targetOutput.Sign() <= 0 || maxRoutes <= 0 {
		return result
	}

	result.Allocations = make([]Allocation, 0, min(maxRoutes, len(book.routes)))
	used := make(map[liquidlane.RouteID]struct{}, min(maxRoutes, len(book.routes)))
	for result.Remaining.Sign() > 0 && len(result.Allocations) < maxRoutes {
		candidate, wanted, ok := chooseOutputLeg(book.routes, used, result.Remaining)
		if !ok {
			break
		}
		amountIn := liquidlane.MinAmountInForAmountOut(
			wanted,
			candidate.Rate,
			candidate.Route.TokenInDecimals,
			candidate.Route.TokenOutDecimals,
		)
		amountOut := output(candidate, amountIn)
		if amountIn.Sign() <= 0 || amountIn.Cmp(candidate.MaxAmountIn) > 0 || amountOut.Sign() <= 0 {
			used[candidate.Route.ID] = struct{}{}
			continue
		}
		result.Allocations = append(result.Allocations, Allocation{
			Candidate: candidate,
			AmountIn:  amountIn,
			AmountOut: amountOut,
		})
		result.TotalAmountIn.Add(result.TotalAmountIn, amountIn)
		result.TotalAmountOut.Add(result.TotalAmountOut, amountOut)
		result.Remaining.Sub(result.Remaining, cloneMin(result.Remaining, amountOut))
		used[candidate.Route.ID] = struct{}{}
	}
	return result
}

func newAllocation(remaining *big.Int) allocation {
	result := allocation{
		TotalAmountIn: new(big.Int), TotalAmountOut: new(big.Int), Remaining: new(big.Int),
	}
	if remaining != nil {
		result.Remaining.Set(remaining)
	}
	return result
}

type pricedRoute struct {
	id           liquidlane.RouteID
	alternatives []liquidlane.QuoteCandidate
	maxInput     *big.Int
	maxOutput    *big.Int
	bestRate     *big.Int
}

func indexRoutes(candidates []liquidlane.QuoteCandidate) []pricedRoute {
	byRoute := make(map[liquidlane.RouteID][]liquidlane.QuoteCandidate)
	for _, candidate := range candidates {
		if !validCandidate(candidate) {
			continue
		}
		byRoute[candidate.Route.ID] = append(byRoute[candidate.Route.ID], candidate)
	}

	routes := make([]pricedRoute, 0, len(byRoute))
	for routeID, alternatives := range byRoute {
		item := pricedRoute{
			id: routeID, alternatives: alternatives,
			maxInput: new(big.Int), maxOutput: new(big.Int), bestRate: new(big.Int),
		}
		for _, candidate := range alternatives {
			if candidate.MaxAmountIn.Cmp(item.maxInput) > 0 {
				item.maxInput.Set(candidate.MaxAmountIn)
			}
			if candidate.Rate.Cmp(item.bestRate) > 0 {
				item.bestRate.Set(candidate.Rate)
			}
			if candidateOutput := output(candidate, candidate.MaxAmountIn); candidateOutput.Cmp(item.maxOutput) > 0 {
				item.maxOutput.Set(candidateOutput)
			}
		}
		routes = append(routes, item)
	}
	sort.Slice(routes, func(i, j int) bool {
		if cmp := routes[i].bestRate.Cmp(routes[j].bestRate); cmp != 0 {
			return cmp > 0
		}
		if cmp := routes[i].maxInput.Cmp(routes[j].maxInput); cmp != 0 {
			return cmp > 0
		}
		return routes[i].id < routes[j].id
	})
	return routes
}

func validCandidate(candidate liquidlane.QuoteCandidate) bool {
	return candidate.ID != "" && candidate.Route.ID != "" &&
		candidate.Rate != nil && candidate.Rate.Sign() > 0 &&
		candidate.MaxAmountIn != nil && candidate.MaxAmountIn.Sign() > 0 &&
		candidate.MaxAmountOut != nil && candidate.MaxAmountOut.Sign() > 0
}

func chooseInputLeg(
	routes []pricedRoute,
	used map[liquidlane.RouteID]struct{},
	remaining *big.Int,
	mustCoverRemaining bool,
) (liquidlane.QuoteCandidate, *big.Int, bool) {
	var best liquidlane.QuoteCandidate
	var bestAmount *big.Int
	found := false
	for _, item := range routes {
		if _, alreadyUsed := used[item.id]; alreadyUsed {
			continue
		}
		if mustCoverRemaining && item.maxInput.Cmp(remaining) < 0 {
			continue
		}
		amount := cloneMin(remaining, item.maxInput)
		for _, candidate := range item.alternatives {
			if candidate.MaxAmountIn.Cmp(amount) < 0 {
				continue
			}
			if !found || better(candidate, best) {
				best, bestAmount, found = candidate, amount, true
			}
		}
	}
	return best, bestAmount, found
}

func chooseOutputLeg(
	routes []pricedRoute,
	used map[liquidlane.RouteID]struct{},
	remaining *big.Int,
) (liquidlane.QuoteCandidate, *big.Int, bool) {
	var best liquidlane.QuoteCandidate
	var bestAmount *big.Int
	found := false
	for _, item := range routes {
		if _, alreadyUsed := used[item.id]; alreadyUsed {
			continue
		}
		amount := cloneMin(remaining, item.maxOutput)
		for _, candidate := range item.alternatives {
			if output(candidate, candidate.MaxAmountIn).Cmp(amount) < 0 {
				continue
			}
			if !found || better(candidate, best) {
				best, bestAmount, found = candidate, amount, true
			}
		}
	}
	return best, bestAmount, found
}

func better(left, right liquidlane.QuoteCandidate) bool {
	if cmp := left.Rate.Cmp(right.Rate); cmp != 0 {
		return cmp > 0
	}
	if cmp := left.MaxAmountIn.Cmp(right.MaxAmountIn); cmp != 0 {
		return cmp > 0
	}
	leftDirect := left.DiscountID == nil
	rightDirect := right.DiscountID == nil
	if leftDirect != rightDirect {
		return leftDirect
	}
	if left.ValidUntil.IsZero() != right.ValidUntil.IsZero() {
		return left.ValidUntil.IsZero()
	}
	if !left.ValidUntil.Equal(right.ValidUntil) {
		return left.ValidUntil.After(right.ValidUntil)
	}
	return left.ID < right.ID
}

func output(candidate liquidlane.QuoteCandidate, amountIn *big.Int) *big.Int {
	amountOut := liquidlane.AmountOutForRate(
		amountIn,
		candidate.Rate,
		candidate.Route.TokenInDecimals,
		candidate.Route.TokenOutDecimals,
	)
	if amountOut.Cmp(candidate.MaxAmountOut) > 0 {
		return liquidlane.CloneBig(candidate.MaxAmountOut)
	}
	return amountOut
}
