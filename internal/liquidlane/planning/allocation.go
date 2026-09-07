package planning

import (
	"cmp"
	"math/big"
	"slices"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Allocation selects one direct or private alternative of a physical route.
type Allocation struct {
	Candidate liquidlane.QuoteCandidate
	AmountIn  *big.Int
	AmountOut *big.Int
}

type allocationResult struct {
	Allocations    []Allocation
	TotalAmountIn  *big.Int
	TotalAmountOut *big.Int
	Remaining      *big.Int
}

// QuoteRoute groups mutually exclusive alternatives of one physical route.
// Its fields and integer values are read-only for the lifetime of a QuotePool.
type QuoteRoute struct {
	id           liquidlane.RouteID
	Alternatives []liquidlane.QuoteCandidate
	MaxInput     *big.Int
	maxOutput    *big.Int
	bestRate     *big.Int
}

// QuotePool groups physical routes once for repeated amount queries. Candidates and
// their integer fields must remain immutable while the pool is in use; each Solve
// owns its computed input/output amounts, so calls can run concurrently.
type QuotePool struct{ sources []QuoteRoute }
type quoteSide bool

const (
	inputSide  quoteSide = false
	outputSide quoteSide = true
)

func NewQuotePool(candidates []liquidlane.QuoteCandidate) QuotePool {
	return QuotePool{sources: buildSources(candidates)}
}

// Routes returns the prepared read-only route groups in ranking order.
func (a QuotePool) Routes() []QuoteRoute { return a.sources }

func newAllocationResult(amount *big.Int) allocationResult {
	remaining := new(big.Int)
	if amount != nil {
		remaining.Set(amount)
	}
	return allocationResult{Remaining: remaining, TotalAmountIn: new(big.Int), TotalAmountOut: new(big.Int)}
}

func (a QuotePool) allocateExactInputWithPolicy(amount *big.Int, limit int, complete bool) allocationResult {
	return a.allocate(amount, limit, inputSide, complete)
}

func (a QuotePool) allocateExactOutput(amount *big.Int, limit int) allocationResult {
	return a.allocate(amount, limit, outputSide, false)
}

// allocate keeps one remaining amount in the requested units. Selection always chooses a complete
// physical-route leg before comparing its alternatives, so a small discount cannot hide a larger
// direct route. Exact-output conversion rounds input up; output surplus is retained explicitly.
func (a QuotePool) allocate(amount *big.Int, limit int, side quoteSide, complete bool) allocationResult {
	result := newAllocationResult(amount)
	used := make([]bool, len(a.sources))
	for result.Remaining.Sign() > 0 && len(result.Allocations) < limit {
		candidate, wanted, index := a.choose(used, result.Remaining, side, complete && len(result.Allocations) == limit-1)
		if index < 0 {
			break
		}
		used[index] = true
		in := wanted
		if side == outputSide {
			in = liquidlane.MinAmountInForAmountOut(wanted, candidate.Rate, candidate.Route.TokenInDecimals, candidate.Route.TokenOutDecimals)
		}
		out := output(candidate, in)
		if in.Sign() <= 0 || in.Cmp(candidate.MaxAmountIn) > 0 || out.Sign() <= 0 {
			continue
		}
		result.Allocations = append(result.Allocations, Allocation{Candidate: candidate, AmountIn: in, AmountOut: out})
		result.TotalAmountIn.Add(result.TotalAmountIn, in)
		result.TotalAmountOut.Add(result.TotalAmountOut, out)
		consumed := in
		if side == outputSide {
			consumed = bigmath.Min(result.Remaining, out)
		}
		result.Remaining.Sub(result.Remaining, consumed)
	}
	return result
}

func (a QuotePool) choose(used []bool, remaining *big.Int, side quoteSide, complete bool) (liquidlane.QuoteCandidate, *big.Int, int) {
	var chosen liquidlane.QuoteCandidate
	var amount *big.Int
	chosenIndex := -1
	for index, route := range a.sources {
		if used[index] {
			continue
		}
		capacity := route.MaxInput
		if side == outputSide {
			capacity = route.maxOutput
		}
		if complete && capacity.Cmp(remaining) < 0 {
			continue
		}
		wanted := bigmath.Min(remaining, capacity)
		for _, option := range route.Alternatives {
			available := option.MaxAmountIn
			if side == outputSide {
				available = output(option, option.MaxAmountIn)
			}
			if available.Cmp(wanted) < 0 {
				continue
			}
			if amount == nil || better(option, chosen) {
				chosen, amount, chosenIndex = option, wanted, index
			}
		}
	}
	return chosen, amount, chosenIndex
}

func buildSources(candidates []liquidlane.QuoteCandidate) []QuoteRoute {
	routes := make(map[liquidlane.RouteID]*QuoteRoute)
	for _, candidate := range candidates {
		if !validCandidate(candidate) {
			continue
		}
		r := routes[candidate.Route.ID]
		if r == nil {
			r = &QuoteRoute{id: candidate.Route.ID, MaxInput: candidate.MaxAmountIn, maxOutput: new(big.Int), bestRate: candidate.Rate}
			routes[r.id] = r
		}
		r.Alternatives = append(r.Alternatives, candidate)
		if candidate.MaxAmountIn.Cmp(r.MaxInput) > 0 {
			r.MaxInput = candidate.MaxAmountIn
		}
		if candidate.Rate.Cmp(r.bestRate) > 0 {
			r.bestRate = candidate.Rate
		}
		if out := output(candidate, candidate.MaxAmountIn); out.Cmp(r.maxOutput) > 0 {
			r.maxOutput = out
		}
	}
	result := make([]QuoteRoute, 0, len(routes))
	for _, r := range routes {
		result = append(result, *r)
	}
	slices.SortFunc(result, func(a, b QuoteRoute) int {
		return cmp.Or(b.bestRate.Cmp(a.bestRate), b.MaxInput.Cmp(a.MaxInput), cmp.Compare(a.id, b.id))
	})
	return result
}

func validCandidate(c liquidlane.QuoteCandidate) bool {
	return c.ID != "" && c.Route.ID != "" && c.Rate != nil && c.Rate.Sign() > 0 &&
		c.MaxAmountIn != nil && c.MaxAmountIn.Sign() > 0 && c.MaxAmountOut != nil && c.MaxAmountOut.Sign() > 0
}

func better(a, b liquidlane.QuoteCandidate) bool {
	if c := a.Rate.Cmp(b.Rate); c != 0 {
		return c > 0
	}
	if c := a.MaxAmountIn.Cmp(b.MaxAmountIn); c != 0 {
		return c > 0
	}
	if (a.DiscountID == nil) != (b.DiscountID == nil) {
		return a.DiscountID == nil
	}
	if a.ValidUntil.IsZero() != b.ValidUntil.IsZero() {
		return a.ValidUntil.IsZero()
	}
	if !a.ValidUntil.Equal(b.ValidUntil) {
		return a.ValidUntil.After(b.ValidUntil)
	}
	return a.ID < b.ID
}

func output(c liquidlane.QuoteCandidate, in *big.Int) *big.Int {
	return bigmath.Min(liquidlane.AmountOutForRate(in, c.Rate, c.Route.TokenInDecimals, c.Route.TokenOutDecimals), c.MaxAmountOut)
}
