package rfq

import (
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// fillPlanReservations maps each leg to the capacity of the candidate it spends. Output is reserved
// per physical vault capacity. A discount is not held: the adapter never consumes its nonce, so it
// stays usable by other fills until revoked or expired.
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
