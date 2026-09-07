package defaultstrategy

import (
	"math/big"
	"slices"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"

	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

// rangeQuoter prepares one immutable candidate set for every interval in a curve.
// Route grouping and gas bounds are shared; only the interval amount changes.
type rangeQuoter struct {
	task                    planning.QuoteTask
	pool                    planning.QuotePool
	inDecimals, outDecimals int
	maximum, gasCost        *big.Int
}

func newRangeQuoter(s *Strategy, candidates []liquidlane.QuoteCandidate, maxRoutes int, pricing planning.GasPricing) rangeQuoter {
	q := rangeQuoter{
		task: planning.QuoteTask{MaxRoutes: maxRoutes, MinInput: s.policy.MinAmount,
			OutputBufferBps: 2 * s.policy.PriceBufferBps, InputPolicy: planning.RejectUncoveredInput, GasPricing: &pricing},
		pool:    planning.NewQuotePool(candidates),
		maximum: new(big.Int),
	}
	if len(candidates) > 0 {
		q.inDecimals, q.outDecimals = candidates[0].Route.TokenInDecimals, candidates[0].Route.TokenOutDecimals
	}
	private := 0
	for _, route := range q.pool.Routes() {
		q.maximum.Add(q.maximum, route.MaxInput)
		for _, alternative := range route.Alternatives {
			if alternative.DiscountID != nil {
				private++
				break
			}
		}
	}
	q.gasCost = pricing.MaxCost(len(q.pool.Routes()), private)
	return q
}

func (s *Strategy) buildQuoteRanges(candidates []liquidlane.QuoteCandidate, maxRoutes int, pricing planning.GasPricing) ([]types.QuoteRange, map[liquidlane.CandidateID]liquidlane.QuoteCandidate, error) {
	candidates = planning.BestRouteCandidates(candidates, maxRoutes)
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	q := newRangeQuoter(s, candidates, maxRoutes, pricing)
	if q.maximum.Sign() <= 0 {
		return nil, nil, nil
	}
	breakpoints := quoteBreakpoints(q.maximum, s.policy.MinAmount, s.rangeCount)
	ranges := make([]types.QuoteRange, 0, len(breakpoints))
	lower := new(big.Int).Set(s.policy.MinAmount)
	for _, upper := range breakpoints {
		if upper.Cmp(lower) < 0 {
			continue
		}
		floor := q.floor(upper)
		if safeLower := floor.firstSafe(lower, upper); safeLower != nil {
			quoted, err := q.quote(safeLower, upper, floor)
			if err != nil {
				return nil, nil, err
			}
			if quoted != nil {
				ranges = append(ranges, *quoted)
			}
		}
		lower = new(big.Int).Add(upper, big.NewInt(1))
	}
	used := make(map[liquidlane.CandidateID]liquidlane.QuoteCandidate)
	if len(ranges) > 0 {
		for _, candidate := range candidates {
			used[candidate.ID] = candidate
		}
	}
	return ranges, used, nil
}

func (q rangeQuoter) quote(lower, upper *big.Int, floor rangeFloor) (*types.QuoteRange, error) {
	rate := floor.at(lower)
	for _, amount := range []*big.Int{lower, upper} {
		task := q.task
		task.ExactInput = amount
		result, err := q.pool.Solve(task)
		if err != nil || result == nil {
			return nil, err
		}
		bound := liquidlane.MaxRateForAmountOut(result.AmountOut, amount, floor.inDecimals, floor.outDecimals)
		if bound.Cmp(rate) < 0 {
			rate = bound
		}
	}
	if rate.Sign() <= 0 || liquidlane.AmountOutForRate(lower, rate, floor.inDecimals, floor.outDecimals).Sign() <= 0 {
		return nil, nil
	}
	return &types.QuoteRange{MinAmount: new(big.Int).Set(lower), MaxAmount: new(big.Int).Set(upper), Quote: bigmath.Decimal(rate, rateScaleDigits)}, nil
}

// rangeFloor stores the guaranteed rate before amortizing gas and integer
// rounding losses over the smallest request in a quoted interval.
type rangeFloor struct {
	rate, loss              *big.Int
	inDecimals, outDecimals int
}

func (q rangeQuoter) floor(maximum *big.Int) rangeFloor {
	floor := rangeFloor{rate: new(big.Int), loss: new(big.Int)}
	if len(q.pool.Routes()) == 0 || maximum == nil || maximum.Sign() <= 0 {
		return floor
	}
	floor.inDecimals, floor.outDecimals = q.inDecimals, q.outDecimals
	var worst *big.Int
	for _, route := range q.pool.Routes() {
		capacity := route.MaxInput
		if capacity.Cmp(maximum) > 0 {
			capacity = maximum
		}
		var best *big.Int
		for _, choice := range route.Alternatives {
			if choice.MaxAmountIn.Cmp(capacity) >= 0 && (best == nil || choice.Rate.Cmp(best) > 0) {
				best = choice.Rate
			}
		}
		if best != nil && (worst == nil || best.Cmp(worst) < 0) {
			worst = best
		}
	}
	if worst == nil || worst.Sign() <= 0 {
		return floor
	}
	floor.rate.Mul(worst, big.NewInt(int64(bpsDenominator-q.task.OutputBufferBps)))
	floor.rate.Quo(floor.rate, big.NewInt(bpsDenominator))
	if q.gasCost != nil {
		floor.loss.Set(q.gasCost)
	}
	// Summing route floors loses at most n-1 base units; buffering can lose one more.
	floor.loss.Add(floor.loss, big.NewInt(int64(len(q.pool.Routes())-1)))
	if q.task.OutputBufferBps > 0 {
		floor.loss.Add(floor.loss, big.NewInt(1))
	}
	return floor
}

func (floor rangeFloor) firstSafe(lower, upper *big.Int) *big.Int {
	if lower == nil || upper == nil || lower.Sign() <= 0 || lower.Cmp(upper) > 0 {
		return nil
	}
	safe := func(amount *big.Int) bool {
		rate := floor.at(amount)
		return rate.Sign() > 0 && liquidlane.AmountOutForRate(amount, rate, floor.inDecimals, floor.outDecimals).Sign() > 0
	}
	if safe(lower) {
		return new(big.Int).Set(lower)
	}
	if !safe(upper) {
		return nil
	}
	// Upper fixes rate and worst loss. Only amortization changes during the search.
	low, high := new(big.Int).Set(lower), new(big.Int).Set(upper)
	for low.Cmp(high) < 0 {
		middle := new(big.Int).Rsh(new(big.Int).Add(low, high), 1)
		if safe(middle) {
			high.Set(middle)
		} else {
			low.Add(middle, big.NewInt(1))
		}
	}
	return low
}

func (floor rangeFloor) at(minimum *big.Int) *big.Int {
	if minimum == nil || minimum.Sign() <= 0 || floor.rate.Sign() <= 0 {
		return new(big.Int)
	}
	rate := new(big.Int).Set(floor.rate)
	if floor.loss.Sign() != 0 {
		lossRate := liquidlane.RateForAmountOut(floor.loss, minimum, floor.inDecimals, floor.outDecimals)
		rate.Sub(rate, lossRate.Add(lossRate, big.NewInt(1)))
	}
	if rate.Sign() < 0 {
		rate.SetInt64(0)
	}
	return rate
}

// Split the interval with the widest relative span. The slice remains sorted,
// so each split needs neither decimal map keys nor a full re-sort.
func quoteBreakpoints(maximum, minimum *big.Int, targetCount int) []*big.Int {
	if maximum == nil || minimum == nil || maximum.Cmp(minimum) < 0 || targetCount <= 0 {
		return nil
	}
	points := []*big.Int{new(big.Int).Set(maximum)}
	for len(points) < targetCount {
		selected := -1
		var selectedLow, selectedHigh *big.Int
		low := minimum
		for i, high := range points {
			splittable := low.Sign() > 0 && new(big.Int).Sub(high, low).Cmp(big.NewInt(1)) > 0
			if splittable && (selected < 0 || new(big.Int).Mul(high, selectedLow).Cmp(new(big.Int).Mul(selectedHigh, low)) > 0) {
				selected, selectedLow, selectedHigh = i, low, high
			}
			low = high
		}
		if selected < 0 {
			break
		}
		// Only the chosen interval needs a square root. Its positive integer
		// endpoints differ by at least two, so the adjusted midpoint stays inside.
		mid := new(big.Int).Sqrt(new(big.Int).Mul(selectedLow, selectedHigh))
		if mid.Cmp(selectedLow) <= 0 {
			mid.Add(selectedLow, big.NewInt(1))
		}
		points = slices.Insert(points, selected, mid)
	}
	return points
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
