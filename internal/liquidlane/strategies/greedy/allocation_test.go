package greedy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type Candidate = liquidlane.QuoteCandidate

func TestAllocateExactInputUsesBestRatesAcrossSources(t *testing.T) {
	result := newAllocator([]Candidate{
		candidate("worse", "route-2", 90, 100),
		candidate("better", "route-1", 100, 60),
	}).allocateExactInputWithPolicy(big.NewInt(100), 2, false)

	if result.Remaining.Sign() != 0 || result.TotalAmountOut.Int64() != 96 {
		t.Fatalf("result = %+v, want complete allocation with output 96", result)
	}
	if len(result.Allocations) != 2 || result.Allocations[0].Candidate.ID != "better" ||
		result.Allocations[0].AmountIn.Int64() != 60 || result.Allocations[1].Candidate.ID != "worse" {
		t.Fatalf("allocations = %+v, want better route first", result.Allocations)
	}
}

func TestAllocateExactInputUsesCoveringAlternative(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("private", "route-1", 120, 40)
	private.DiscountID = &discountID
	direct := candidate("direct", "route-1", 100, 100)

	result := newAllocator([]Candidate{private, direct}).allocateExactInputWithPolicy(big.NewInt(80), 1, false)

	if result.Remaining.Sign() != 0 || len(result.Allocations) != 1 ||
		result.Allocations[0].Candidate.ID != "direct" || result.TotalAmountOut.Int64() != 80 {
		t.Fatalf("result = %+v, want direct alternative covering full input", result)
	}
}

func TestAllocateExactInputDoesNotReusePhysicalRoute(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("private", "route-1", 120, 40)
	private.DiscountID = &discountID
	direct := candidate("direct", "route-1", 100, 100)

	result := newAllocator([]Candidate{private, direct}).allocateExactInputWithPolicy(big.NewInt(120), 2, false)

	if len(result.Allocations) != 1 || result.Remaining.Int64() != 20 {
		t.Fatalf("result = %+v, want one physical route and 20 input remaining", result)
	}
}

func TestAllocateExactInputPrefersDirectCandidateOnTie(t *testing.T) {
	discountID := common.HexToHash("0x01")
	private := candidate("a-private", "route-1", 100, 100)
	private.DiscountID = &discountID
	private.ValidUntil = time.Unix(100, 0)
	direct := candidate("z-direct", "route-1", 100, 100)

	result := newAllocator([]Candidate{private, direct}).allocateExactInputWithPolicy(big.NewInt(50), 1, false)

	if len(result.Allocations) != 1 || result.Allocations[0].Candidate.ID != "z-direct" {
		t.Fatalf("allocations = %+v, want direct candidate", result.Allocations)
	}
}

func TestAllocateExactInputChoosesBestCoveringRouteBeforeApplyingLimit(t *testing.T) {
	discountID := common.HexToHash("0x01")
	narrowPrivate := candidate("private", "route-1", 120, 40)
	narrowPrivate.DiscountID = &discountID
	wideDirect := candidate("direct", "route-1", 50, 1_000)
	betterCovering := candidate("covering", "route-2", 100, 1_000)

	result := newAllocator([]Candidate{narrowPrivate, wideDirect, betterCovering}).
		allocateExactInputWithPolicy(big.NewInt(100), 1, false)

	if result.Remaining.Sign() != 0 || len(result.Allocations) != 1 ||
		result.Allocations[0].Candidate.ID != "covering" || result.TotalAmountOut.Int64() != 100 {
		t.Fatalf("result = %+v, want the best route that covers the requested leg", result)
	}
}

func TestAllocateExactInputRejectsInvalidRequestWithoutNilAmounts(t *testing.T) {
	result := newAllocator(nil).allocateExactInputWithPolicy(nil, 0, false)
	if result.TotalAmountOut == nil || result.Remaining == nil ||
		result.TotalAmountOut.Sign() != 0 || result.Remaining.Sign() != 0 {
		t.Fatalf("result = %+v, want non-nil zero amounts", result)
	}
}

func FuzzAllocateExactInputInvariants(f *testing.F) {
	f.Add(uint64(100), uint8(3), uint64(80), uint64(120))
	f.Add(uint64(1_000_000), uint8(1), uint64(1), uint64(250))
	f.Fuzz(func(t *testing.T, rawAmount uint64, rawRoutes uint8, rawCapacity uint64, rawRate uint64) {
		amount := new(big.Int).SetUint64(rawAmount%1_000_000 + 1)
		maxRoutes := int(rawRoutes%4) + 1
		capacity := int64(rawCapacity%1_000 + 1)
		rate := int64(rawRate%200 + 1)
		candidates := []Candidate{
			candidate("route-1", "route-1", rate, capacity),
			candidate("route-2", "route-2", rate+1, capacity+1),
			candidate("route-3", "route-3", rate+2, capacity+2),
			candidate("route-4", "route-4", rate+3, capacity+3),
		}

		result := newAllocator(candidates).allocateExactInputWithPolicy(amount, maxRoutes, false)
		if len(result.Allocations) > maxRoutes {
			t.Fatalf("allocations = %d, maxRoutes = %d", len(result.Allocations), maxRoutes)
		}
		sumIn := new(big.Int).Set(result.Remaining)
		sumOut := new(big.Int)
		seen := make(map[liquidlane.RouteID]bool, len(result.Allocations))
		for _, allocation := range result.Allocations {
			if seen[allocation.Candidate.Route.ID] {
				t.Fatalf("route %q allocated twice", allocation.Candidate.Route.ID)
			}
			seen[allocation.Candidate.Route.ID] = true
			if allocation.AmountIn.Sign() <= 0 || allocation.AmountIn.Cmp(allocation.Candidate.MaxAmountIn) > 0 {
				t.Fatalf("amountIn %s is outside candidate capacity", allocation.AmountIn)
			}
			if allocation.AmountOut.Sign() <= 0 || allocation.AmountOut.Cmp(allocation.Candidate.MaxAmountOut) > 0 {
				t.Fatalf("amountOut %s is outside candidate capacity", allocation.AmountOut)
			}
			sumIn.Add(sumIn, allocation.AmountIn)
			sumOut.Add(sumOut, allocation.AmountOut)
		}
		if sumIn.Cmp(amount) != 0 {
			t.Fatalf("allocated + remaining input = %s, want %s", sumIn, amount)
		}
		if sumOut.Cmp(result.TotalAmountOut) != 0 {
			t.Fatalf("allocation output = %s, result total = %s", sumOut, result.TotalAmountOut)
		}
	})
}

func candidate(id, routeID string, ratePercent, maxInput int64) Candidate {
	rateScale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	rate := new(big.Int).Mul(rateScale, big.NewInt(ratePercent))
	rate.Div(rate, big.NewInt(100))
	return Candidate{
		ID: liquidlane.CandidateID(id),
		Route: liquidlane.Route{
			ID: liquidlane.RouteID(routeID), TokenInDecimals: 0, TokenOutDecimals: 0,
		},
		Rate: rate, MaxAmountIn: big.NewInt(maxInput), MaxAmountOut: big.NewInt(maxInput * 2),
	}
}
