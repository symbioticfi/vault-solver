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

	capacityIDs := make([]liquidlane.CapacityID, 0, len(groups))
	for capacityID := range groups {
		capacityIDs = append(capacityIDs, capacityID)
	}
	slices.Sort(capacityIDs)

	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, capacityID := range capacityIDs {
		routes := groups[capacityID]
		routeIDs := make([]liquidlane.RouteID, 0, len(routes))
		for routeID := range routes {
			routeIDs = append(routeIDs, routeID)
		}
		slices.Sort(routeIDs)
		domainMax := new(big.Int)
		for _, items := range routes {
			for _, item := range items {
				if item.MaxAssets.Cmp(domainMax) > 0 {
					domainMax.Set(item.MaxAssets)
				}
			}
		}
		remaining := AvailableCapacity(domainMax, reserveBps)
		if reserved := reservations[capacityID]; reserved != nil && reserved.Sign() > 0 {
			remaining.Sub(remaining, reserved)
		}
		if remaining.Sign() <= 0 {
			continue
		}

		for index, routeID := range routeIDs {
			items := routes[routeID]
			share := liquidlane.MulDivUp(remaining, big.NewInt(1), big.NewInt(int64(len(routeIDs)-index)))
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
			if share.Sign() <= 0 {
				continue
			}
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
			remaining.Sub(remaining, share)
			if remaining.Sign() <= 0 {
				break
			}
		}
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
