package planning

import (
	"maps"
	"math/big"
	"slices"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// FilterLiveInventory removes expired and duplicate route alternatives.
func FilterLiveInventory(inventory []liquidlane.Inventory, validAfter time.Time) []liquidlane.Inventory {
	seen := make(map[liquidlane.CandidateID]bool, len(inventory))
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, candidate := range inventory {
		id := liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID)
		if (!candidate.ValidUntil.IsZero() && !candidate.ValidUntil.After(validAfter)) || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, candidate)
	}
	return out
}

// AllocateInventoryCapacity partitions each vault's available output among physical routes.
// A route's direct/private alternatives share that partition and never multiply vault capacity.
func AllocateInventoryCapacity(inventory []liquidlane.Inventory, reservations liquidlane.CapacityReservations, reserveBps int) []liquidlane.Inventory {
	type routeBudget struct {
		alternatives []liquidlane.Inventory
		capacity     *big.Int
	}
	type vaultBudget struct {
		routes    map[liquidlane.RouteID]*routeBudget
		remaining *big.Int
	}
	vaults := make(map[liquidlane.CapacityID]*vaultBudget)
	for _, item := range inventory {
		capacity := AvailableCapacity(item.MaxAssets, reserveBps)
		if capacity.Sign() <= 0 {
			continue
		}
		id := liquidlane.RouteCapacityID(item.Route)
		vault := vaults[id]
		if vault == nil {
			vault = &vaultBudget{routes: make(map[liquidlane.RouteID]*routeBudget), remaining: new(big.Int)}
			vaults[id] = vault
		}
		route := vault.routes[item.ID]
		if route == nil {
			route = &routeBudget{capacity: new(big.Int)}
			vault.routes[item.ID] = route
		}
		if capacity.Cmp(route.capacity) > 0 {
			route.capacity.Set(capacity)
		}
		if capacity.Cmp(vault.remaining) > 0 {
			vault.remaining.Set(capacity)
		}
		item.MaxAssets, item.MaxRate, item.DiscountID = capacity, bigmath.Clone(item.MaxRate), liquidlane.CloneHash(item.DiscountID)
		route.alternatives = append(route.alternatives, item)
	}
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, id := range slices.Sorted(maps.Keys(vaults)) {
		vault := vaults[id]
		if reserved := reservations[id]; reserved != nil && reserved.Sign() > 0 {
			vault.remaining.Sub(vault.remaining, reserved)
		}
		routes := slices.Sorted(maps.Keys(vault.routes))
		for index, routeID := range routes {
			if vault.remaining.Sign() <= 0 {
				break
			}
			route := vault.routes[routeID]
			share := liquidlane.MulDivUp(vault.remaining, big.NewInt(1), big.NewInt(int64(len(routes)-index)))
			share = bigmath.Min(share, route.capacity)
			for _, item := range route.alternatives {
				item.MaxAssets = bigmath.Min(item.MaxAssets, share)
				out = append(out, item)
			}
			vault.remaining.Sub(vault.remaining, share)
		}
	}
	return out
}

func AvailableCapacity(maxAssets *big.Int, reserveBps int) *big.Int {
	return applyBpsDown(maxAssets, bpsDenominator-reserveBps)
}
