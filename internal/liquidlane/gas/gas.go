// Package gas provides LiquidLane route gas prediction and Chainlink-backed gas conversion facts.
//
// It is intentionally limited to LiquidLane adapter swap accounting: callers provide
// expected swap demands plus a compact adapter liquidity snapshot, and the package
// returns route labels and route gas units. Solver-specific settlement and payload overhead,
// auction/executor gas limits, price updates, bids, and economics stay outside.
package gas

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/bigmath"
)

type Prediction struct {
	Units  uint64
	Routes []Route
}

// Predict consumes one owned copy of the adapter's balances. The same route
// transition is used by PredictAdapters; a single adapter needs no address maps.
func Predict(demands []Demand, state *State) Prediction {
	if len(demands) == 0 {
		return Prediction{}
	}
	var free, withdrawable *big.Int
	acquire := make(map[common.Address]*big.Int)
	if state != nil {
		free, withdrawable = bigmath.Clone(state.FreeAssets), bigmath.Clone(state.Withdrawable)
		for token, amount := range state.Acquire {
			acquire[token] = bigmath.Clone(amount)
		}
	}
	out := Prediction{Routes: make([]Route, len(demands))}
	for i, demand := range demands {
		route := predictRoute(demand.AmountOut, demand.Collateral, acquire, free, withdrawable)
		out.Routes[i] = route
		out.Units = bigmath.SaturatingAdd(out.Units, UnitsForRouteAt(route, i == 0))
	}
	return out
}

// Each row is the measured first/subsequent swap cost. Unknown route values
// retain the conservative ceiling used for an unavailable liquidity snapshot.
var routeCosts = [...]struct {
	name              string
	first, additional uint64
}{
	RouteUnknown:    {"unknown", 850_000, 650_000},
	RouteAcquire:    {"acquire", 300_000, 140_000},
	RouteAllocate:   {"allocate", 530_000, 350_000},
	RouteDeallocate: {"deallocate", 650_000, 450_000},
}

func UnitsForRouteAt(route Route, first bool) uint64 {
	if int(route) >= len(routeCosts) {
		route = RouteUnknown
	}
	if first {
		return routeCosts[route].first
	}
	return routeCosts[route].additional
}
