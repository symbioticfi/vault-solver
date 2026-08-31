package gas_test

import (
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

var (
	adapter    = common.HexToAddress("0x00000000000000000000000000000000000000a1")
	vault      = common.HexToAddress("0x00000000000000000000000000000000000000f1")
	otherVault = common.HexToAddress("0x00000000000000000000000000000000000000f2")
	collateral = common.HexToAddress("0x00000000000000000000000000000000000000c1")
	otherColl  = common.HexToAddress("0x00000000000000000000000000000000000000c2")
)

func TestPredictAdaptersSingleAdapterCharacterization(t *testing.T) {
	transitionAcquire := big.NewInt(100)
	transitionFree := big.NewInt(80)
	transitionWithdrawable := big.NewInt(200)
	transitionAmounts := []*big.Int{big.NewInt(70), big.NewInt(70), big.NewInt(70), big.NewInt(100)}
	transitionSnapshot := snapshot(transitionAcquire, transitionFree, transitionWithdrawable)
	transitionDemands := demands(collateral, transitionAmounts...)

	tests := []struct {
		name       string
		demands    []gas.AdapterDemand
		snapshot   *gas.Snapshot
		wantRoutes []gas.Route
		wantUnits  uint64
		check      func(*testing.T)
	}{
		{
			name:       "nil snapshot",
			demands:    demands(collateral, big.NewInt(100)),
			wantRoutes: []gas.Route{gas.RouteUnknown},
			wantUnits:  850_000,
		},
		{
			name:    "nil adapter state",
			demands: demands(collateral, big.NewInt(100)),
			snapshot: &gas.Snapshot{
				Adapters: map[common.Address]*gas.AdapterState{adapter: nil},
				Vaults: map[common.Address]*gas.VaultState{
					vault: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(100)},
				},
			},
			wantRoutes: []gas.Route{gas.RouteUnknown},
			wantUnits:  850_000,
		},
		{
			name:    "incomplete vault state",
			demands: demands(collateral, big.NewInt(100)),
			snapshot: &gas.Snapshot{
				Adapters: map[common.Address]*gas.AdapterState{
					adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{collateral: big.NewInt(100)}},
				},
				Vaults: map[common.Address]*gas.VaultState{
					vault: {FreeAssets: big.NewInt(100)},
				},
			},
			wantRoutes: []gas.Route{gas.RouteUnknown},
			wantUnits:  850_000,
		},
		{
			name:    "adapter vault mismatch",
			demands: demands(collateral, big.NewInt(100)),
			snapshot: &gas.Snapshot{
				Adapters: map[common.Address]*gas.AdapterState{
					adapter: {Vault: otherVault, Acquire: map[common.Address]*big.Int{collateral: big.NewInt(100)}},
				},
				Vaults: map[common.Address]*gas.VaultState{
					vault: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(100)},
				},
			},
			wantRoutes: []gas.Route{gas.RouteUnknown},
			wantUnits:  850_000,
		},
		{
			name: "invalid amounts preserve liquidity for later demand",
			demands: demands(collateral,
				nil,
				big.NewInt(0),
				big.NewInt(-1),
				big.NewInt(100),
			),
			snapshot:   snapshot(big.NewInt(100), big.NewInt(0), big.NewInt(0)),
			wantRoutes: []gas.Route{gas.RouteUnknown, gas.RouteUnknown, gas.RouteUnknown, gas.RouteAcquire},
			wantUnits:  2_290_000,
		},
		{
			name:       "repeated collateral consumes acquire then shared budgets",
			demands:    transitionDemands,
			snapshot:   transitionSnapshot,
			wantRoutes: []gas.Route{gas.RouteAcquire, gas.RouteAllocate, gas.RouteDeallocate, gas.RouteUnknown},
			wantUnits:  1_750_000,
			check: func(t *testing.T) {
				t.Helper()
				adapterState := transitionSnapshot.Adapters[adapter]
				vaultState := transitionSnapshot.Vaults[vault]
				if len(transitionSnapshot.Adapters) != 1 || len(transitionSnapshot.Vaults) != 1 ||
					adapterState == nil || adapterState.Vault != vault || len(adapterState.Acquire) != 1 || vaultState == nil {
					t.Fatalf("source maps changed: %+v", transitionSnapshot)
				}
				if adapterState.Acquire[collateral].String() != "100" || vaultState.FreeAssets.String() != "80" ||
					vaultState.Withdrawable.String() != "200" || transitionAcquire.String() != "100" ||
					transitionFree.String() != "80" || transitionWithdrawable.String() != "200" {
					t.Fatalf("source snapshot integers changed: acquire=%s free=%s withdrawable=%s",
						adapterState.Acquire[collateral], vaultState.FreeAssets, vaultState.Withdrawable)
				}
				for i, demand := range transitionDemands {
					want := []string{"70", "70", "70", "100"}[i]
					if demand.AmountOut.String() != want || transitionAmounts[i].String() != want {
						t.Fatalf("source demand %d changed to %s, want %s", i, demand.AmountOut, want)
					}
				}
			},
		},
		{
			name:       "demand order acquire then allocate",
			demands:    append(demands(collateral, big.NewInt(60)), demands(otherColl, big.NewInt(60))...),
			snapshot:   snapshot(big.NewInt(100), big.NewInt(100), big.NewInt(200)),
			wantRoutes: []gas.Route{gas.RouteAcquire, gas.RouteAllocate},
			wantUnits:  650_000,
		},
		{
			name:       "demand order allocate then acquire",
			demands:    append(demands(otherColl, big.NewInt(60)), demands(collateral, big.NewInt(60))...),
			snapshot:   snapshot(big.NewInt(100), big.NewInt(100), big.NewInt(200)),
			wantRoutes: []gas.Route{gas.RouteAllocate, gas.RouteAcquire},
			wantUnits:  670_000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := gas.PredictAdapters(test.demands, test.snapshot)
			if !slices.Equal(got.Routes, test.wantRoutes) {
				t.Fatalf("routes = %v, want %v", got.Routes, test.wantRoutes)
			}
			if got.Units != test.wantUnits {
				t.Fatalf("units = %d, want %d", got.Units, test.wantUnits)
			}
			if test.check != nil {
				test.check(t)
			}
		})
	}
}

func snapshot(acquire, free, withdrawable *big.Int) *gas.Snapshot {
	return &gas.Snapshot{
		Adapters: map[common.Address]*gas.AdapterState{
			adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{collateral: acquire}},
		},
		Vaults: map[common.Address]*gas.VaultState{
			vault: {FreeAssets: free, Withdrawable: withdrawable},
		},
	}
}

func demands(coll common.Address, amounts ...*big.Int) []gas.AdapterDemand {
	out := make([]gas.AdapterDemand, len(amounts))
	for i, amount := range amounts {
		out[i] = gas.AdapterDemand{
			Adapter: adapter,
			Vault:   vault,
			Demand:  gas.Demand{Collateral: coll, AmountOut: amount},
		}
	}
	return out
}
