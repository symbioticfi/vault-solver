package defaultstrategy

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

// Each branch replays repayment against its own market state. Several borrowers
// share a market and all swaps consume one adapter's acquire balance.
func BenchmarkSelectNetBundle(b *testing.B) {
	for _, count := range []int{8, 24} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			cfg := Config{Sizing: SizingParams{AllowFullLiquidation: true}}
			collateral := common.BigToAddress(big.NewInt(1))
			info := MarketInfo{
				Params: MarketParams{LoanToken: tokenA, CollateralToken: collateral, Lltv: big.NewInt(500_000_000_000_000_000)},
				State: morpho.MarketState{
					TotalSupplyAssets: big.NewInt(50_000_000_000), TotalSupplyShares: big.NewInt(50_000_000_000),
					TotalBorrowAssets: big.NewInt(30_000_000_000), TotalBorrowShares: big.NewInt(30_000_000_000),
					Lltv: big.NewInt(500_000_000_000_000_000), Fee: new(big.Int), BorrowRatePerSec: new(big.Int),
				},
			}
			price := mustBig("1000000000000000000000000000")
			quote := newQuote("1200000000000000000000", nil)
			scored := make([]scoredLeg, count)
			for i := range scored {
				candidate := Candidate{MarketID: common.BigToHash(big.NewInt(int64(i%2 + 1))), Borrower: common.BigToAddress(big.NewInt(int64(i + 1))), Market: info,
					Position: morpho.PositionState{BorrowShares: big.NewInt(1_200_000_000), Collateral: big.NewInt(1_000_000_000_000_000_000)}}
				leg, profit, ok := evalLeg(candidate, price, quote, info.State.LastUpdate, cfg.Sizing)
				if !ok {
					b.Fatal("fixture cannot be sized")
				}
				scored[i] = scoredLeg{bundleLeg: bundleLeg{selectedLeg: leg, expectedLoanOut: expectedLoanOutFor(leg.MaxSeizeAssets, quote, cfg.Sizing.SwapHaircutBps), collateral: collateral}, profit: profit,
					source: &evalItem{cand: candidate, price: price, quote: quote}}
			}
			state := &liquidLaneState{FreeAssets: new(big.Int), Withdrawable: new(big.Int), Acquire: map[common.Address]*big.Int{collateral: big.NewInt(100_000_000_000)}}
			engine, gasPrice := testBundleEngine(cfg), big.NewInt(1)
			b.ReportAllocs()
			for b.Loop() {
				bundle, reason := engine.selectNetBundle(b.Context(), scored, morpho.Wad, state, gasPrice, maxSettlementGasUnits, defaultPriceUpdateFeeds)
				if reason != "" || len(bundle.legs) < 2 {
					b.Fatalf("legs=%d reason=%s", len(bundle.legs), reason)
				}
			}
		})
	}
}
