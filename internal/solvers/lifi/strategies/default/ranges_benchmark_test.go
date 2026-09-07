package defaultstrategy

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
)

// Eight physical routes, each with direct/private alternatives trading rate for
// capacity. Every interval must preserve per-route exclusivity and integer floors.
func BenchmarkBuildQuoteRanges(b *testing.B) {
	strategy := &Strategy{policy: planning.ExecutionPolicy{MinAmount: big.NewInt(1)}, rangeCount: 8}
	var candidates []liquidlane.QuoteCandidate
	for route := range 8 {
		for alternative := range 3 {
			id := strconv.Itoa(route) + ":" + strconv.Itoa(alternative)
			candidate := liquidlane.QuoteCandidate{
				ID:          liquidlane.CandidateID(id),
				Route:       liquidlane.Route{ID: liquidlane.RouteID(strconv.Itoa(route)), TokenInDecimals: 6, TokenOutDecimals: 6},
				Rate:        new(big.Int).Mul(big.NewInt(1e18), big.NewInt(int64(alternative+1))),
				MaxAmountIn: big.NewInt(100_000_000 / int64(alternative+1)), MaxAmountOut: big.NewInt(100_000_000),
			}
			if alternative > 0 {
				hash := common.BigToHash(big.NewInt(int64(route*3 + alternative)))
				candidate.DiscountID = &hash
			}
			candidates = append(candidates, candidate)
		}
	}
	pricing, err := planning.NewGasPricing(new(big.Int), common.Address{}, nil, nil, 0, planning.GasEnvelope{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		ranges, _, err := strategy.buildQuoteRanges(candidates, 8, pricing)
		if err != nil || len(ranges) == 0 {
			b.Fatalf("ranges=%d: %v", len(ranges), err)
		}
	}
}
