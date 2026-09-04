package planning

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type Candidate = liquidlane.QuoteCandidate

func TestRouteBookAllocatesEachPhysicalRouteOnce(t *testing.T) {
	private := quoteCandidate("private", "route-1", 120, 40)
	discount := common.HexToHash("0x01")
	private.DiscountID = &discount
	private.ValidUntil = time.Unix(100, 0)

	tests := []struct {
		name       string
		candidates []liquidlane.QuoteCandidate
		amount     int64
		maxRoutes  int
		wantIDs    []liquidlane.CandidateID
		wantOut    int64
		remaining  int64
	}{
		{
			name: "best rates across routes",
			candidates: []liquidlane.QuoteCandidate{
				quoteCandidate("worse", "route-2", 90, 100),
				quoteCandidate("better", "route-1", 100, 60),
			},
			amount: 100, maxRoutes: 2,
			wantIDs: []liquidlane.CandidateID{"better", "worse"}, wantOut: 96,
		},
		{
			name: "covering alternative wins",
			candidates: []liquidlane.QuoteCandidate{
				private, quoteCandidate("direct", "route-1", 100, 100),
			},
			amount: 80, maxRoutes: 1,
			wantIDs: []liquidlane.CandidateID{"direct"}, wantOut: 80,
		},
		{
			name: "route is never reused",
			candidates: []liquidlane.QuoteCandidate{
				private, quoteCandidate("direct", "route-1", 100, 100),
			},
			amount: 120, maxRoutes: 2,
			wantIDs: []liquidlane.CandidateID{"direct"}, wantOut: 100, remaining: 20,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := newRouteBook(test.candidates).allocateInput(big.NewInt(test.amount), test.maxRoutes, false)
			if got.TotalAmountOut.Int64() != test.wantOut || got.Remaining.Int64() != test.remaining ||
				len(got.Allocations) != len(test.wantIDs) {
				t.Fatalf("allocation = %+v", got)
			}
			for index, wantID := range test.wantIDs {
				if got.Allocations[index].Candidate.ID != wantID {
					t.Fatalf("leg %d candidate = %q, want %q", index, got.Allocations[index].Candidate.ID, wantID)
				}
			}
		})
	}
}

func FuzzRouteBookPreservesInput(f *testing.F) {
	f.Add(uint64(100), uint8(3), uint64(80), uint64(120))
	f.Fuzz(func(t *testing.T, rawAmount uint64, rawRoutes uint8, rawCapacity, rawRate uint64) {
		amount := new(big.Int).SetUint64(rawAmount%1_000_000 + 1)
		limit := int(rawRoutes%4) + 1
		capacity, rate := int64(rawCapacity%1_000+1), int64(rawRate%200+1)
		candidates := make([]liquidlane.QuoteCandidate, 4)
		for index := range candidates {
			id := liquidlane.RouteID(rune('a' + index))
			candidates[index] = quoteCandidate(liquidlane.CandidateID(id), id, rate+int64(index), capacity+int64(index))
		}
		got := newRouteBook(candidates).allocateInput(amount, limit, false)
		sum := new(big.Int).Set(got.Remaining)
		seen := make(map[liquidlane.RouteID]struct{}, len(got.Allocations))
		for _, leg := range got.Allocations {
			if _, duplicate := seen[leg.Candidate.Route.ID]; duplicate {
				t.Fatalf("route %q allocated twice", leg.Candidate.Route.ID)
			}
			seen[leg.Candidate.Route.ID] = struct{}{}
			sum.Add(sum, leg.AmountIn)
		}
		if len(got.Allocations) > limit || sum.Cmp(amount) != 0 {
			t.Fatalf("invalid allocation: %+v", got)
		}
	})
}

func quoteCandidate(id liquidlane.CandidateID, routeID liquidlane.RouteID, ratePercent, maxInput int64) liquidlane.QuoteCandidate {
	rate := new(big.Int).Mul(big.NewInt(ratePercent), new(big.Int).Exp(big.NewInt(10), big.NewInt(16), nil))
	return liquidlane.QuoteCandidate{
		ID:    id,
		Route: liquidlane.Route{ID: routeID, TokenInDecimals: 0, TokenOutDecimals: 0},
		Rate:  rate, MaxAmountIn: big.NewInt(maxInput), MaxAmountOut: big.NewInt(maxInput * 2),
	}
}

func candidate(id, routeID string, ratePercent, maxInput int64) Candidate {
	return quoteCandidate(liquidlane.CandidateID(id), liquidlane.RouteID(routeID), ratePercent, maxInput)
}
