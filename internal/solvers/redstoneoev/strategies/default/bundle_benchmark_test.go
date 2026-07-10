package defaultstrategy

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

func BenchmarkBundleSearch(b *testing.B) {
	tests := []struct {
		name       string
		candidates int
		depth      int
	}{
		{name: "N100_D2", candidates: 100, depth: 2},
		{name: "N1000_D2", candidates: 1000, depth: 2},
		{name: "N1000_D8", candidates: 1000, depth: 8},
		{name: "N10000_D2", candidates: 10000, depth: 2},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			engine := testBundleEngine(Config{})
			legs := make([]scoredLeg, tc.candidates)
			for i := range legs {
				legs[i] = scoredFor(byte(i%255+1), big.NewInt(int64(tc.candidates-i+1)))
				legs[i].Borrower = common.BigToAddress(big.NewInt(int64(i + 1)))
			}
			usable := fixedSettlementGasUnits(defaultPriceUpdateFeeds) +
				liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, true)
			if tc.depth > 1 {
				usable += uint64(tc.depth-1) * liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, false)
			}
			laneState := &liquidLaneState{
				FreeAssets:   big.NewInt(0),
				Withdrawable: big.NewInt(0),
				Acquire:      map[common.Address]*big.Int{{}: new(big.Int).SetUint64(^uint64(0))},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _ = engine.searchBundle(
					legs,
					laneState,
					headerGasLimitForUsable(usable),
					defaultPriceUpdateFeeds,
					func(bundle chosenBundle) *big.Int {
						return new(big.Int).Set(bundle.grossLoan)
					},
				)
			}
		})
	}
}
