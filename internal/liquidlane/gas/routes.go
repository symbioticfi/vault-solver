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

// AdapterState contains liquidity owned by one adapter.
type AdapterState struct {
	Vault   common.Address              `json:"vault"`
	Acquire map[common.Address]*big.Int `json:"acquire"`
}

// VaultState contains liquidity shared by every adapter backed by the vault.
type VaultState struct {
	FreeAssets   *big.Int `json:"freeAssets"`
	Withdrawable *big.Int `json:"withdrawable"`
}

// Snapshot separates adapter-local acquire balances from shared vault liquidity.
type Snapshot struct {
	Adapters map[common.Address]*AdapterState `json:"adapters"`
	Vaults   map[common.Address]*VaultState   `json:"vaults"`
}

// Demand is one expected loan-token output from a swap through a LiquidLane adapter.
type Demand struct {
	Collateral common.Address
	AmountOut  *big.Int
}

// AdapterDemand is one expected swap output scoped to its LiquidLane adapter and shared vault.
type AdapterDemand struct {
	Demand

	Adapter common.Address
	Vault   common.Address
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

// PredictAdapters predicts swap routes for a multi-adapter transaction. Acquire balances are consumed
// per adapter while free and withdrawable liquidity is consumed once across adapters sharing a vault.
func PredictAdapters(demands []AdapterDemand, snapshot *Snapshot) Prediction {
	if len(demands) == 0 {
		return Prediction{}
	}
	adapters, vaults := cloneSnapshot(snapshot)
	seen := make(map[common.Address]bool, len(adapters))
	routes := make([]Route, 0, len(demands))
	var units uint64
	for _, demand := range demands {
		route := RouteUnknown
		adapterState := adapters[demand.Adapter]
		vaultState := vaults[demand.Vault]
		if adapterState != nil && adapterState.Vault == demand.Vault && vaultState != nil {
			route = predictRoute(
				demand.AmountOut,
				demand.Collateral,
				adapterState.Acquire,
				vaultState.FreeAssets,
				vaultState.Withdrawable,
			)
		}
		routes = append(routes, route)
		first := !seen[demand.Adapter]
		seen[demand.Adapter] = true
		units = saturatingAddUint64(units, UnitsForRouteAt(route, first))
	}
	return Prediction{Units: units, Routes: routes}
}

// WithReserveBps returns a conservative copy of snapshot with every mutable liquidity budget reduced.
func WithReserveBps(snapshot *Snapshot, reserveBps int) *Snapshot {
	adapters, vaults := cloneSnapshot(snapshot)
	if reserveBps <= 0 {
		return &Snapshot{Adapters: adapters, Vaults: vaults}
	}
	if reserveBps > 10_000 {
		reserveBps = 10_000
	}
	remainingBps := int64(10_000 - reserveBps)
	for _, state := range adapters {
		for token, amount := range state.Acquire {
			state.Acquire[token] = applyBpsDown(amount, remainingBps)
		}
	}
	for _, state := range vaults {
		state.FreeAssets = applyBpsDown(state.FreeAssets, remainingBps)
		state.Withdrawable = applyBpsDown(state.Withdrawable, remainingBps)
	}
	return &Snapshot{Adapters: adapters, Vaults: vaults}
}

func cloneSnapshot(snapshot *Snapshot) (map[common.Address]*AdapterState, map[common.Address]*VaultState) {
	if snapshot == nil {
		return nil, nil
	}
	adapters := make(map[common.Address]*AdapterState, len(snapshot.Adapters))
	for address, state := range snapshot.Adapters {
		if state == nil {
			continue
		}
		acquire := make(map[common.Address]*big.Int, len(state.Acquire))
		for token, amount := range state.Acquire {
			acquire[token] = cloneBig(amount)
		}
		adapters[address] = &AdapterState{Vault: state.Vault, Acquire: acquire}
	}
	vaults := make(map[common.Address]*VaultState, len(snapshot.Vaults))
	for address, state := range snapshot.Vaults {
		if state == nil || state.FreeAssets == nil || state.Withdrawable == nil {
			continue
		}
		vaults[address] = &VaultState{
			FreeAssets: cloneBig(state.FreeAssets), Withdrawable: cloneBig(state.Withdrawable),
		}
	}
	return adapters, vaults
}

func applyBpsDown(amount *big.Int, bps int64) *big.Int {
	if amount == nil || amount.Sign() <= 0 || bps <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(amount, big.NewInt(bps)), big.NewInt(10_000))
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
