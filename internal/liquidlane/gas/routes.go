package gas

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Route describes the LiquidLane adapter path a swap is expected to take.
type Route uint8

const (
	RouteUnknown Route = iota
	RouteAcquire
	RouteAllocate
	RouteDeallocate
)

// State is the minimal LiquidLane liquidity snapshot needed for route prediction.
type State struct {
	FreeAssets   *big.Int
	Withdrawable *big.Int
	Acquire      map[common.Address]*big.Int
}

// Demand is one expected loan-token output from a swap through a LiquidLane adapter.
type Demand struct {
	Collateral common.Address
	AmountOut  *big.Int
}

// PredictRoutes estimates the adapter route for each demand in order.
func PredictRoutes(demands []Demand, st *State) []Route {
	if len(demands) == 0 {
		return nil
	}
	routes := make([]Route, 0, len(demands))
	if st == nil || st.FreeAssets == nil || st.Withdrawable == nil {
		for range demands {
			routes = append(routes, RouteUnknown)
		}
		return routes
	}
	acquire := make(map[common.Address]*big.Int, len(st.Acquire))
	for k, v := range st.Acquire {
		acquire[k] = cloneBig(v)
	}
	free := cloneBig(st.FreeAssets)
	withdrawable := cloneBig(st.Withdrawable)
	for _, demand := range demands {
		routes = append(routes, predictRoute(demand.AmountOut, demand.Collateral, acquire, free, withdrawable))
	}
	return routes
}

func predictRoute(amountOut *big.Int, collateral common.Address, acquire map[common.Address]*big.Int, free, withdrawable *big.Int) Route {
	if amountOut == nil || amountOut.Sign() <= 0 || free == nil || withdrawable == nil {
		return RouteUnknown
	}
	remaining := new(big.Int).Set(amountOut)
	if a := acquire[collateral]; a != nil && a.Sign() > 0 {
		used := minBig(remaining, a)
		remaining.Sub(remaining, used)
		a.Sub(a, used)
	}
	if remaining.Sign() == 0 {
		return RouteAcquire
	}
	if free.Cmp(remaining) >= 0 {
		free.Sub(free, remaining)
		if withdrawable.Cmp(remaining) >= 0 {
			withdrawable.Sub(withdrawable, remaining)
		} else {
			withdrawable.SetInt64(0)
		}
		return RouteAllocate
	}
	if withdrawable.Cmp(remaining) >= 0 {
		withdrawable.Sub(withdrawable, remaining)
		free.SetInt64(0)
		return RouteDeallocate
	}
	return RouteUnknown
}

func cloneBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func (r Route) String() string {
	switch r {
	case RouteAcquire:
		return "acquire"
	case RouteAllocate:
		return "allocate"
	case RouteDeallocate:
		return "deallocate"
	case RouteUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func RoutesString(routes []Route) string {
	if len(routes) == 0 {
		return ""
	}
	out := make([]string, len(routes))
	for i, r := range routes {
		out[i] = r.String()
	}
	return strings.Join(out, ",")
}
