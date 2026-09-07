package gas

import (
	"math/big"
	"strings"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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

// PredictAdapters predicts swap routes for a multi-adapter transaction. Acquire balances are consumed
// per adapter while free and withdrawable liquidity is consumed once across adapters sharing a vault.
func PredictAdapters(demands []AdapterDemand, snapshot *Snapshot) Prediction {
	if len(demands) == 0 {
		return Prediction{}
	}
	out := Prediction{Routes: make([]Route, 0, len(demands))}
	adapters := make(map[common.Address]*AdapterState)
	vaults := make(map[common.Address]*VaultState)
	seen := make(map[common.Address]bool)
	for _, demand := range demands {
		if snapshot != nil {
			if _, loaded := adapters[demand.Adapter]; !loaded {
				adapters[demand.Adapter] = cloneAdapter(snapshot.Adapters[demand.Adapter], 0)
			}
			if _, loaded := vaults[demand.Vault]; !loaded {
				vaults[demand.Vault] = cloneVault(snapshot.Vaults[demand.Vault], 0)
			}
		}
		route := RouteUnknown
		adapter, vault := adapters[demand.Adapter], vaults[demand.Vault]
		if adapter != nil && adapter.Vault == demand.Vault && vault != nil {
			route = predictRoute(demand.AmountOut, demand.Collateral, adapter.Acquire, vault.FreeAssets, vault.Withdrawable)
		}
		out.Routes = append(out.Routes, route)
		out.Units = bigmath.SaturatingAdd(out.Units, UnitsForRouteAt(route, !seen[demand.Adapter]))
		seen[demand.Adapter] = true
	}
	return out
}

// WithReserveBps owns one copy of each budget, applying the reserve while copying.
func WithReserveBps(snapshot *Snapshot, reserveBps int) *Snapshot {
	out := &Snapshot{}
	if snapshot == nil {
		return out
	}
	out.Adapters = make(map[common.Address]*AdapterState, len(snapshot.Adapters))
	out.Vaults = make(map[common.Address]*VaultState, len(snapshot.Vaults))
	for address, state := range snapshot.Adapters {
		if cloned := cloneAdapter(state, reserveBps); cloned != nil {
			out.Adapters[address] = cloned
		}
	}
	for address, state := range snapshot.Vaults {
		if cloned := cloneVault(state, reserveBps); cloned != nil {
			out.Vaults[address] = cloned
		}
	}
	return out
}

func cloneAdapter(state *AdapterState, reserveBps int) *AdapterState {
	if state == nil {
		return nil
	}
	out := &AdapterState{Vault: state.Vault, Acquire: make(map[common.Address]*big.Int, len(state.Acquire))}
	for token, amount := range state.Acquire {
		out.Acquire[token] = reservedBalance(amount, reserveBps)
	}
	return out
}

func cloneVault(state *VaultState, reserveBps int) *VaultState {
	if state == nil || state.FreeAssets == nil || state.Withdrawable == nil {
		return nil
	}
	return &VaultState{FreeAssets: reservedBalance(state.FreeAssets, reserveBps), Withdrawable: reservedBalance(state.Withdrawable, reserveBps)}
}

func reservedBalance(amount *big.Int, reserveBps int) *big.Int {
	if reserveBps <= 0 {
		return bigmath.Clone(amount)
	}
	if amount == nil || amount.Sign() <= 0 || reserveBps >= 10_000 {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(amount, big.NewInt(int64(10_000-reserveBps))), big.NewInt(10_000))
}

func predictRoute(amountOut *big.Int, collateral common.Address, acquire map[common.Address]*big.Int, free, withdrawable *big.Int) Route {
	if amountOut == nil || amountOut.Sign() <= 0 || free == nil || withdrawable == nil {
		return RouteUnknown
	}
	remaining := amountOut
	if a := acquire[collateral]; a != nil && a.Sign() > 0 {
		if a.Cmp(remaining) >= 0 {
			a.Sub(a, remaining)
			return RouteAcquire
		}
		remaining = new(big.Int).Sub(remaining, a)
		a.SetInt64(0)
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

func (r Route) String() string {
	if int(r) >= len(routeCosts) {
		return routeCosts[RouteUnknown].name
	}
	return routeCosts[r].name
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
