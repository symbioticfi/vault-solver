package defaultstrategy

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

func TestFixedSettlementGasAndLimit(t *testing.T) {
	pred := predictGasForFeeds(gasLegsFor(common.Address{}, 1, 1), nil, 3)
	want := fixedSettlementGasUnits(3) +
		liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, true) +
		liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, false)
	if pred.Units != want {
		t.Fatalf("gas with feed updates = %d, want %d", pred.Units, want)
	}
	if got, wantLimit := usableGasLimit(30_000_000), uint64(1_700_000); got != wantLimit {
		t.Fatalf("usable limit = %d, want %d", got, wantLimit)
	}
	if got, wantLimit := usableGasLimit(1_000_000), uint64(850_000); got != wantLimit {
		t.Fatalf("small-chain usable limit = %d, want %d", got, wantLimit)
	}
}

func TestFitsRedStoneLimit(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	st := &liquidLaneState{
		FreeAssets:   big.NewInt(10),
		Withdrawable: big.NewInt(10),
		Acquire:      map[common.Address]*big.Int{},
	}

	if !fitsGasLimit(gasLegsFor(coll, 1, 1), st, 2_000_000, 3) {
		t.Fatal("two allocate legs should fit the observed RedStone settlement gas limit")
	}
	if !fitsGasLimit(gasLegsFor(coll, 1, 1, 1), st, 2_000_000, 3) {
		t.Fatal("three allocate legs should fit the observed RedStone settlement gas limit")
	}
	if fitsGasLimit(gasLegsFor(coll, 1, 1, 1, 1), st, 2_000_000, 3) {
		t.Fatal("four allocate legs must not fit the observed RedStone settlement gas limit")
	}
}

func TestGasPredictionTracksForkCalibratedSettlements(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	cases := []struct {
		name     string
		legs     int
		state    *liquidLaneState
		debitGas uint64
	}{
		{
			name: "one acquire leg",
			legs: 1,
			state: &liquidLaneState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(10)},
			},
			debitGas: 469_911,
		},
		{
			name: "two acquire legs",
			legs: 2,
			state: &liquidLaneState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(10)},
			},
			debitGas: 588_048,
		},
		{
			name: "one allocate leg",
			legs: 1,
			state: &liquidLaneState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{},
			},
			debitGas: 703_664,
		},
		{
			name: "two allocate legs",
			legs: 2,
			state: &liquidLaneState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{},
			},
			debitGas: 969_948,
		},
		{
			name: "mixed acquire then allocate",
			legs: 2,
			state: &liquidLaneState{
				FreeAssets:   big.NewInt(10),
				Withdrawable: big.NewInt(10),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(1)},
			},
			debitGas: 817_877,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outs := make([]int64, c.legs)
			for i := range outs {
				outs[i] = 1
			}
			predicted := predictGasForFeeds(gasLegsFor(coll, outs...), c.state, defaultPriceUpdateFeeds).Units
			if predicted < c.debitGas {
				t.Fatalf("predicted gas %d below debit gas %d", predicted, c.debitGas)
			}
			if predicted > c.debitGas*115/100 {
				t.Fatalf("predicted gas %d too far above debit gas %d", predicted, c.debitGas)
			}
		})
	}
}

func gasLegsFor(coll common.Address, outs ...int64) []legHint {
	legs := make([]legHint, len(outs))
	for i, out := range outs {
		legs[i] = legHint{Collateral: coll, ExpectedLoanOut: big.NewInt(out)}
	}
	return legs
}
