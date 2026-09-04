package gas

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var routeGas = map[Route][2]uint64{
	RouteAcquire:    {300_000, 140_000},
	RouteAllocate:   {530_000, 350_000},
	RouteDeallocate: {650_000, 450_000},
	RouteUnknown:    {850_000, 650_000},
}

const (
	firstUnknownSwap      uint64 = 850_000
	additionalUnknownSwap uint64 = 650_000
)

func Predict(demands []Demand, state *State) Prediction {
	routes := PredictRoutes(demands, state)
	return Prediction{Units: RouteUnits(routes), Routes: routes}
}

func PredictRoutes(demands []Demand, state *State) []Route {
	if len(demands) == 0 {
		return nil
	}
	budget := newBudget(state)
	routes := make([]Route, len(demands))
	for index, demand := range demands {
		routes[index] = budget.consume(demand.Collateral, demand.AmountOut)
	}
	return routes
}

func PredictAdapters(demands []AdapterDemand, snapshot *Snapshot) Prediction {
	if len(demands) == 0 {
		return Prediction{}
	}
	adapters, vaults := copySnapshot(snapshot)
	seenAdapters := make(map[common.Address]struct{}, len(adapters))
	prediction := Prediction{Routes: make([]Route, 0, len(demands))}
	for _, demand := range demands {
		route := RouteUnknown
		adapter := adapters[demand.Adapter]
		vault := vaults[demand.Vault]
		if adapter != nil && adapter.Vault == demand.Vault && vault != nil {
			budget := liquidityBudget{acquire: adapter.Acquire, free: vault.FreeAssets, withdrawable: vault.Withdrawable}
			route = budget.consume(demand.Collateral, demand.AmountOut)
		}
		_, repeated := seenAdapters[demand.Adapter]
		seenAdapters[demand.Adapter] = struct{}{}
		prediction.Units = saturatingAddUint64(prediction.Units, UnitsForRouteAt(route, !repeated))
		prediction.Routes = append(prediction.Routes, route)
	}
	return prediction
}

func RouteUnits(routes []Route) uint64 {
	var units uint64
	for index, route := range routes {
		units = saturatingAddUint64(units, UnitsForRouteAt(route, index == 0))
	}
	return units
}

func UnitsForRouteAt(route Route, first bool) uint64 {
	prices, valid := routeGas[route]
	if !valid {
		prices = routeGas[RouteUnknown]
	}
	if first {
		return prices[0]
	}
	return prices[1]
}

func WithReserveBps(snapshot *Snapshot, reserveBps int) *Snapshot {
	adapters, vaults := copySnapshot(snapshot)
	reserveBps = max(0, min(reserveBps, 10_000))
	remaining := int64(10_000 - reserveBps)
	for _, adapter := range adapters {
		for token, amount := range adapter.Acquire {
			adapter.Acquire[token] = scaleBps(amount, remaining)
		}
	}
	for _, vault := range vaults {
		vault.FreeAssets = scaleBps(vault.FreeAssets, remaining)
		vault.Withdrawable = scaleBps(vault.Withdrawable, remaining)
	}
	return &Snapshot{Adapters: adapters, Vaults: vaults}
}

type liquidityBudget struct {
	acquire      map[common.Address]*big.Int
	free         *big.Int
	withdrawable *big.Int
}

func newBudget(state *State) liquidityBudget {
	if state == nil {
		return liquidityBudget{}
	}
	acquire := make(map[common.Address]*big.Int, len(state.Acquire))
	for token, amount := range state.Acquire {
		acquire[token] = copyBig(amount)
	}
	return liquidityBudget{acquire: acquire, free: copyBig(state.FreeAssets), withdrawable: copyBig(state.Withdrawable)}
}

func (budget liquidityBudget) consume(token common.Address, amount *big.Int) Route {
	if amount == nil || amount.Sign() <= 0 || budget.free == nil || budget.withdrawable == nil {
		return RouteUnknown
	}
	remaining := new(big.Int).Set(amount)
	if available := budget.acquire[token]; available != nil && available.Sign() > 0 {
		used := new(big.Int).Set(remaining)
		if available.Cmp(used) < 0 {
			used.Set(available)
		}
		remaining.Sub(remaining, used)
		available.Sub(available, used)
	}
	if remaining.Sign() == 0 {
		return RouteAcquire
	}
	if budget.free.Cmp(remaining) >= 0 {
		budget.free.Sub(budget.free, remaining)
		consumeFloor(budget.withdrawable, remaining)
		return RouteAllocate
	}
	if budget.withdrawable.Cmp(remaining) >= 0 {
		budget.withdrawable.Sub(budget.withdrawable, remaining)
		budget.free.SetInt64(0)
		return RouteDeallocate
	}
	return RouteUnknown
}

func consumeFloor(available, amount *big.Int) {
	if available.Cmp(amount) >= 0 {
		available.Sub(available, amount)
	} else {
		available.SetInt64(0)
	}
}

func copySnapshot(snapshot *Snapshot) (map[common.Address]*AdapterState, map[common.Address]*VaultState) {
	if snapshot == nil {
		return nil, nil
	}
	adapters := make(map[common.Address]*AdapterState, len(snapshot.Adapters))
	for address, source := range snapshot.Adapters {
		if source == nil {
			continue
		}
		acquire := make(map[common.Address]*big.Int, len(source.Acquire))
		for token, amount := range source.Acquire {
			acquire[token] = copyBig(amount)
		}
		adapters[address] = &AdapterState{Vault: source.Vault, Acquire: acquire}
	}
	vaults := make(map[common.Address]*VaultState, len(snapshot.Vaults))
	for address, source := range snapshot.Vaults {
		if source != nil && source.FreeAssets != nil && source.Withdrawable != nil {
			vaults[address] = &VaultState{FreeAssets: copyBig(source.FreeAssets), Withdrawable: copyBig(source.Withdrawable)}
		}
	}
	return adapters, vaults
}

func scaleBps(amount *big.Int, bps int64) *big.Int {
	if amount == nil || amount.Sign() <= 0 || bps <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Quo(new(big.Int).Mul(amount, big.NewInt(bps)), big.NewInt(10_000))
}

func copyBig(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func saturatingAddUint64(left, right uint64) uint64 {
	if right > ^uint64(0)-left {
		return ^uint64(0)
	}
	return left + right
}
