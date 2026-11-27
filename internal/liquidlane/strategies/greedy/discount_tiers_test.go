package greedy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

func TestConnectorDiscountTiersChooseByGrossOracleValue(t *testing.T) {
	route := liquidlane.NewRoute(1, common.HexToAddress("0x01"), common.HexToAddress("0x02"), common.HexToAddress("0x03"), common.HexToAddress("0x04"), 6, 6)
	cutoff := big.NewInt(25_000_000_000)
	makeTier := func(id string, discount int64, bounds liquidlane.OracleBounds) *liquidlane.QuoteCandidate {
		item := liquidlane.DiscountInventory(route, big.NewInt(50_000_000_000), nil, common.HexToHash(id), time.Time{})
		item.Discount = big.NewInt(discount)
		item.DiscountPrecision = 100_000_000
		item.Price = big.NewInt(1_000_000_000_000_000_000)
		item.OracleBounds = bounds
		return NewQuoteCandidate(item, item.MaxAssets)
	}
	base := makeTier("0x11", 1_000_000, liquidlane.OracleBounds{Max: cutoff})
	upper := makeTier("0x12", 2_000_000, liquidlane.OracleBounds{Min: cutoff})
	for _, tc := range []struct {
		amount, payout int64
		id             string
	}{
		{24_999_999_999, 24_749_999_999, "0x11"},
		{25_000_000_000, 24_500_000_000, "0x12"},
		{25_000_000_001, 24_500_000_000, "0x12"},
	} {
		result, err := SolveQuote(strategies.QuoteTask{
			ExactInput: big.NewInt(tc.amount), Candidates: []liquidlane.QuoteCandidate{*base, *upper},
			MaxRoutes: 1, InputPolicy: strategies.RejectUncoveredInput,
		})
		if err != nil || result == nil || len(result.Allocations) != 1 {
			t.Fatalf("amount %d: got %v, %v", tc.amount, result, err)
		}
		if result.AmountOut.Int64() != tc.payout || *result.Allocations[0].Candidate.DiscountID != common.HexToHash(tc.id) {
			t.Fatalf("amount %d: payout %s, tier %s", tc.amount, result.AmountOut, result.Allocations[0].Candidate.DiscountID)
		}
	}
}
