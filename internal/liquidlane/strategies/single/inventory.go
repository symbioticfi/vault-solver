package single

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

// AllocateInventoryCapacity gives each alternative its available shared budget,
// bounded by its own limit. These alternatives must never be aggregated.
func AllocateInventoryCapacity(
	inventory []liquidlane.Inventory,
	reservations liquidlane.CapacityReservations,
	reserveBps int,
) []liquidlane.Inventory {
	budgets := make(map[liquidlane.CapacityID]*big.Int)
	for _, item := range inventory {
		if item.MaxAssets == nil || item.MaxAssets.Sign() <= 0 {
			continue
		}
		id := liquidlane.RouteCapacityID(item.Route)
		if budgets[id] == nil || item.MaxAssets.Cmp(budgets[id]) > 0 {
			budgets[id] = item.MaxAssets
		}
	}
	for id, maximum := range budgets {
		available := greedy.AvailableCapacity(maximum, reserveBps)
		if reserved := reservations[id]; reserved != nil && reserved.Sign() > 0 {
			available.Sub(available, reserved)
		}
		budgets[id] = available
	}
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if item.MaxAssets == nil || item.MaxAssets.Sign() <= 0 {
			continue
		}
		capacity := greedy.AvailableCapacity(item.MaxAssets, reserveBps)
		if budget := budgets[liquidlane.RouteCapacityID(item.Route)]; capacity.Cmp(budget) > 0 {
			capacity.Set(budget)
		}
		if capacity.Sign() <= 0 {
			continue
		}
		item.MaxAssets = capacity
		item.MaxRate = liquidlane.CloneBig(item.MaxRate)
		item.DiscountID = liquidlane.CloneHash(item.DiscountID)
		out = append(out, item)
	}
	return out
}
