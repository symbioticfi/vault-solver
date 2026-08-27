package greedy

import (
	"math/big"
	"slices"
	"time"

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

// AllocateInventoryCapacity divides shared vault capacity between physical routes.
func AllocateInventoryCapacity(
	inventory []liquidlane.Inventory,
	reservations liquidlane.CapacityReservations,
	reserveBps int,
) []liquidlane.Inventory {
	groups := groupInventoryByCapacity(inventory)
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, capacityID := range sortedCapacityIDs(groups) {
		allocated := allocateCapacityGroup(groups[capacityID], reservations[capacityID], reserveBps)
		out = append(out, allocated...)
	}
	return out
}

func groupInventoryByCapacity(
	inventory []liquidlane.Inventory,
) map[liquidlane.CapacityID]map[liquidlane.RouteID][]liquidlane.Inventory {
	groups := make(map[liquidlane.CapacityID]map[liquidlane.RouteID][]liquidlane.Inventory)
	for _, item := range inventory {
		if item.MaxAssets == nil || item.MaxAssets.Sign() <= 0 {
			continue
		}
		capacityID := liquidlane.RouteCapacityID(item.Route)
		if groups[capacityID] == nil {
			groups[capacityID] = make(map[liquidlane.RouteID][]liquidlane.Inventory)
		}
		groups[capacityID][item.ID] = append(groups[capacityID][item.ID], item)
	}
	return groups
}

func sortedCapacityIDs(
	groups map[liquidlane.CapacityID]map[liquidlane.RouteID][]liquidlane.Inventory,
) []liquidlane.CapacityID {
	capacityIDs := make([]liquidlane.CapacityID, 0, len(groups))
	for capacityID := range groups {
		capacityIDs = append(capacityIDs, capacityID)
	}
	slices.Sort(capacityIDs)
	return capacityIDs
}

func allocateCapacityGroup(
	routes map[liquidlane.RouteID][]liquidlane.Inventory,
	reserved *big.Int,
	reserveBps int,
) []liquidlane.Inventory {
	routeIDs := sortedRouteIDs(routes)
	remaining := groupAvailableCapacity(routes, reserved, reserveBps)
	if remaining.Sign() <= 0 {
		return nil
	}

	out := make([]liquidlane.Inventory, 0, len(routes))
	for index, routeID := range routeIDs {
		items := routes[routeID]
		share := routeCapacityShare(remaining, items, len(routeIDs)-index, reserveBps)
		if share.Sign() <= 0 {
			continue
		}
		out = append(out, capRouteInventory(items, share, reserveBps)...)
		remaining.Sub(remaining, share)
		if remaining.Sign() <= 0 {
			break
		}
	}
	return out
}

func sortedRouteIDs(routes map[liquidlane.RouteID][]liquidlane.Inventory) []liquidlane.RouteID {
	routeIDs := make([]liquidlane.RouteID, 0, len(routes))
	for routeID := range routes {
		routeIDs = append(routeIDs, routeID)
	}
	slices.Sort(routeIDs)
	return routeIDs
}

func groupAvailableCapacity(
	routes map[liquidlane.RouteID][]liquidlane.Inventory,
	reserved *big.Int,
	reserveBps int,
) *big.Int {
	domainMax := new(big.Int)
	for _, items := range routes {
		for _, item := range items {
			if item.MaxAssets.Cmp(domainMax) > 0 {
				domainMax.Set(item.MaxAssets)
			}
		}
	}
	remaining := AvailableCapacity(domainMax, reserveBps)
	if reserved != nil && reserved.Sign() > 0 {
		remaining.Sub(remaining, reserved)
	}
	return remaining
}

func routeCapacityShare(
	remaining *big.Int,
	items []liquidlane.Inventory,
	routesLeft int,
	reserveBps int,
) *big.Int {
	share := liquidlane.MulDivUp(remaining, big.NewInt(1), big.NewInt(int64(routesLeft)))
	routeCap := new(big.Int)
	for _, item := range items {
		itemCap := AvailableCapacity(item.MaxAssets, reserveBps)
		if itemCap.Cmp(routeCap) > 0 {
			routeCap.Set(itemCap)
		}
	}
	if share.Cmp(routeCap) > 0 {
		share.Set(routeCap)
	}
	return share
}

func capRouteInventory(
	items []liquidlane.Inventory,
	share *big.Int,
	reserveBps int,
) []liquidlane.Inventory {
	out := make([]liquidlane.Inventory, 0, len(items))
	for _, item := range items {
		itemCap := AvailableCapacity(item.MaxAssets, reserveBps)
		if itemCap.Cmp(share) > 0 {
			itemCap.Set(share)
		}
		if itemCap.Sign() <= 0 {
			continue
		}
		item.MaxAssets = itemCap
		item.MaxRate = liquidlane.CloneBig(item.MaxRate)
		item.DiscountID = liquidlane.CloneHash(item.DiscountID)
		out = append(out, item)
	}
	return out
}

func AvailableCapacity(maxAssets *big.Int, reserveBps int) *big.Int {
	return applyBpsDown(maxAssets, bpsDenominator-reserveBps)
}

func QuoteCapacity(route liquidlane.Inventory, priceBufferBps int) *big.Int {
	if route.DiscountID == nil {
		return liquidlane.CloneBig(route.MaxAssets)
	}
	return applyBpsDown(route.MaxAssets, bpsDenominator-priceBufferBps)
}
