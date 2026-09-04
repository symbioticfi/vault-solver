// Package liquidlane defines the shared LiquidLane read model and capacity accounting.
package liquidlane

import (
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/capacity"
)

type RouteID string
type CandidateID string
type CapacityID = capacity.ID
type CapacityReservations = capacity.Amounts

const DiscountPrecision int64 = 1_000_000

func NewCapacityID(chainID int64, vault, tokenOut common.Address) CapacityID {
	return CapacityID(lowerID("capacity", chainID, vault, tokenOut))
}

func NewRouteID(chainID int64, adapter, tokenIn, tokenOut common.Address) RouteID {
	return RouteID(lowerID("route", chainID, adapter, tokenIn, tokenOut))
}

func lowerID(prefix string, chainID int64, addresses ...common.Address) string {
	parts := make([]string, 0, 2+len(addresses))
	parts = append(parts, prefix, strconv.FormatInt(chainID, 10))
	for _, address := range addresses {
		parts = append(parts, address.Hex())
	}
	return strings.ToLower(strings.Join(parts, ":"))
}

func RouteCapacityID(route Route) CapacityID {
	if route.CapacityID != "" {
		return route.CapacityID
	}
	return CapacityID(route.ID)
}

func NewCandidateID(route Route, discountID *common.Hash) CandidateID {
	id := "candidate:" + string(route.ID)
	if discountID != nil {
		id += ":discount:" + discountID.Hex()
	}
	return CandidateID(strings.ToLower(id))
}
