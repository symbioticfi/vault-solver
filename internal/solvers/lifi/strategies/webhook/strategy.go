package webhookstrategy

import (
	"context"
	"math/big"
	"net/http"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	Name              = "webhook"
	decideQuotesRoute = "/decide-quotes"
	decideFillRoute   = "/decide-fill"
)

type Strategy struct {
	client *webhook.Client
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node, _ strategies.Deps) (types.Strategy, error) {
	cfg, err := webhook.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	client, err := webhook.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return New(client), nil
}

func New(client *webhook.Client) *Strategy {
	return &Strategy{client: client}
}

func (s *Strategy) DecideQuotes(ctx context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	var out types.QuoteOutput
	if err := s.client.DoJSON(ctx, http.MethodPost, decideQuotesRoute, input, &out); err != nil {
		return types.QuoteOutput{}, err
	}
	if err := validateQuotes(input, &out); err != nil {
		return types.QuoteOutput{}, err
	}
	return out, nil
}

func (s *Strategy) DecideFill(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	var out *types.FillPlan
	if err := s.client.DoJSON(ctx, http.MethodPost, decideFillRoute, input, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	if err := validateFill(input, out); err != nil {
		return nil, err
	}
	return out, nil
}

type quotePair struct {
	from, to       common.Address
	fromDec, toDec int
}

func validateQuotes(input types.QuoteInput, out *types.QuoteOutput) error {
	pairs := make(map[quotePair]bool)
	seen := make(map[quotePair]bool)
	for _, candidate := range input.Inventory {
		pairs[quotePair{
			from: candidate.TokenIn, to: candidate.TokenOut,
			fromDec: candidate.TokenInDecimals, toDec: candidate.TokenOutDecimals,
		}] = true
	}
	for i := range out.Quotes {
		quote := &out.Quotes[i]
		pair := quotePair{quote.FromAsset, quote.ToAsset, quote.FromDecimals, quote.ToDecimals}
		if !pairs[pair] {
			return errors.Errorf("webhook quote %d uses unknown token pair", i)
		}
		if seen[pair] {
			return errors.Errorf("webhook quote %d repeats token pair", i)
		}
		seen[pair] = true
		if quote.Expiry <= input.ServerTime.Unix() || quote.Expiry > input.QuoteExpiresAt.Unix() {
			return errors.Errorf("webhook quote %d expiry is outside the solver window", i)
		}
		if len(quote.Ranges) == 0 || len(quote.Ranges) > types.MaxQuoteRanges {
			return errors.Errorf("webhook quote %d has %d ranges, allowed [1,%d]", i, len(quote.Ranges), types.MaxQuoteRanges)
		}
		for j, priceRange := range quote.Ranges {
			rate, rateOK := new(big.Rat).SetString(priceRange.Quote)
			if priceRange.MinAmount == nil || priceRange.MaxAmount == nil || priceRange.MinAmount.Sign() <= 0 ||
				priceRange.MinAmount.Cmp(priceRange.MaxAmount) > 0 || priceRange.Quote == "" {
				return errors.Errorf("webhook quote %d range %d is invalid", i, j)
			}
			if !rateOK || rate.Sign() <= 0 {
				return errors.Errorf("webhook quote %d range %d rate is invalid", i, j)
			}
		}
		sort.Slice(quote.Ranges, func(i, j int) bool { return quote.Ranges[i].MinAmount.Cmp(quote.Ranges[j].MinAmount) < 0 })
		for j, priceRange := range quote.Ranges {
			if j > 0 && quote.Ranges[j-1].MaxAmount.Cmp(priceRange.MinAmount) >= 0 {
				return errors.Errorf("webhook quote %d ranges overlap", i)
			}
		}
		quote.ExclusiveFor = input.Solver
	}
	return nil
}

func validateFill(input types.FillInput, plan *types.FillPlan) error {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return errors.New("webhook fill input amount is invalid")
	}
	if len(plan.Routes) == 0 || len(plan.Routes) > types.MaxRoutes {
		return errors.Errorf("webhook fill has %d routes, allowed [1,%d]", len(plan.Routes), types.MaxRoutes)
	}
	candidates := make(map[liquidlane.CandidateID]liquidlane.FillQuote, len(input.Quotes))
	for _, candidate := range input.Quotes {
		candidates[liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID)] = candidate
	}
	usedRoutes := make(map[liquidlane.RouteID]bool, len(plan.Routes))
	capacityLimits := make(map[liquidlane.CapacityID]*big.Int, len(plan.Routes))
	capacityUsed := make(map[liquidlane.CapacityID]*big.Int, len(plan.Routes))
	totalInput := new(big.Int)
	totalMinimumOutput := new(big.Int)
	gasLegs := make([]strategies.GasLeg, 0, len(plan.Routes))
	for i := range plan.Routes {
		route := &plan.Routes[i]
		id := liquidlane.NewCandidateID(liquidlane.Route{ID: route.RouteID}, route.DiscountID)
		candidate, ok := candidates[id]
		if !ok {
			return errors.Errorf("webhook fill route %d uses unknown candidate %s", i, id)
		}
		if usedRoutes[candidate.ID] {
			return errors.Errorf("webhook fill repeats physical route %s", candidate.ID)
		}
		usedRoutes[candidate.ID] = true
		if route.AmountIn == nil || route.AmountIn.Sign() <= 0 || route.ExpectedAmountOut == nil ||
			route.ExpectedAmountOut.Sign() <= 0 || route.MinAmountOut == nil || route.MinAmountOut.Sign() <= 0 ||
			route.MinAmountOut.Cmp(route.ExpectedAmountOut) > 0 || route.ReservedAmountOut == nil ||
			route.ReservedAmountOut.Sign() <= 0 {
			return errors.Errorf("webhook fill route %d has invalid amounts", i)
		}
		if candidate.MaxAssets == nil || candidate.MaxAssets.Sign() <= 0 {
			return errors.Errorf("webhook fill route %d candidate capacity is invalid", i)
		}
		available := scaledOutput(candidate, route.AmountIn)
		if route.ExpectedAmountOut.Cmp(available) > 0 ||
			route.ReservedAmountOut.Cmp(route.ExpectedAmountOut) < 0 ||
			route.ReservedAmountOut.Cmp(candidate.MaxAssets) > 0 {
			return errors.Errorf("webhook fill route %d exceeds current candidate output or capacity", i)
		}
		route.RouteID = candidate.ID
		route.CapacityID = liquidlane.RouteCapacityID(candidate.Route)
		route.Adapter = candidate.Adapter
		route.DiscountID = liquidlane.CloneHash(candidate.DiscountID)
		totalInput.Add(totalInput, route.AmountIn)
		totalMinimumOutput.Add(totalMinimumOutput, route.MinAmountOut)
		if limit := capacityLimits[route.CapacityID]; limit == nil || candidate.MaxAssets.Cmp(limit) > 0 {
			capacityLimits[route.CapacityID] = liquidlane.CloneBig(candidate.MaxAssets)
		}
		if capacityUsed[route.CapacityID] == nil {
			capacityUsed[route.CapacityID] = new(big.Int)
		}
		capacityUsed[route.CapacityID].Add(capacityUsed[route.CapacityID], route.ReservedAmountOut)
		gasLegs = append(gasLegs, strategies.GasLeg{
			Route: candidate.Route, AmountOut: available, Private: candidate.DiscountID != nil,
		})
	}
	if totalInput.Cmp(input.AmountIn) != 0 {
		return errors.Errorf("webhook fill input sum %s does not match order %s", totalInput, input.AmountIn)
	}
	if input.RequireSingleRoute && len(plan.Routes) != 1 {
		return errors.New("webhook fill aggregates a permissioned token")
	}
	if input.OutputAmount == nil || input.OutputAmount.Sign() <= 0 {
		return errors.New("webhook fill output amount is invalid")
	}
	gasCost, err := strategies.FillGasCost(
		input.MaxFeePerGas, input.TokenOut, input.GasPrices, input.GasSnapshot, gasLegs,
	)
	if err != nil {
		return errors.Errorf("webhook fill gas cost: %w", err)
	}
	requiredOutput := new(big.Int).Add(input.OutputAmount, gasCost)
	if totalMinimumOutput.Cmp(requiredOutput) < 0 {
		return errors.New("webhook fill minimum output does not cover the order")
	}
	for capacityID, used := range capacityUsed {
		if reserved := input.Reservations[capacityID]; reserved != nil && reserved.Sign() > 0 {
			used.Add(used, reserved)
		}
		if used.Cmp(capacityLimits[capacityID]) > 0 {
			return errors.Errorf("webhook fill exceeds shared capacity %s", capacityID)
		}
	}
	return nil
}

func scaledOutput(candidate liquidlane.FillQuote, amountIn *big.Int) *big.Int {
	if candidate.AmountIn == nil || candidate.AmountIn.Sign() <= 0 || candidate.MaxAmountOut == nil {
		return new(big.Int)
	}
	return new(big.Int).Div(new(big.Int).Mul(candidate.MaxAmountOut, amountIn), candidate.AmountIn)
}

var _ types.Strategy = (*Strategy)(nil)
