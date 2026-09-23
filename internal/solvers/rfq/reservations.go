package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// discountReservationID marks a single-use signed discount as taken by a pending fill.
func discountReservationID(id common.Hash) liquidlane.CapacityID {
	return liquidlane.CapacityID("discount:" + id.Hex())
}

// withoutReservedDiscounts drops inventory whose discount a pending fill already holds.
func withoutReservedDiscounts(
	inventory []solverInventory, reservations liquidlane.CapacityReservations,
) []solverInventory {
	out := make([]solverInventory, 0, len(inventory))
	for _, item := range inventory {
		if item.DiscountID != nil && reservations[discountReservationID(*item.DiscountID)] != nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

// fillPlanReservations maps each leg to the capacity of the candidate it spends. Output is reserved
// per physical vault capacity; a discount leg also takes its single-use discount.
func fillPlanReservations(
	plan *fillPlan, candidates []liquidlane.QuoteCandidate,
) (liquidlane.CapacityReservations, error) {
	if len(plan.Legs) == 0 {
		return nil, errors.New("fill: plan has no legs to reserve")
	}
	reservations := make(liquidlane.CapacityReservations)
	for _, leg := range plan.Legs {
		if leg.AmountOut == nil || leg.AmountOut.Sign() <= 0 {
			return nil, errors.Errorf("fill: leg through %s has no output to reserve", leg.Adapter.Hex())
		}
		candidate, ok := legCandidate(leg, candidates)
		if !ok {
			return nil, errors.Errorf("fill: leg through %s matches no planned candidate", leg.Adapter.Hex())
		}
		reservations.Add(liquidlane.RouteCapacityID(candidate.Route), leg.AmountOut)
		if leg.DiscountID != nil {
			reservations.Add(discountReservationID(*leg.DiscountID), big.NewInt(1))
		}
	}
	return reservations, nil
}

func legCandidate(leg fillLeg, candidates []liquidlane.QuoteCandidate) (liquidlane.QuoteCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.Route.Adapter != leg.Adapter {
			continue
		}
		if (candidate.DiscountID == nil) == (leg.DiscountID == nil) &&
			(leg.DiscountID == nil || *candidate.DiscountID == *leg.DiscountID) {
			return candidate, true
		}
	}
	return liquidlane.QuoteCandidate{}, false
}
