package redstoneoev

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestGasUnitsForBundleRoutes(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	bundle := chosenBundle{
		legs:        []LiquidationLeg{{SwapAmountOut: big.NewInt(100)}},
		collaterals: []common.Address{coll},
	}

	cases := []struct {
		name string
		st   *gasPredictorState
		want uint64
	}{
		{
			name: "unknown snapshot uses conservative code fallback",
			st:   nil,
			want: fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstUnknownLeg,
		},
		{
			name: "acquire-only",
			st: &gasPredictorState{
				FreeAssets:   big.NewInt(0),
				Withdrawable: big.NewInt(0),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(100)},
			},
			want: fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAcquireLeg,
		},
		{
			name: "allocate from free assets",
			st: &gasPredictorState{
				FreeAssets:   big.NewInt(100),
				Withdrawable: big.NewInt(100),
				Acquire:      map[common.Address]*big.Int{},
			},
			want: fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAllocateLeg,
		},
		{
			name: "deallocate before allocate",
			st: &gasPredictorState{
				FreeAssets:   big.NewInt(0),
				Withdrawable: big.NewInt(100),
				Acquire:      map[common.Address]*big.Int{},
			},
			want: fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstDeallocateLeg,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gasPredictionForBundle(bundle, c.st).Units; got != c.want {
				t.Fatalf("gasPredictionForBundle units = %d, want %d", got, c.want)
			}
		})
	}
}

func TestGasUnitsForBundleConsumesSharedBudgets(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	bundle := chosenBundle{
		legs: []LiquidationLeg{
			{SwapAmountOut: big.NewInt(70)},
			{SwapAmountOut: big.NewInt(70)},
			{SwapAmountOut: big.NewInt(70)},
		},
		collaterals: []common.Address{coll, coll, coll},
	}
	st := &gasPredictorState{
		FreeAssets:   big.NewInt(80),
		Withdrawable: big.NewInt(200),
		Acquire:      map[common.Address]*big.Int{coll: big.NewInt(100)},
	}
	want := fixedGasUnits(defaultPriceUpdateFeeds) + gasFirstAcquireLeg + gasAdditionalAllocateLeg + gasAdditionalDeallocateLeg
	pred := gasPredictionForBundle(bundle, st)
	if got := pred.Units; got != want {
		t.Fatalf("gasPredictionForBundle units = %d, want %d", got, want)
	}
	if got := gasRoutesString(pred.Routes); got != "acquire,allocate,deallocate" {
		t.Fatalf("routes = %q", got)
	}
	// The estimator must not mutate the cached predictor snapshot; buildBid reads it lock-free across bids.
	if st.Acquire[coll].String() != "100" || st.FreeAssets.String() != "80" || st.Withdrawable.String() != "200" {
		t.Fatalf("predictor mutated input state: %+v", st)
	}
}

func TestGasPredictionFixedFeedCostAndLimit(t *testing.T) {
	bundle := chosenBundle{legs: []LiquidationLeg{
		{SwapAmountOut: big.NewInt(1)},
		{SwapAmountOut: big.NewInt(1)},
	}}
	pred := gasPredictionForBundleFeeds(bundle, nil, 3)
	want := gasBaseUnits + gasExecutorDebitSurcharge + 3*gasPriceUpdatePerFeed + gasFirstUnknownLeg + gasAdditionalUnknownLeg
	if pred.Units != want {
		t.Fatalf("gas with feed updates = %d, want %d", pred.Units, want)
	}
	if got, limitWant := usableBundleGasLimit(30_000_000), uint64(1_700_000); got != limitWant {
		t.Fatalf("usableBundleGasLimit = %d, want %d", got, limitWant)
	}
	if got, limitWant := usableBundleGasLimit(1_000_000), uint64(850_000); got != limitWant {
		t.Fatalf("small-chain usableBundleGasLimit = %d, want %d", got, limitWant)
	}
}

func TestLiveRedStoneLimitRejectsThreeAllocateLegs(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	two := chosenBundle{
		legs: []LiquidationLeg{
			{SwapAmountOut: big.NewInt(1)},
			{SwapAmountOut: big.NewInt(1)},
		},
		collaterals: []common.Address{coll, coll},
	}
	three := chosenBundle{
		legs: []LiquidationLeg{
			{SwapAmountOut: big.NewInt(1)},
			{SwapAmountOut: big.NewInt(1)},
			{SwapAmountOut: big.NewInt(1)},
		},
		collaterals: []common.Address{coll, coll, coll},
	}
	four := chosenBundle{
		legs: []LiquidationLeg{
			{SwapAmountOut: big.NewInt(1)},
			{SwapAmountOut: big.NewInt(1)},
			{SwapAmountOut: big.NewInt(1)},
			{SwapAmountOut: big.NewInt(1)},
		},
		collaterals: []common.Address{coll, coll, coll, coll},
	}
	st := &gasPredictorState{
		FreeAssets:   big.NewInt(10),
		Withdrawable: big.NewInt(10),
		Acquire:      map[common.Address]*big.Int{},
	}

	if !bundleFitsGasLimit(two, st, 2_000_000, 3) {
		t.Fatal("two allocate legs should fit the observed RedStone settlement gas limit")
	}
	if !bundleFitsGasLimit(three, st, 2_000_000, 3) {
		t.Fatal("three allocate legs should fit the observed RedStone settlement gas limit")
	}
	if bundleFitsGasLimit(four, st, 2_000_000, 3) {
		t.Fatal("four allocate legs must not fit the observed RedStone settlement gas limit")
	}
}

func TestGasPredictionTracksForkCalibratedSettlements(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	bundle := func(legs int) chosenBundle {
		b := chosenBundle{
			legs:        make([]LiquidationLeg, legs),
			collaterals: make([]common.Address, legs),
		}
		for i := range legs {
			b.legs[i] = LiquidationLeg{SwapAmountOut: big.NewInt(1)}
			b.collaterals[i] = coll
		}
		return b
	}
	cases := []struct {
		name     string
		legs     int
		state    *gasPredictorState
		debitGas uint64
	}{
		{
			name: "one acquire leg",
			legs: 1,
			state: &gasPredictorState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(10)},
			},
			debitGas: 469_911,
		},
		{
			name: "two acquire legs",
			legs: 2,
			state: &gasPredictorState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(10)},
			},
			debitGas: 588_048,
		},
		{
			name: "one allocate leg",
			legs: 1,
			state: &gasPredictorState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{},
			},
			debitGas: 703_664,
		},
		{
			name: "two allocate legs",
			legs: 2,
			state: &gasPredictorState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{},
			},
			debitGas: 969_948,
		},
		{
			name: "mixed acquire then allocate",
			legs: 2,
			state: &gasPredictorState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(1)},
			},
			debitGas: 817_877,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			predicted := gasPredictionForBundleFeeds(bundle(c.legs), c.state, defaultPriceUpdateFeeds).Units
			if predicted < c.debitGas {
				t.Fatalf("predicted gas %d below debit gas %d", predicted, c.debitGas)
			}
			if predicted > c.debitGas*115/100 {
				t.Fatalf("predicted gas %d too far above debit gas %d", predicted, c.debitGas)
			}
		})
	}
}
