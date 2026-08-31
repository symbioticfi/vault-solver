// Package gas provides LiquidLane route gas prediction and Chainlink-backed gas conversion facts.
//
// It is intentionally limited to LiquidLane adapter swap accounting: callers provide
// expected swap demands plus a compact adapter liquidity snapshot, and the package
// returns route labels and route gas units. Solver-specific settlement and payload overhead,
// auction/executor gas limits, price updates, bids, and economics stay outside.
package gas

const (
	firstAcquireSwap         uint64 = 300_000
	additionalAcquireSwap    uint64 = 140_000
	firstAllocateSwap        uint64 = 530_000
	additionalAllocateSwap   uint64 = 350_000
	firstDeallocateSwap      uint64 = 650_000
	additionalDeallocateSwap uint64 = 450_000
	firstUnknownSwap         uint64 = 850_000
	additionalUnknownSwap    uint64 = 650_000
)

type Prediction struct {
	Units  uint64
	Routes []Route
}

// Predict returns LiquidLane adapter route gas for the demands in order.
func Predict(demands []Demand, st *State) Prediction {
	routes := PredictRoutes(demands, st)
	return Prediction{Units: RouteUnits(routes), Routes: routes}
}

func RouteUnits(routes []Route) uint64 {
	var total uint64
	for i, route := range routes {
		total = saturatingAddUint64(total, UnitsForRouteAt(route, i == 0))
	}
	return total
}

func UnitsForRouteAt(route Route, first bool) uint64 {
	switch route { //nolint:exhaustive // Unknown and invalid routes intentionally share the conservative default.
	case RouteAcquire:
		if first {
			return firstAcquireSwap
		}
		return additionalAcquireSwap
	case RouteAllocate:
		if first {
			return firstAllocateSwap
		}
		return additionalAllocateSwap
	case RouteDeallocate:
		if first {
			return firstDeallocateSwap
		}
		return additionalDeallocateSwap
	default:
		if first {
			return firstUnknownSwap
		}
		return additionalUnknownSwap
	}
}

func saturatingAddUint64(a, b uint64) uint64 {
	if b > ^uint64(0)-a {
		return ^uint64(0)
	}
	return a + b
}
