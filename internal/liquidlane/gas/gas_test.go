package gas

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestPredictionForRoutes(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	demands := demandsFor(coll, 100)

	cases := []struct {
		name string
		st   *State
		want uint64
	}{
		{
			name: "unknown snapshot uses conservative code fallback",
			st:   nil,
			want: UnitsForRouteAt(RouteUnknown, true),
		},
		{
			name: "acquire-only",
			st: &State{
				FreeAssets:   big.NewInt(0),
				Withdrawable: big.NewInt(0),
				Acquire:      map[common.Address]*big.Int{coll: big.NewInt(100)},
			},
			want: UnitsForRouteAt(RouteAcquire, true),
		},
		{
			name: "allocate from free assets",
			st: &State{
				FreeAssets:   big.NewInt(100),
				Withdrawable: big.NewInt(100),
				Acquire:      map[common.Address]*big.Int{},
			},
			want: UnitsForRouteAt(RouteAllocate, true),
		},
		{
			name: "deallocate before allocate",
			st: &State{
				FreeAssets:   big.NewInt(0),
				Withdrawable: big.NewInt(100),
				Acquire:      map[common.Address]*big.Int{},
			},
			want: UnitsForRouteAt(RouteDeallocate, true),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Predict(demands, c.st).Units; got != c.want {
				t.Fatalf("Predict units = %d, want %d", got, c.want)
			}
		})
	}
}

func TestPredictionConsumesSharedBudgets(t *testing.T) {
	coll := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	demands := demandsFor(coll, 70, 70, 70)
	st := &State{
		FreeAssets:   big.NewInt(80),
		Withdrawable: big.NewInt(200),
		Acquire:      map[common.Address]*big.Int{coll: big.NewInt(100)},
	}
	want := UnitsForRouteAt(RouteAcquire, true) +
		UnitsForRouteAt(RouteAllocate, false) +
		UnitsForRouteAt(RouteDeallocate, false)
	pred := Predict(demands, st)
	if got := pred.Units; got != want {
		t.Fatalf("Predict units = %d, want %d", got, want)
	}
	if got := RoutesString(pred.Routes); got != "acquire,allocate,deallocate" {
		t.Fatalf("routes = %q", got)
	}
	// The estimator must not mutate the cached LiquidLane snapshot; strategies read it lock-free across bids.
	if st.Acquire[coll].String() != "100" || st.FreeAssets.String() != "80" || st.Withdrawable.String() != "200" {
		t.Fatalf("predictor mutated input state: %+v", st)
	}
}

func demandsFor(coll common.Address, outs ...int64) []Demand {
	demands := make([]Demand, len(outs))
	for i, out := range outs {
		demands[i] = Demand{Collateral: coll, AmountOut: big.NewInt(out)}
	}
	return demands
}
