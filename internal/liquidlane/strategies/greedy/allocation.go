// Package greedy contains the greedy LiquidLane quote and fill strategy shared
// by protocol-specific solver strategies.
package greedy

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

// allocationResult describes the allocated prefix of an exact-input request.
type allocationResult struct {
	Allocations    []Allocation
	TotalAmountIn  *big.Int
	TotalAmountOut *big.Int
	Remaining      *big.Int
}

// allocator is an immutable, normalized set of priced LiquidLane routes.
type allocator struct {
	sources []source
}

func newAllocator(candidates []liquidlane.QuoteCandidate) allocator {
	return allocator{sources: buildSources(candidates)}
}

func (a allocator) allocateExactInputWithPolicy(
	amountIn *big.Int,
	maxRoutes int,
	requireComplete bool,
) allocationResult {
	result := newAllocationResult(amountIn)
	if amountIn == nil || amountIn.Sign() <= 0 || maxRoutes <= 0 {
		return result
	}

	result.Allocations = make([]Allocation, 0, min(maxRoutes, len(a.sources)))
	used := make(map[liquidlane.RouteID]bool, min(maxRoutes, len(a.sources)))
	for result.Remaining.Sign() > 0 && len(result.Allocations) < maxRoutes {
		mustCoverRemaining := requireComplete && len(result.Allocations) == maxRoutes-1
		candidate, amount, ok := bestInputLeg(a.sources, used, result.Remaining, mustCoverRemaining)
		if !ok {
			break
		}
		amountOut := output(candidate, amount)
		if amountOut.Sign() <= 0 {
			used[candidate.Route.ID] = true
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
		used[candidate.Route.ID] = true
	}
	return result
}

// allocateExactOutput greedily buys amountOut from the best physical routes.
// Output rounding may intentionally produce a surplus.
func (a allocator) allocateExactOutput(targetOutput *big.Int, maxRoutes int) allocationResult {
	result := newAllocationResult(targetOutput)
	if targetOutput == nil || targetOutput.Sign() <= 0 || maxRoutes <= 0 {
		return result
	}

	result.Allocations = make([]Allocation, 0, min(maxRoutes, len(a.sources)))
	used := make(map[liquidlane.RouteID]bool, min(maxRoutes, len(a.sources)))
	for result.Remaining.Sign() > 0 && len(result.Allocations) < maxRoutes {
		candidate, wanted, ok := bestOutputLeg(a.sources, used, result.Remaining)
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
			used[candidate.Route.ID] = true
			continue
		}
		result.Allocations = append(result.Allocations, Allocation{
			Candidate: candidate,
			AmountIn:  amountIn,
			AmountOut: amountOut,
		})
		result.TotalAmountIn.Add(result.TotalAmountIn, amountIn)
		result.TotalAmountOut.Add(result.TotalAmountOut, amountOut)
		result.Remaining.Sub(result.Remaining, minBig(result.Remaining, amountOut))
		used[candidate.Route.ID] = true
	}
	return result
}

func newAllocationResult(remaining *big.Int) allocationResult {
	result := allocationResult{
		TotalAmountIn: new(big.Int), TotalAmountOut: new(big.Int), Remaining: new(big.Int),
	}
	if remaining != nil {
		result.Remaining.Set(remaining)
	}
	return result
}

type source struct {
	id           liquidlane.RouteID
	alternatives []liquidlane.QuoteCandidate
	maxInput     *big.Int
	maxOutput    *big.Int
	bestRate     *big.Int
}

func buildSources(candidates []liquidlane.QuoteCandidate) []source {
	bySource := make(map[liquidlane.RouteID][]liquidlane.QuoteCandidate)
	for _, candidate := range candidates {
		if !validCandidate(candidate) {
			continue
		}
		bySource[candidate.Route.ID] = append(bySource[candidate.Route.ID], candidate)
	}

	sources := make([]source, 0, len(bySource))
	for sourceID, group := range bySource {
		item := source{
			id: sourceID, alternatives: group,
			maxInput: new(big.Int), maxOutput: new(big.Int), bestRate: new(big.Int),
		}
		for _, candidate := range group {
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
		if len(group) > 0 {
			sources = append(sources, item)
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if cmp := sources[i].bestRate.Cmp(sources[j].bestRate); cmp != 0 {
			return cmp > 0
		}
		if cmp := sources[i].maxInput.Cmp(sources[j].maxInput); cmp != 0 {
			return cmp > 0
		}
		return sources[i].id < sources[j].id
	})
	return sources
}

func validCandidate(candidate liquidlane.QuoteCandidate) bool {
	return candidate.ID != "" && candidate.Route.ID != "" &&
		candidate.Rate != nil && candidate.Rate.Sign() > 0 &&
		candidate.MaxAmountIn != nil && candidate.MaxAmountIn.Sign() > 0 &&
		candidate.MaxAmountOut != nil && candidate.MaxAmountOut.Sign() > 0
}

func bestInputLeg(
	sources []source,
	used map[liquidlane.RouteID]bool,
	remaining *big.Int,
	mustCoverRemaining bool,
) (liquidlane.QuoteCandidate, *big.Int, bool) {
	var best liquidlane.QuoteCandidate
	var bestAmount *big.Int
	found := false
	for _, item := range sources {
		if used[item.id] {
			continue
		}
		if mustCoverRemaining && item.maxInput.Cmp(remaining) < 0 {
			continue
		}
		amount := minAmount(remaining, item.maxInput)
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

func bestOutputLeg(
	sources []source,
	used map[liquidlane.RouteID]bool,
	remaining *big.Int,
) (liquidlane.QuoteCandidate, *big.Int, bool) {
	var best liquidlane.QuoteCandidate
	var bestAmount *big.Int
	found := false
	for _, item := range sources {
		if used[item.id] {
			continue
		}
		amount := minBig(remaining, item.maxOutput)
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

func minAmount(left, right *big.Int) *big.Int {
	if left.Cmp(right) <= 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}
