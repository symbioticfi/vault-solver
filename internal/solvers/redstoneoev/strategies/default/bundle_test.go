package defaultstrategy

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

func testBundleEngine(cfg Config) bundleEngine {
	return newBundleEngine(cfg, logr.Discard())
}

func scoredFor(borrowerByte byte, profit *big.Int) scoredLeg {
	var b common.Address
	b[19] = borrowerByte
	return scoredLeg{
		bundleLeg: bundleLeg{
			selectedLeg:     selectedLeg{Borrower: b, MarketId: common.Hash{}},
			expectedLoanOut: profit,
		},
		profit: profit,
	}
}

func headerGasLimitForUsable(usable uint64) uint64 {
	return (usable*10_000 + gasLimitSafetyBps - 1) / gasLimitSafetyBps
}

func TestBundleSearchBounds(t *testing.T) {
	t.Run("candidate order keeps all candidates by gross", func(t *testing.T) {
		const candidates = 600
		scored := make([]scoredLeg, 0, candidates)
		for i := candidates; i > 0; i-- {
			scored = append(scored, scoredLeg{
				bundleLeg: bundleLeg{
					selectedLeg: selectedLeg{Borrower: common.BigToAddress(big.NewInt(int64(i)))},
				},
				profit: big.NewInt(int64(i)),
			})
		}

		got := sortedScoredLegs(scored)
		if len(got) != candidates {
			t.Fatalf("candidate count = %d, want %d", len(got), candidates)
		}
		if got[0].profit.Int64() != candidates || got[len(got)-1].profit.Int64() != 1 {
			t.Fatalf("candidate order wrong: first=%s last=%s", got[0].profit, got[len(got)-1].profit)
		}
	})

	t.Run("depth follows usable gas", func(t *testing.T) {
		if got := bundleSearchDepth(1, defaultPriceUpdateFeeds); got != 0 {
			t.Fatalf("depth below fixed gas = %d, want 0", got)
		}
		usable := fixedSettlementGasUnits(defaultPriceUpdateFeeds) +
			liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, true) +
			liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, false)
		if got := bundleSearchDepth(headerGasLimitForUsable(usable), defaultPriceUpdateFeeds); got != 2 {
			t.Fatalf("depth = %d, want 2", got)
		}
	})
}

func TestSelectBundleSingleToken(t *testing.T) {
	t.Run("bundles all profitable legs into one bid", func(t *testing.T) {
		laneState := &liquidLaneState{
			FreeAssets:   big.NewInt(0),
			Withdrawable: big.NewInt(0),
			Acquire:      map[common.Address]*big.Int{{}: mustBig("100000000000")},
		}
		b, skip := testBundleEngine(Config{}).selectBundleWithGas([]scoredLeg{
			scoredFor(1, mustBig("60000000")),
			scoredFor(2, mustBig("30000000")),
			scoredFor(3, mustBig("9000000")),
		}, laneState, maxSettlementGasUnits, defaultPriceUpdateFeeds)
		if skip != "" {
			t.Fatalf("unexpected skip %q", skip)
		}
		if len(b.legs) != 3 || b.grossLoan.String() != "99000000" {
			t.Fatalf("legs=%d grossLoan=%s, want 3 / 99000000", len(b.legs), b.grossLoan)
		}
	})

	t.Run("header gas limit caps the group", func(t *testing.T) {
		twoAcquireLegs := fixedSettlementGasUnits(defaultPriceUpdateFeeds) +
			liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, true) +
			liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, false)
		laneState := &liquidLaneState{
			FreeAssets:   big.NewInt(0),
			Withdrawable: big.NewInt(0),
			Acquire:      map[common.Address]*big.Int{{}: mustBig("100000000")},
		}
		b, skip := testBundleEngine(Config{}).selectBundleWithGas([]scoredLeg{
			scoredFor(1, mustBig("10000000")),
			scoredFor(2, mustBig("30000000")),
			scoredFor(3, mustBig("20000000")),
		}, laneState, headerGasLimitForUsable(twoAcquireLegs), defaultPriceUpdateFeeds)
		if skip != "" {
			t.Fatalf("unexpected skip %q", skip)
		}
		if len(b.legs) != 2 || b.grossLoan.String() != "50000000" {
			t.Fatalf("legs=%d gross=%s, want 2 / 50000000", len(b.legs), b.grossLoan)
		}
	})

	t.Run("empty scored set", func(t *testing.T) {
		if _, skip := testBundleEngine(Config{}).selectBundleWithGas(nil, nil, 0, defaultPriceUpdateFeeds); skip != "no_legs" {
			t.Fatalf("skip = %q, want no_legs", skip)
		}
	})

	t.Run("net selection rejects an invalid rate", func(t *testing.T) {
		if _, skip := testBundleEngine(Config{}).selectNetBundle(
			[]scoredLeg{scoredFor(1, big.NewInt(1))}, nil, nil, big.NewInt(1), maxSettlementGasUnits, defaultPriceUpdateFeeds,
		); skip != skipGasUnprofitable {
			t.Fatalf("skip = %q, want %q", skip, skipGasUnprofitable)
		}
	})

	t.Run("equal-profit legs ordered deterministically", func(t *testing.T) {
		laneState := &liquidLaneState{
			FreeAssets:   big.NewInt(0),
			Withdrawable: big.NewInt(0),
			Acquire:      map[common.Address]*big.Int{{}: mustBig("30000000")},
		}
		b, skip := testBundleEngine(Config{}).selectBundleWithGas([]scoredLeg{
			scoredFor(3, mustBig("10000000")),
			scoredFor(1, mustBig("10000000")),
			scoredFor(2, mustBig("10000000")),
		}, laneState, maxSettlementGasUnits, defaultPriceUpdateFeeds)
		if skip != "" {
			t.Fatalf("unexpected skip %q", skip)
		}
		if len(b.legs) != 3 || b.legs[0].Borrower[19] != 1 || b.legs[1].Borrower[19] != 2 || b.legs[2].Borrower[19] != 3 {
			t.Fatalf("borrower order = %d,%d,%d, want 1,2,3",
				b.legs[0].Borrower[19], b.legs[1].Borrower[19], b.legs[2].Borrower[19])
		}
	})
}

func TestPriceBundleWithoutGasAccountingKeepsNativeSafetyAndOneUnitFloors(t *testing.T) {
	engine := testBundleEngine(Config{BidWei: big.NewInt(100)})
	bundle := chosenBundle{
		grossLoan: big.NewInt(1_000_000),
		legs: []bundleLeg{
			{
				selectedLeg: selectedLeg{
					MarketId: common.Hash{31: 1}, Borrower: common.Address{19: 1},
					MaxSeizeAssets: big.NewInt(10), MinProfit: big.NewInt(999),
				},
				expectedLoanOut: big.NewInt(500_000),
			},
			{
				selectedLeg: selectedLeg{
					MarketId: common.Hash{31: 2}, Borrower: common.Address{19: 2},
					MaxSeizeAssets: big.NewInt(20), MinProfit: big.NewInt(999),
				},
				expectedLoanOut: big.NewInt(500_000),
			},
		},
	}
	gasPrice := big.NewInt(2)
	priced := engine.priceBundleWithoutGasAccounting(bundle, nil, gasPrice, defaultPriceUpdateFeeds)

	if priced.bidNative.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("bid = %s, want fixed bid 100", priced.bidNative)
	}
	if priced.minBundleProfitLoan.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("minimum bundle profit = %s, want 1", priced.minBundleProfitLoan)
	}
	for i, leg := range priced.selectedLegs {
		if leg.MinProfit == nil || leg.MinProfit.Cmp(big.NewInt(1)) != 0 {
			t.Fatalf("leg %d minimum profit = %v, want 1", i, leg.MinProfit)
		}
	}
	wantGasNative := new(big.Int).Mul(new(big.Int).SetUint64(priced.gas.Units), gasPrice)
	if priced.gas.Units == 0 || priced.gasNative.Cmp(wantGasNative) != 0 {
		t.Fatalf("gas reservation = %s for %d units, want %s", priced.gasNative, priced.gas.Units, wantGasNative)
	}
}

func TestSelectBundlePerCollateralBudget(t *testing.T) {
	collA := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	collB := common.HexToAddress("0x00000000000000000000000000000000000000cb")
	withColl := func(byteID byte, profit int64, c common.Address, maxA int64) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = c
		sl.maxAssets = big.NewInt(maxA)
		return sl
	}
	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{collA: big.NewInt(100), collB: big.NewInt(100)},
	}
	b, skip := testBundleEngine(Config{}).selectBundleWithGas([]scoredLeg{
		withColl(1, 60, collA, 100),
		withColl(2, 60, collA, 100),
		withColl(3, 10, collB, 100),
	}, laneState, maxSettlementGasUnits, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	got := map[byte]bool{}
	for _, l := range b.legs {
		got[l.Borrower[19]] = true
	}
	if len(b.legs) != 2 || !got[1] || got[2] || !got[3] {
		t.Fatalf("included borrowers = %v (legs=%d), want {1,3}", got, len(b.legs))
	}
	if b.grossLoan.String() != "70" {
		t.Fatalf("grossLoan = %s, want 70", b.grossLoan)
	}
}

func TestSelectBundleAllowsSameMarketStaticLegs(t *testing.T) {
	marketA := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	marketB := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	withMarket := func(byteID byte, profit int64, market common.Hash) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.MarketId = market
		return sl
	}
	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{{}: big.NewInt(150)},
	}
	b, skip := testBundleEngine(Config{}).selectBundleWithGas([]scoredLeg{
		withMarket(1, 60, marketA),
		withMarket(2, 50, marketA),
		withMarket(3, 40, marketB),
	}, laneState, maxSettlementGasUnits, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	got := map[byte]bool{}
	for _, leg := range b.legs {
		got[leg.Borrower[19]] = true
	}
	if len(b.legs) != 3 || !got[1] || !got[2] || !got[3] {
		t.Fatalf("selected borrowers = %v (legs=%d), want both same-market static legs plus other market", got, len(b.legs))
	}
}

func TestSelectBundleReplaysSameMarketSources(t *testing.T) {
	cfg := Config{Sizing: SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0}}
	market := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	coll := common.HexToAddress("0x00000000000000000000000000000000000000c0")
	info := MarketInfo{
		Params: abiMarketParams{LoanToken: tokenA, CollateralToken: coll, Lltv: mustBig("500000000000000000")},
		State: morpho.MarketState{
			TotalSupplyAssets: mustBig("5000000000"),
			TotalSupplyShares: mustBig("5000000000"),
			TotalBorrowAssets: mustBig("3000000000"),
			TotalBorrowShares: mustBig("3000000000"),
			Lltv:              mustBig("500000000000000000"),
			Fee:               big.NewInt(0),
			BorrowRatePerSec:  big.NewInt(0),
		},
	}
	price := mustBig("1000000000000000000000000000")
	quote := newQuote("1200000000000000000000", nil)
	replayable := func(byteID byte) scoredLeg {
		var borrower common.Address
		borrower[19] = byteID
		pos := morpho.PositionState{BorrowShares: mustBig("1200000000"), Collateral: mustBig("1000000000000000000")}
		cand := Candidate{MarketID: market, Borrower: borrower, Market: info, Position: pos}
		sized, ok := sizeLeg(cand, price, quote, info.State.TotalBorrowAssets, cfg.Sizing)
		if !ok {
			t.Fatal("fixture should size")
		}
		leg := sized.leg
		leg.MaxSeizeAssets = big.NewInt(1)
		return scoredLeg{
			bundleLeg: bundleLeg{
				selectedLeg:     leg,
				expectedLoanOut: big.NewInt(1),
				collateral:      coll,
			},
			profit: mustBig("999999999999999999"),
			source: evalItem{cand: cand, price: price, quote: quote, accrued: info.State.TotalBorrowAssets},
			replay: true,
		}
	}

	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{coll: mustBig("10000000000000000000000")},
	}
	b, skip := testBundleEngine(cfg).selectBundleWithGas([]scoredLeg{replayable(1), replayable(2)}, laneState, maxSettlementGasUnits, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	if len(b.legs) != 2 {
		t.Fatalf("selected %d same-market replayed legs, want 2", len(b.legs))
	}
	if b.legs[0].MaxSeizeAssets.Cmp(big.NewInt(1)) == 0 || b.legs[0].expectedLoanOut.Cmp(big.NewInt(1)) == 0 {
		t.Fatalf("selected stale precomputed leg instead of replaying source: %+v", b.legs[0])
	}
	if b.grossLoan.Cmp(mustBig("999999999999999999")) >= 0 {
		t.Fatalf("grossLoan used stale bogus profit: %s", b.grossLoan)
	}
	if _, ok := morpho.ApplySeizeLiquidation(info.State, replayable(1).source.cand.Position, b.legs[0].MaxSeizeAssets, price); !ok {
		t.Fatal("first replayed leg should apply to initial market state")
	}
}

func TestSelectNetBundleAvoidsGrossBestGasFalseSkip(t *testing.T) {
	collHigh := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	collLow := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	withColl := func(byteID byte, profit int64, c common.Address) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = c
		return sl
	}
	engine := testBundleEngine(Config{})
	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{collLow: big.NewInt(1_000_000)},
	}
	b, skip := engine.selectNetBundle([]scoredLeg{
		withColl(1, 640_000, collHigh),
		withColl(2, 600_000, collLow),
	}, morpho.Wad, laneState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("lower-gross passing route should be selected, got skip %q", skip)
	}
	if len(b.legs) != 1 || b.legs[0].Borrower[19] != 2 {
		t.Fatalf("selected borrowers = %+v, want only lower-gross acquire leg", b.legs)
	}
	if got := engine.bundleNetNative(b, morpho.Wad, laneState, big.NewInt(1)); got.Cmp(big.NewInt(1)) < 0 {
		t.Fatalf("selected bundle net = %s, want >= min margin", got)
	}

	t.Run("searches past gross-only candidate window", func(t *testing.T) {
		const formerGrossWindow = 512
		withAddr := func(addr common.Address, profit int64, c common.Address) scoredLeg {
			return scoredLeg{
				bundleLeg: bundleLeg{
					selectedLeg:     selectedLeg{Borrower: addr},
					expectedLoanOut: big.NewInt(profit),
					collateral:      c,
				},
				profit: big.NewInt(profit),
			}
		}
		scored := make([]scoredLeg, 0, formerGrossWindow+2)
		for i := 0; i <= formerGrossWindow; i++ {
			scored = append(scored, withAddr(common.BigToAddress(big.NewInt(int64(i+1))), 620_000, collHigh))
		}
		wantBorrower := common.BigToAddress(big.NewInt(10_000))
		scored = append(scored, withAddr(wantBorrower, 600_000, collLow))

		gotBundle, gotSkip := engine.selectNetBundle(scored, morpho.Wad, laneState, big.NewInt(1), maxSettlementGasUnits, defaultPriceUpdateFeeds)
		if gotSkip != "" {
			t.Fatalf("lower-gross passing leg after the old window should be selected, got skip %q", gotSkip)
		}
		if len(gotBundle.legs) != 1 || gotBundle.legs[0].Borrower != wantBorrower {
			t.Fatalf("selected borrowers = %+v, want only lower-gross acquire leg past gross-only window", gotBundle.legs)
		}
	})
}

func TestSelectNetBundleAllowsSameMarketStaticLegs(t *testing.T) {
	market := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	collA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	collB := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	withMarket := func(byteID byte, profit int64, c common.Address) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.MarketId = market
		sl.collateral = c
		return sl
	}
	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire: map[common.Address]*big.Int{
			collA: big.NewInt(700_000),
			collB: big.NewInt(700_000),
		},
	}
	b, skip := testBundleEngine(Config{}).selectNetBundle([]scoredLeg{
		withMarket(1, 700_000, collA),
		withMarket(2, 700_000, collB),
	}, morpho.Wad, laneState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("unexpected skip %q", skip)
	}
	if len(b.legs) != 2 {
		t.Fatalf("selected %d same-market legs, want 2", len(b.legs))
	}
}

func TestSelectNetBundleSharesBaseGasAcrossLegs(t *testing.T) {
	collA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	collB := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	withColl := func(byteID byte, profit int64, c common.Address) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = c
		return sl
	}
	engine := testBundleEngine(Config{})
	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire: map[common.Address]*big.Int{
			collA: big.NewInt(590_000),
			collB: big.NewInt(590_000),
		},
	}
	b, skip := engine.selectNetBundle([]scoredLeg{
		withColl(1, 590_000, collA),
		withColl(2, 590_000, collB),
	}, morpho.Wad, laneState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("combined bundle should share base gas and pass, got skip %q", skip)
	}
	if len(b.legs) != 2 {
		t.Fatalf("selected %d legs, want 2", len(b.legs))
	}
	if got := engine.bundleNetNative(b, morpho.Wad, laneState, big.NewInt(1)); got.Cmp(big.NewInt(1)) < 0 {
		t.Fatalf("selected bundle net = %s, want >= min margin", got)
	}
}

func TestSelectNetBundleSearchesPastGreedyBudgetTrap(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	withColl := func(byteID byte, profit int64) scoredLeg {
		sl := scoredFor(byteID, big.NewInt(profit))
		sl.collateral = coll
		sl.maxAssets = big.NewInt(1_240_000)
		return sl
	}
	engine := testBundleEngine(Config{BidWei: big.NewInt(0)})
	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{coll: big.NewInt(1_400_000)},
	}
	b, skip := engine.selectNetBundle([]scoredLeg{
		withColl(1, 700_000),
		withColl(2, 620_000),
		withColl(3, 620_000),
	}, morpho.Wad, laneState, big.NewInt(1), 0, defaultPriceUpdateFeeds)
	if skip != "" {
		t.Fatalf("expected lower-gross pair to pass, got skip %q", skip)
	}
	got := map[byte]bool{}
	for _, leg := range b.legs {
		got[leg.Borrower[19]] = true
	}
	if len(b.legs) != 2 || got[1] || !got[2] || !got[3] {
		t.Fatalf("selected borrowers = %v (legs=%d), want {2,3}", got, len(b.legs))
	}
	if gotNet := engine.bundleNetNative(b, morpho.Wad, laneState, big.NewInt(1)); gotNet.Cmp(big.NewInt(1)) < 0 {
		t.Fatalf("selected bundle net = %s, want >= min margin", gotNet)
	}
}

func TestSearchBundleDoesNotRequireMonotonicScore(t *testing.T) {
	engine := testBundleEngine(Config{})
	legs := []scoredLeg{
		scoredFor(1, big.NewInt(1)),
		scoredFor(2, big.NewInt(1)),
	}
	scoreFn := func(b chosenBundle) *big.Int {
		if len(b.legs) < 2 {
			return big.NewInt(-1)
		}
		return big.NewInt(10)
	}

	laneState := &liquidLaneState{
		FreeAssets:   big.NewInt(0),
		Withdrawable: big.NewInt(0),
		Acquire:      map[common.Address]*big.Int{{}: big.NewInt(2)},
	}
	best, ok := engine.searchBundle(legs, laneState, maxSettlementGasUnits, defaultPriceUpdateFeeds, scoreFn)
	if !ok {
		t.Fatal("search should keep temporary negative states when a deeper bundle can become profitable")
	}
	if len(best.bundle.legs) != 2 {
		t.Fatalf("selected %d legs, want 2", len(best.bundle.legs))
	}
}

func TestBundleBidNativeUsesProfitShareFloor(t *testing.T) {
	b := chosenBundle{grossLoan: big.NewInt(1_000)}
	engine := testBundleEngine(Config{BidWei: big.NewInt(100), TotalBundleProfitBps: 2_000})
	if got := engine.bundleBidNative(b, morpho.Wad); got.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("bid = %s, want 20%% of gross native", got)
	}
	engine = testBundleEngine(Config{BidWei: big.NewInt(100), TotalBundleProfitBps: 500})
	if got := engine.bundleBidNative(b, morpho.Wad); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("bid = %s, want minimal bid floor", got)
	}
}
