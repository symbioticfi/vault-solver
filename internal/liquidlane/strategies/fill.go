package strategies

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

// FillRoute is one canonical LiquidLane execution leg selected by a strategy.
type FillRoute struct {
	CandidateID       liquidlane.CandidateID `json:"-"`
	RouteID           liquidlane.RouteID     `json:"routeId"`
	CapacityID        liquidlane.CapacityID  `json:"capacityId"`
	Adapter           common.Address         `json:"adapter"`
	AmountIn          *big.Int               `json:"amountIn"`
	ExpectedAmountOut *big.Int               `json:"expectedAmountOut"`
	MinAmountOut      *big.Int               `json:"minAmountOut"`
	ReservedAmountOut *big.Int               `json:"reservedAmountOut"`
	DiscountID        *common.Hash           `json:"discountId"`
}

// FillValidation contains the solver-owned facts used to validate an external fill decision.
type FillValidation struct {
	TokenIn            common.Address
	TokenOut           common.Address
	AmountIn           *big.Int
	RequiredAmountOut  *big.Int
	RequireSingleRoute bool
	MaxRoutes          int

	Quotes       []liquidlane.FillQuote
	Reservations liquidlane.CapacityReservations
	GasSnapshot  *liquidlanegas.Snapshot
	GasPrices    *liquidlanegas.PriceSnapshot
	MaxFeePerGas *big.Int
	GasEnvelope  GasEnvelope
}

// ValidateFillRoutes validates and canonicalizes untrusted strategy output.
func ValidateFillRoutes(input FillValidation, routes []FillRoute) ([]FillRoute, error) {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return nil, errors.New("fill input amount is invalid")
	}
	if input.RequiredAmountOut == nil || input.RequiredAmountOut.Sign() <= 0 {
		return nil, errors.New("fill output amount is invalid")
	}
	if len(routes) == 0 || len(routes) > input.MaxRoutes {
		return nil, errors.Errorf("fill has %d routes, allowed [1,%d]", len(routes), input.MaxRoutes)
	}
	if input.RequireSingleRoute && len(routes) != 1 {
		return nil, errors.New("fill aggregates a permissioned token")
	}

	candidates := make(map[liquidlane.CandidateID]liquidlane.FillQuote, len(input.Quotes))
	for _, candidate := range input.Quotes {
		candidates[liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID)] = candidate
	}

	normalized := make([]FillRoute, len(routes))
	usedRoutes := make(map[liquidlane.RouteID]bool, len(routes))
	capacityLimits := make(liquidlane.CapacityReservations, len(routes))
	capacityUsed := make(liquidlane.CapacityReservations, len(routes))
	totalInput := new(big.Int)
	totalMinimumOutput := new(big.Int)
	gasLegs := make([]GasLeg, 0, len(routes))
	for index, route := range routes {
		id := liquidlane.NewCandidateID(liquidlane.Route{ID: route.RouteID}, route.DiscountID)
		candidate, ok := candidates[id]
		if !ok {
			return nil, errors.Errorf("fill route %d uses unknown candidate %s", index, id)
		}
		if candidate.TokenIn != input.TokenIn || candidate.TokenOut != input.TokenOut {
			return nil, errors.Errorf("fill route %d uses a candidate from another token pair", index)
		}
		if usedRoutes[candidate.ID] {
			return nil, errors.Errorf("fill repeats physical route %s", candidate.ID)
		}
		usedRoutes[candidate.ID] = true
		if !validFillRouteAmounts(route) {
			return nil, errors.Errorf("fill route %d has invalid amounts", index)
		}
		if candidate.MaxAssets == nil || candidate.MaxAssets.Sign() <= 0 {
			return nil, errors.Errorf("fill route %d candidate capacity is invalid", index)
		}
		available := scaledOutput(candidate, route.AmountIn)
		if route.ExpectedAmountOut.Cmp(available) > 0 ||
			route.ReservedAmountOut.Cmp(route.ExpectedAmountOut) < 0 ||
			route.ReservedAmountOut.Cmp(candidate.MaxAssets) > 0 {
			return nil, errors.Errorf("fill route %d exceeds current candidate output or capacity", index)
		}

		capacityID := liquidlane.RouteCapacityID(candidate.Route)
		normalized[index] = cloneFillRoute(route)
		normalized[index].CandidateID = id
		normalized[index].RouteID = candidate.ID
		normalized[index].CapacityID = capacityID
		normalized[index].Adapter = candidate.Adapter
		normalized[index].DiscountID = liquidlane.CloneHash(candidate.DiscountID)
		totalInput.Add(totalInput, route.AmountIn)
		totalMinimumOutput.Add(totalMinimumOutput, route.MinAmountOut)
		if limit := capacityLimits[capacityID]; limit == nil || candidate.MaxAssets.Cmp(limit) > 0 {
			capacityLimits[capacityID] = liquidlane.CloneBig(candidate.MaxAssets)
		}
		capacityUsed.Add(capacityID, route.ReservedAmountOut)
		gasLegs = append(gasLegs, GasLeg{
			Route: candidate.Route, AmountOut: available, Private: candidate.DiscountID != nil,
		})
	}

	if totalInput.Cmp(input.AmountIn) != 0 {
		return nil, errors.Errorf("fill input sum %s does not match order %s", totalInput, input.AmountIn)
	}
	gasCost, err := FillGasCost(
		input.MaxFeePerGas, input.TokenOut, input.GasPrices, input.GasSnapshot, input.GasEnvelope, gasLegs,
	)
	if err != nil {
		return nil, errors.Errorf("fill gas cost: %w", err)
	}
	requiredOutput := new(big.Int).Add(input.RequiredAmountOut, gasCost)
	if totalMinimumOutput.Cmp(requiredOutput) < 0 {
		return nil, errors.New("fill minimum output does not cover the order")
	}
	for capacityID, used := range capacityUsed {
		if reserved := input.Reservations[capacityID]; reserved != nil && reserved.Sign() > 0 {
			used.Add(used, reserved)
		}
		if used.Cmp(capacityLimits[capacityID]) > 0 {
			return nil, errors.Errorf("fill exceeds shared capacity %s", capacityID)
		}
	}
	return normalized, nil
}

// FillRouteReservations validates and aggregates the capacity reserved by a fill plan.
func FillRouteReservations(routes []FillRoute) (liquidlane.CapacityReservations, bool) {
	reservations := make(liquidlane.CapacityReservations)
	for _, route := range routes {
		if route.CapacityID == "" || route.ReservedAmountOut == nil || route.ReservedAmountOut.Sign() <= 0 {
			return nil, false
		}
		reservations.Add(route.CapacityID, route.ReservedAmountOut)
	}
	return reservations, len(reservations) > 0
}

func validFillRouteAmounts(route FillRoute) bool {
	return route.AmountIn != nil && route.AmountIn.Sign() > 0 &&
		route.ExpectedAmountOut != nil && route.ExpectedAmountOut.Sign() > 0 &&
		route.MinAmountOut != nil && route.MinAmountOut.Sign() > 0 &&
		route.MinAmountOut.Cmp(route.ExpectedAmountOut) <= 0 &&
		route.ReservedAmountOut != nil && route.ReservedAmountOut.Sign() > 0
}

func scaledOutput(candidate liquidlane.FillQuote, amountIn *big.Int) *big.Int {
	if candidate.AmountIn == nil || candidate.AmountIn.Sign() <= 0 || candidate.MaxAmountOut == nil {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(candidate.MaxAmountOut, amountIn), candidate.AmountIn)
}

func cloneFillRoute(route FillRoute) FillRoute {
	route.AmountIn = liquidlane.CloneBig(route.AmountIn)
	route.ExpectedAmountOut = liquidlane.CloneBig(route.ExpectedAmountOut)
	route.MinAmountOut = liquidlane.CloneBig(route.MinAmountOut)
	route.ReservedAmountOut = liquidlane.CloneBig(route.ReservedAmountOut)
	route.DiscountID = liquidlane.CloneHash(route.DiscountID)
	return route
}
