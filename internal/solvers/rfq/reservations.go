package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func discountReservationID(id common.Hash) liquidlane.CapacityID {
	return liquidlane.CapacityID("discount:" + id.Hex())
}

func exclusiveReservationID(id liquidlane.CapacityID) liquidlane.CapacityID {
	return "exclusive:" + id
}

// Candidates already divide a vault's capacity among its routes. Subtracting a
// pending fill from each route can underallocate a shared vault, but never offers
// the same capacity twice. A discount is exact-input with live oracle pricing;
// reserve its whole capacity domain until settlement rather than predicting its
// eventual output from a quote. Direct swaps have an exact output to reserve.
func candidatesAfterReservations(
	candidates []liquidlane.QuoteCandidate, reserved liquidlane.CapacityReservations,
) []liquidlane.QuoteCandidate {
	out := make([]liquidlane.QuoteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		capacityID := liquidlane.RouteCapacityID(candidate.Route)
		if reserved[exclusiveReservationID(capacityID)] != nil {
			continue
		}
		pending := reserved[capacityID]
		if candidate.DiscountID != nil &&
			(reserved[discountReservationID(*candidate.DiscountID)] != nil || pending != nil) {
			continue
		}
		if candidate.MaxAmountOut == nil || candidate.MaxAmountIn == nil {
			continue
		}
		if pending != nil {
			candidate.MaxAmountOut = new(big.Int).Sub(candidate.MaxAmountOut, pending)
			candidate.MaxAmountIn = new(big.Int).Set(candidate.MaxAmountIn)
			maxIn := liquidlane.MaxAmountInForRate(candidate.MaxAmountOut, candidate.Rate,
				candidate.Route.TokenInDecimals, candidate.Route.TokenOutDecimals)
			if maxIn.Cmp(candidate.MaxAmountIn) < 0 {
				candidate.MaxAmountIn.Set(maxIn)
			}
		}
		if candidate.MaxAmountOut.Sign() > 0 && candidate.MaxAmountIn.Sign() > 0 {
			out = append(out, candidate)
		}
	}
	return out
}

// fillReservations checks the strategy's selected legs against the exact fresh
// candidates used for planning. Reservations cannot rely on a strategy honoring
// capacity bounds, especially when the strategy is an external webhook.
func fillReservations(
	plan *fillPlan, candidates []liquidlane.QuoteCandidate,
) (liquidlane.CapacityReservations, error) {
	reserved := make(liquidlane.CapacityReservations)
	usedIn := make(map[int]*big.Int)
	usedOut := make(map[int]*big.Int)
	selectedRoutes := make(map[liquidlane.RouteID]int)
	for _, leg := range plan.Legs {
		index := matchingCandidate(leg, candidates)
		if index < 0 || leg.AmountIn == nil || leg.AmountOut == nil ||
			leg.AmountIn.Sign() <= 0 || leg.AmountOut.Sign() <= 0 {
			return nil, errors.New("fill: plan contains an unavailable route or invalid amount")
		}
		candidate := candidates[index]
		if candidate.Route.TokenIn != plan.TokenIn || candidate.Route.TokenOut != plan.TokenOut {
			return nil, errors.New("fill: plan tokens do not match reserved route")
		}
		if previous, ok := selectedRoutes[candidate.Route.ID]; ok && previous != index {
			return nil, errors.New("fill: plan combines mutually exclusive route alternatives")
		}
		selectedRoutes[candidate.Route.ID] = index
		if usedIn[index] == nil {
			usedIn[index], usedOut[index] = new(big.Int), new(big.Int)
		}
		usedIn[index].Add(usedIn[index], leg.AmountIn)
		usedOut[index].Add(usedOut[index], leg.AmountOut)
		// Direct swaps cap output and may absorb surplus exact input. Discounts
		// spend exact input at a live rate, so their input bound remains mandatory.
		if (leg.DiscountID != nil && usedIn[index].Cmp(candidate.MaxAmountIn) > 0) ||
			usedOut[index].Cmp(candidate.MaxAmountOut) > 0 {
			return nil, errors.New("fill: plan exceeds unreserved route capacity")
		}
		capacityID := liquidlane.RouteCapacityID(candidate.Route)
		reserved.Add(capacityID, leg.AmountOut)
		if leg.DiscountID != nil {
			reserved.Add(exclusiveReservationID(capacityID), big.NewInt(1))
			reserved.Add(discountReservationID(*leg.DiscountID), big.NewInt(1))
		}
	}
	return reserved, nil
}

func matchingCandidate(leg fillLeg, candidates []liquidlane.QuoteCandidate) int {
	for index, candidate := range candidates {
		if candidate.Route.Adapter != leg.Adapter {
			continue
		}
		if (candidate.DiscountID == nil && leg.DiscountID == nil) ||
			(candidate.DiscountID != nil && leg.DiscountID != nil && *candidate.DiscountID == *leg.DiscountID) {
			return index
		}
	}
	return -1
}
