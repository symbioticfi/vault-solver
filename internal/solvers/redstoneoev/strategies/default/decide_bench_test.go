package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

// BenchmarkDecideBidNoLegs drives the common auction: every tracked position is healthy, plus markets
// the frame does not price and markets the adapter cannot quote, so DecideBid ends in no_legs.
func BenchmarkDecideBidNoLegs(b *testing.B) {
	const (
		quotedMarkets   = 40
		unpricedMarkets = 10
		unquotedMarkets = 10
		perMarket       = 10
	)
	adapter, callback := common.Address{0xad}, common.Address{0xcb}
	quotedColl, unquotedColl := common.Address{0xc1}, common.Address{0xc2}
	healthy := mustBig("2000000000000000000000000000")
	const blockTime = 1780059204
	now := time.Unix(blockTime, 0)

	seed := SnapshotSeed{
		Markets:   map[common.Hash]MarketInfo{},
		Positions: map[common.Hash]map[common.Address]morpho.PositionState{},
		Block:     1, BlockTime: blockTime, UpdatedAt: now,
	}
	var prices []types.AuctionPrice
	for i := range quotedMarkets + unpricedMarkets + unquotedMarkets {
		id := common.BigToHash(big.NewInt(int64(i + 1)))
		oracle := common.BigToAddress(big.NewInt(int64(1_000 + i)))
		coll := quotedColl
		if i >= quotedMarkets+unpricedMarkets {
			coll = unquotedColl
		}
		if i < quotedMarkets || i >= quotedMarkets+unpricedMarkets {
			prices = append(prices, types.AuctionPrice{Oracle: oracle, Price: healthy})
		}
		seed.Markets[id] = MarketInfo{
			Params: MarketParams{LoanToken: tokenA, CollateralToken: coll, Oracle: oracle, Lltv: mustBig("860000000000000000")},
			State:  goldenMarket(),
		}
		positions := make(map[common.Address]morpho.PositionState, perMarket)
		for j := range perMarket {
			positions[common.BigToAddress(big.NewInt(int64(i*perMarket+j+1)))] = goldenBorrower()
		}
		seed.Positions[id] = positions
	}

	s := NewWithSnapshotForTest(Config{Sizing: SizingParams{AllowFullLiquidation: true}}, "bench", adapter, callback,
		false, seed, logr.Discard(), nil)
	s.StoreDecisionStateForTest(big.NewInt(1e18), now)
	input := types.BidInput{
		Now:     now,
		Auction: types.AuctionSnapshot{ID: "bench", Timestamp: blockTime * 1000, Prices: prices},
		Adapter: types.AdapterSnapshot{
			Address: adapter, Loan: tokenA, LoanDecimals: 6, Filler: true,
			Redeemable: []types.RedeemableSnapshot{{
				Asset: quotedColl, Decimals: 18, MaxRate: mustBig("1780000000000000000000"), MaxAssets: big.NewInt(100_000_000_000),
			}},
		},
		Context: types.BidContext{Callback: callback},
	}
	ctx := b.Context()
	if out, err := s.DecideBid(ctx, input); err != nil || out.Reason != types.SkipReasonNoLegs {
		b.Fatalf("DecideBid = %+v, %v; want no_legs", out, err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.DecideBid(ctx, input); err != nil {
			b.Fatal(err)
		}
	}
}
