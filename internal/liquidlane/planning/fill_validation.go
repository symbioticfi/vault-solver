package planning

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

type FillValidation struct {
	TokenIn, TokenOut  common.Address
	AmountIn           *big.Int
	RequiredAmountOut  *big.Int
	RequireSingleRoute bool
	MaxRoutes          int
	Quotes             []liquidlane.FillQuote
	Reservations       liquidlane.CapacityReservations
	GasSnapshot        *liquidlanegas.Snapshot
	GasPrices          *liquidlanegas.PriceSnapshot
	MaxFeePerGas       *big.Int
	GasEnvelope        GasEnvelope
}

type fillValidator struct {
	input      FillValidation
	candidates map[liquidlane.CandidateID]liquidlane.FillQuote
	used       map[liquidlane.RouteID]struct{}
	limits     liquidlane.CapacityReservations
	reserved   liquidlane.CapacityReservations
	totalIn    *big.Int
	totalOut   *big.Int
	gasLegs    []GasLeg
}

func ValidateFillRoutes(input FillValidation, routes []liquidlane.PlanLeg) ([]liquidlane.PlanLeg, error) {
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

	validator := newFillValidator(input)
	normalized := make([]liquidlane.PlanLeg, len(routes))
	for index, route := range routes {
		canonical, err := validator.validateRoute(index, route)
		if err != nil {
			return nil, err
		}
		normalized[index] = canonical
	}
	if err := validator.validateTotals(); err != nil {
		return nil, err
	}
	return normalized, nil
}

func newFillValidator(input FillValidation) *fillValidator {
	candidates := make(map[liquidlane.CandidateID]liquidlane.FillQuote, len(input.Quotes))
	for _, candidate := range input.Quotes {
		candidates[liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID)] = candidate
	}
	return &fillValidator{
		input: input, candidates: candidates, used: make(map[liquidlane.RouteID]struct{}),
		limits: make(liquidlane.CapacityReservations), reserved: make(liquidlane.CapacityReservations),
		totalIn: new(big.Int), totalOut: new(big.Int), gasLegs: make([]GasLeg, 0),
	}
}

func (validator *fillValidator) validateRoute(index int, route liquidlane.PlanLeg) (liquidlane.PlanLeg, error) {
	candidateID := liquidlane.NewCandidateID(liquidlane.Route{ID: route.RouteID}, route.DiscountID)
	candidate, exists := validator.candidates[candidateID]
	if !exists {
		return liquidlane.PlanLeg{}, errors.Errorf("fill route %d uses unknown candidate %s", index, candidateID)
	}
	if candidate.TokenIn != validator.input.TokenIn || candidate.TokenOut != validator.input.TokenOut {
		return liquidlane.PlanLeg{}, errors.Errorf("fill route %d uses a candidate from another token pair", index)
	}
	if _, used := validator.used[candidate.ID]; used {
		return liquidlane.PlanLeg{}, errors.Errorf("fill repeats physical route %s", candidate.ID)
	}
	validator.used[candidate.ID] = struct{}{}
	if !validFillRouteAmounts(route) {
		return liquidlane.PlanLeg{}, errors.Errorf("fill route %d has invalid amounts", index)
	}
	if candidate.MaxAssets == nil || candidate.MaxAssets.Sign() <= 0 {
		return liquidlane.PlanLeg{}, errors.Errorf("fill route %d candidate capacity is invalid", index)
	}
	available := scaledOutput(candidate, route.AmountIn)
	if route.ExpectedAmountOut.Cmp(available) > 0 || route.ReservedAmountOut.Cmp(route.ExpectedAmountOut) < 0 ||
		route.ReservedAmountOut.Cmp(candidate.MaxAssets) > 0 {
		return liquidlane.PlanLeg{}, errors.Errorf("fill route %d exceeds current candidate output or capacity", index)
	}

	capacityID := liquidlane.RouteCapacityID(candidate.Route)
	if limit := validator.limits[capacityID]; limit == nil || candidate.MaxAssets.Cmp(limit) > 0 {
		validator.limits[capacityID] = liquidlane.CloneBig(candidate.MaxAssets)
	}
	validator.reserved.Add(capacityID, route.ReservedAmountOut)
	validator.totalIn.Add(validator.totalIn, route.AmountIn)
	validator.totalOut.Add(validator.totalOut, route.MinAmountOut)
	validator.gasLegs = append(validator.gasLegs, GasLeg{
		Route: candidate.Route, AmountOut: available, Private: candidate.DiscountID != nil,
	})

	canonical := cloneFillRoute(route)
	canonical.CandidateID = candidateID
	canonical.RouteID = candidate.ID
	canonical.CapacityID = capacityID
	canonical.Adapter = candidate.Adapter
	canonical.DiscountID = liquidlane.CloneHash(candidate.DiscountID)
	return canonical, nil
}

func (validator *fillValidator) validateTotals() error {
	if validator.totalIn.Cmp(validator.input.AmountIn) != 0 {
		return errors.Errorf("fill input sum %s does not match order %s", validator.totalIn, validator.input.AmountIn)
	}
	gasCost, err := FillGasCost(
		validator.input.MaxFeePerGas, validator.input.TokenOut, validator.input.GasPrices,
		validator.input.GasSnapshot, validator.input.GasEnvelope, validator.gasLegs,
	)
	if err != nil {
		return errors.Errorf("fill gas cost: %w", err)
	}
	required := new(big.Int).Add(validator.input.RequiredAmountOut, gasCost)
	if validator.totalOut.Cmp(required) < 0 {
		return errors.New("fill minimum output does not cover the order")
	}
	for capacityID, used := range validator.reserved {
		used = liquidlane.CloneBig(used)
		if pending := validator.input.Reservations[capacityID]; pending != nil && pending.Sign() > 0 {
			used.Add(used, pending)
		}
		if used.Cmp(validator.limits[capacityID]) > 0 {
			return errors.Errorf("fill exceeds shared capacity %s", capacityID)
		}
	}
	return nil
}

func FillRouteReservations(routes []liquidlane.PlanLeg) (liquidlane.CapacityReservations, bool) {
	reservations := make(liquidlane.CapacityReservations)
	for _, route := range routes {
		if route.CapacityID == "" || route.ReservedAmountOut == nil || route.ReservedAmountOut.Sign() <= 0 {
			return nil, false
		}
		reservations.Add(route.CapacityID, route.ReservedAmountOut)
	}
	return reservations, len(reservations) > 0
}

func validFillRouteAmounts(route liquidlane.PlanLeg) bool {
	return route.AmountIn != nil && route.AmountIn.Sign() > 0 && route.ExpectedAmountOut != nil &&
		route.ExpectedAmountOut.Sign() > 0 && route.MinAmountOut != nil && route.MinAmountOut.Sign() > 0 &&
		route.MinAmountOut.Cmp(route.ExpectedAmountOut) <= 0 && route.ReservedAmountOut != nil &&
		route.ReservedAmountOut.Sign() > 0
}

func scaledOutput(candidate liquidlane.FillQuote, amountIn *big.Int) *big.Int {
	if candidate.AmountIn == nil || candidate.AmountIn.Sign() <= 0 || candidate.MaxAmountOut == nil {
		return new(big.Int)
	}
	return new(big.Int).Quo(new(big.Int).Mul(candidate.MaxAmountOut, amountIn), candidate.AmountIn)
}

func cloneFillRoute(route liquidlane.PlanLeg) liquidlane.PlanLeg {
	route.AmountIn = liquidlane.CloneBig(route.AmountIn)
	route.ExpectedAmountOut = liquidlane.CloneBig(route.ExpectedAmountOut)
	route.MinAmountOut = liquidlane.CloneBig(route.MinAmountOut)
	route.ReservedAmountOut = liquidlane.CloneBig(route.ReservedAmountOut)
	route.DiscountID = liquidlane.CloneHash(route.DiscountID)
	return route
}
