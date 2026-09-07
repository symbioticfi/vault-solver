package planning

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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

// PlannedSurplus returns the positive expected output above the required output.
func PlannedSurplus(routes []FillRoute, requiredOutput *big.Int) *big.Int {
	expectedOutput := new(big.Int)
	for _, route := range routes {
		if route.ExpectedAmountOut != nil {
			expectedOutput.Add(expectedOutput, route.ExpectedAmountOut)
		}
	}
	return liquidlane.PlannedSurplus(expectedOutput, requiredOutput)
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
	if len(routes) < 1 || len(routes) > input.MaxRoutes {
		return nil, errors.Errorf("fill has %d routes, allowed [1,%d]", len(routes), input.MaxRoutes)
	}
	if input.RequireSingleRoute && len(routes) != 1 {
		return nil, errors.New("fill aggregates a permissioned token")
	}
	quotes := make(map[liquidlane.CandidateID]liquidlane.FillQuote, len(input.Quotes))
	for _, quote := range input.Quotes {
		quotes[liquidlane.NewCandidateID(quote.Route, quote.DiscountID)] = quote
	}
	type capacity struct{ used, limit big.Int }
	domains := make(map[liquidlane.CapacityID]*capacity)
	physical := make(map[liquidlane.RouteID]bool)
	normalized := make([]FillRoute, 0, len(routes))
	legs := make([]GasLeg, 0, len(routes))
	var consumed, minimum big.Int
	for index, proposed := range routes {
		id := liquidlane.NewCandidateID(liquidlane.Route{ID: proposed.RouteID}, proposed.DiscountID)
		quote, exists := quotes[id]
		if !exists {
			return nil, errors.Errorf("fill route %d uses unknown candidate %s", index, id)
		}
		if quote.TokenIn != input.TokenIn || quote.TokenOut != input.TokenOut {
			return nil, errors.Errorf("fill route %d uses a candidate from another token pair", index)
		}
		if physical[quote.ID] {
			return nil, errors.Errorf("fill repeats physical route %s", quote.ID)
		}
		leg, err := canonicalFillLeg(proposed, quote)
		if err != nil {
			return nil, errors.Errorf("fill route %d: %w", index, err)
		}
		physical[quote.ID] = true
		consumed.Add(&consumed, leg.AmountIn)
		minimum.Add(&minimum, leg.MinAmountOut)
		domain := domains[leg.CapacityID]
		if domain == nil {
			domain = &capacity{}
			domains[leg.CapacityID] = domain
		}
		domain.used.Add(&domain.used, leg.ReservedAmountOut)
		if domain.limit.Cmp(quote.MaxAssets) < 0 {
			domain.limit.Set(quote.MaxAssets)
		}
		normalized = append(normalized, leg)
		legs = append(legs, GasLeg{Route: quote.Route, AmountOut: scaledFillOutput(quote, leg.AmountIn), Private: quote.DiscountID != nil})
	}
	if consumed.Cmp(input.AmountIn) != 0 {
		return nil, errors.Errorf("fill input sum %s does not match order %s", &consumed, input.AmountIn)
	}
	cost, err := FillGasCost(input.MaxFeePerGas, input.TokenOut, input.GasPrices, input.GasSnapshot, input.GasEnvelope, legs)
	if err != nil {
		return nil, errors.Errorf("fill gas cost: %w", err)
	}
	cost.Add(cost, input.RequiredAmountOut)
	if minimum.Cmp(cost) < 0 {
		return nil, errors.New("fill minimum output does not cover the order")
	}
	for id, domain := range domains {
		if held := input.Reservations[id]; held != nil && held.Sign() > 0 {
			domain.used.Add(&domain.used, held)
		}
		if domain.used.Cmp(&domain.limit) > 0 {
			return nil, errors.Errorf("fill exceeds shared capacity %s", id)
		}
	}
	return normalized, nil
}

// Strategy identities and capacity domains are never trusted. Reconstruct the
// executable leg from the matched current quote and independently checked amounts.
func canonicalFillLeg(proposed FillRoute, quote liquidlane.FillQuote) (FillRoute, error) {
	for _, amount := range []*big.Int{proposed.AmountIn, proposed.ExpectedAmountOut, proposed.MinAmountOut, proposed.ReservedAmountOut} {
		if amount == nil || amount.Sign() <= 0 {
			return FillRoute{}, errors.New("invalid amounts")
		}
	}
	if proposed.MinAmountOut.Cmp(proposed.ExpectedAmountOut) > 0 {
		return FillRoute{}, errors.New("minimum exceeds expected output")
	}
	if quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0 {
		return FillRoute{}, errors.New("candidate capacity is invalid")
	}
	if proposed.ExpectedAmountOut.Cmp(scaledFillOutput(quote, proposed.AmountIn)) > 0 ||
		proposed.ReservedAmountOut.Cmp(proposed.ExpectedAmountOut) < 0 || proposed.ReservedAmountOut.Cmp(quote.MaxAssets) > 0 {
		return FillRoute{}, errors.New("exceeds current candidate output or capacity")
	}
	return FillRoute{
		CandidateID: liquidlane.NewCandidateID(quote.Route, quote.DiscountID), RouteID: quote.ID,
		CapacityID: liquidlane.RouteCapacityID(quote.Route), Adapter: quote.Adapter,
		DiscountID: liquidlane.CloneHash(quote.DiscountID), AmountIn: bigmath.Clone(proposed.AmountIn),
		ExpectedAmountOut: bigmath.Clone(proposed.ExpectedAmountOut), MinAmountOut: bigmath.Clone(proposed.MinAmountOut),
		ReservedAmountOut: bigmath.Clone(proposed.ReservedAmountOut),
	}, nil
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
