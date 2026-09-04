package planning

import (
	"math/big"
	"slices"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// FilterLiveInventory removes expired and duplicate route alternatives.
func FilterLiveInventory(inventory []liquidlane.Inventory, validAfter time.Time) []liquidlane.Inventory {
	seen := make(map[liquidlane.CandidateID]struct{}, len(inventory))
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, candidate := range inventory {
		id := liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID)
		_, duplicate := seen[id]
		if duplicate || expiredAt(candidate.ValidUntil, validAfter) {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func expiredAt(deadline, instant time.Time) bool {
	return !deadline.IsZero() && !deadline.After(instant)
}

// AllocateInventoryCapacity divides shared vault capacity between physical routes.
func AllocateInventoryCapacity(
	inventory []liquidlane.Inventory,
	reservations liquidlane.CapacityReservations,
	reserveBps int,
) []liquidlane.Inventory {
	pools := buildCapacityPools(inventory)
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, capacityID := range sortedCapacityIDs(pools) {
		out = append(out, pools[capacityID].allocate(reservations[capacityID], reserveBps)...)
	}
	return out
}

type capacityPool map[liquidlane.RouteID][]liquidlane.Inventory

func buildCapacityPools(inventory []liquidlane.Inventory) map[liquidlane.CapacityID]capacityPool {
	pools := make(map[liquidlane.CapacityID]capacityPool)
	for _, item := range inventory {
		if item.MaxAssets == nil || item.MaxAssets.Sign() <= 0 {
			continue
		}
		capacityID := liquidlane.RouteCapacityID(item.Route)
		if pools[capacityID] == nil {
			pools[capacityID] = make(capacityPool)
		}
		pools[capacityID][item.ID] = append(pools[capacityID][item.ID], item)
	}
	return pools
}

func sortedCapacityIDs(
	pools map[liquidlane.CapacityID]capacityPool,
) []liquidlane.CapacityID {
	capacityIDs := make([]liquidlane.CapacityID, 0, len(pools))
	for capacityID := range pools {
		capacityIDs = append(capacityIDs, capacityID)
	}
	slices.Sort(capacityIDs)
	return capacityIDs
}

func (pool capacityPool) allocate(reserved *big.Int, reserveBps int) []liquidlane.Inventory {
	routeIDs := sortedRouteIDs(pool)
	remaining := pool.available(reserved, reserveBps)
	if remaining.Sign() <= 0 {
		return nil
	}

	out := make([]liquidlane.Inventory, 0, len(pool))
	for index, routeID := range routeIDs {
		items := pool[routeID]
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

func sortedRouteIDs(routes capacityPool) []liquidlane.RouteID {
	routeIDs := make([]liquidlane.RouteID, 0, len(routes))
	for routeID := range routes {
		routeIDs = append(routeIDs, routeID)
	}
	slices.Sort(routeIDs)
	return routeIDs
}

func (pool capacityPool) available(reserved *big.Int, reserveBps int) *big.Int {
	domainMax := new(big.Int)
	for _, items := range pool {
		for _, item := range items {
			if item.MaxAssets.Cmp(domainMax) > 0 {
				domainMax.Set(item.MaxAssets)
			}
		}
	}
	remaining := availableCapacity(domainMax, reserveBps)
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
		itemCap := availableCapacity(item.MaxAssets, reserveBps)
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
		itemCap := availableCapacity(item.MaxAssets, reserveBps)
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

func availableCapacity(maxAssets *big.Int, reserveBps int) *big.Int {
	return scaleDown(maxAssets, bpsDenominator-reserveBps)
}

func QuoteCapacity(route liquidlane.Inventory, priceBufferBps int) *big.Int {
	if route.DiscountID == nil {
		return liquidlane.CloneBig(route.MaxAssets)
	}
	return scaleDown(route.MaxAssets, bpsDenominator-priceBufferBps)
}
