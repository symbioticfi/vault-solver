package defaultstrategy

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

var benchmarkDecisionSink types.BidOutput

func BenchmarkDefaultStrategyDecideBid10000Candidates(b *testing.B) {
	const candidateCount = 10_000
	positions := make(map[common.Address]morpho.PositionState, candidateCount)
	for i := range candidateCount {
		positions[common.BigToAddress(big.NewInt(int64(i+1)))] = goldenBorrower()
	}
	strategy, input := newDecisionCharacterizationFixtureWith(b, characterizationFixtureOptions{
		markets: map[common.Hash]MarketInfo{
			characterizationMarket: characterizationMarketInfo(characterizationOracle),
		},
		prices: map[common.Hash]*big.Int{
			characterizationMarket: mustBig("1550000000000000000000000000"),
		},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{
			characterizationMarket: positions,
		},
	})
	b.ReportAllocs()
	b.ReportMetric(candidateCount, "candidates/op")
	b.ResetTimer()
	for b.Loop() {
		var err error
		benchmarkDecisionSink, err = strategy.DecideBid(b.Context(), input)
		if err != nil {
			b.Fatal(err)
		}
	}
	if benchmarkDecisionSink.Decision != types.DecisionBid {
		b.Fatalf("DecideBid decision = %+v", benchmarkDecisionSink)
	}
}
