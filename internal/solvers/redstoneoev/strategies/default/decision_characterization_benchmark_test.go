package defaultstrategy

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

var (
	benchmarkScoredLegsSink []scoredLeg
	benchmarkBundleSink     chosenBundle
)

func BenchmarkDefaultStrategyScore10000Candidates(b *testing.B) {
	const candidateCount = 10_000
	strategy, input := newDecisionCharacterizationFixture(b, false)
	positions := make(map[common.Address]morpho.PositionState, candidateCount)
	for i := range candidateCount {
		positions[common.BigToAddress(big.NewInt(int64(i+1)))] = goldenBorrower()
	}
	snap := cloneSnapshot(strategy.mon.snapshot())
	snap.positions[characterizationMarket] = positions
	strategy.mon.(*apiMonitor).snap.Store(snap)
	b.ReportAllocs()
	b.ReportMetric(candidateCount, "candidates/op")
	for b.Loop() {
		benchmarkScoredLegsSink = strategy.scoredLegs(input.Auction, input.Now, input.Adapter)
	}
	if len(benchmarkScoredLegsSink) != candidateCount {
		b.Fatalf("scored candidates = %d, want %d", len(benchmarkScoredLegsSink), candidateCount)
	}
}

func BenchmarkDefaultStrategyBoundedBundleSearch10000(b *testing.B) {
	const candidateCount = 10_000
	scored := make([]scoredLeg, candidateCount)
	for i := range scored {
		amount := big.NewInt(int64(candidateCount - i))
		scored[i] = scoredLeg{
			bundleLeg: bundleLeg{
				selectedLeg: selectedLeg{
					MarketId: common.BigToHash(big.NewInt(int64(i + 1))),
					Borrower: common.BigToAddress(big.NewInt(int64(i + 1))),
				},
				expectedLoanOut: amount,
				collateral:      characterizationColl,
			},
			profit: amount,
		}
	}
	laneState := &liquidLaneState{
		FreeAssets: big.NewInt(0), Withdrawable: big.NewInt(0),
		Acquire: map[common.Address]*big.Int{characterizationColl: big.NewInt(candidateCount * candidateCount)},
	}
	usableForOneAcquire := fixedSettlementGasUnits(defaultPriceUpdateFeeds) + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, true)
	gasLimit := (usableForOneAcquire*10_000 + gasLimitSafetyBps - 1) / gasLimitSafetyBps
	engine := newBundleEngine(Config{}, logr.Discard())
	b.ReportAllocs()
	b.ReportMetric(candidateCount, "candidates/op")
	b.ReportMetric(float64(bundleSearchDepth(gasLimit, defaultPriceUpdateFeeds)), "max-depth/op")
	for b.Loop() {
		var skip string
		benchmarkBundleSink, skip = engine.selectBundleWithGas(scored, laneState, gasLimit, defaultPriceUpdateFeeds)
		if skip != "" {
			b.Fatalf("bundle search skipped: %s", skip)
		}
	}
	if len(benchmarkBundleSink.legs) != 1 {
		b.Fatalf("selected legs = %d, want gas-bounded depth 1", len(benchmarkBundleSink.legs))
	}
}
