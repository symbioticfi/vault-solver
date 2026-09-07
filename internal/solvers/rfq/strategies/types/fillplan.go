package types

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// FillPlanFromQuote commits a decision only after every selected candidate is
// bound to the current snapshot and both amount totals have been reconciled.
func FillPlanFromQuote(input QuoteInput, output QuoteOutput) (*FillPlan, error) {
	switch {
	case output.Decision != DecisionQuote:
		return nil, errors.Errorf("invalid fill-plan decision %q", output.Decision)
	case output.QuotedAmountOut == nil || output.QuotedAmountOut.Sign() <= 0:
		return nil, errors.New("quote output has invalid quotedAmountOut")
	case len(output.Legs) == 0:
		return nil, errors.New("quote output has no legs")
	case input.RequireSingleRoute && len(output.Legs) != 1:
		return nil, errors.New("single-route input requires exactly one leg")
	}
	candidates := make(map[string]liquidlane.QuoteCandidate, len(input.Candidates))
	for _, candidate := range input.Candidates {
		key := string(candidate.ID)
		if key == "" {
			return nil, errors.New("candidate id is empty")
		}
		if _, exists := candidates[key]; exists {
			return nil, errors.Errorf("duplicate candidate id %q", key)
		}
		if candidate.Route.ID == "" || candidate.ID != liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID) {
			return nil, errors.Errorf("candidate %q has invalid route", key)
		}
		candidates[key] = candidate
	}
	plan := &FillPlan{QuoteID: input.QuoteID, RequestID: input.RequestID, TokenIn: input.TokenIn, TokenOut: input.TokenOut,
		AmountIn: new(big.Int), QuotedAmountOut: new(big.Int), Legs: make([]FillLeg, len(output.Legs))}
	selected := make(map[liquidlane.RouteID]string, len(output.Legs))
	for index, leg := range output.Legs {
		candidate, exists := candidates[leg.CandidateID]
		if !exists {
			return nil, errors.Errorf("unknown candidate %q", leg.CandidateID)
		}
		if previous, used := selected[candidate.Route.ID]; used {
			if previous == leg.CandidateID {
				return nil, errors.Errorf("duplicate candidate %q", previous)
			}
			return nil, errors.Errorf("fill repeats physical route %q", candidate.Route.ID)
		}
		if candidate.Route.TokenOut != input.TokenOut {
			return nil, errors.Errorf("candidate %q asset does not match tokenOut", candidate.ID)
		}
		checked, err := checkedFillLeg(candidate, leg)
		if err != nil {
			return nil, errors.Errorf("leg %d: %w", index, err)
		}
		selected[candidate.Route.ID] = leg.CandidateID
		plan.Legs[index] = checked
		plan.AmountIn.Add(plan.AmountIn, checked.AmountIn)
		plan.QuotedAmountOut.Add(plan.QuotedAmountOut, checked.AmountOut)
	}
	if input.AmountIn == nil || plan.AmountIn.Cmp(input.AmountIn) != 0 {
		return nil, errors.Errorf("strategy amountIn sum %s does not match request %v", plan.AmountIn, input.AmountIn)
	}
	if plan.QuotedAmountOut.Cmp(output.QuotedAmountOut) != 0 {
		return nil, errors.Errorf("strategy amountOut sum %s does not match quotedAmountOut %s", plan.QuotedAmountOut, output.QuotedAmountOut)
	}
	if input.RequiredAmountOut != nil && plan.QuotedAmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return nil, errors.New("strategy output is below required amount out")
	}
	return plan, nil
}

func checkedFillLeg(candidate liquidlane.QuoteCandidate, leg QuoteLeg) (FillLeg, error) {
	for _, amount := range []struct {
		name  string
		value *big.Int
	}{
		{"amountIn", leg.AmountIn}, {"amountOut", leg.AmountOut}, {"rate", candidate.Rate},
	} {
		if amount.value == nil || amount.value.Sign() <= 0 {
			return FillLeg{}, errors.Errorf("invalid %s", amount.name)
		}
	}
	if candidate.MaxAmountOut == nil || candidate.MaxAmountOut.Sign() <= 0 || leg.AmountOut.Cmp(candidate.MaxAmountOut) > 0 {
		return FillLeg{}, errors.New("exceeds candidate maxAmountOut")
	}
	maximum := liquidlane.AmountOutForRate(leg.AmountIn, candidate.Rate, candidate.Route.TokenInDecimals, candidate.Route.TokenOutDecimals)
	if leg.AmountOut.Cmp(maximum) > 0 {
		return FillLeg{}, errors.New("exceeds output achievable at candidate rate")
	}
	return FillLeg{Adapter: candidate.Route.Adapter, AmountIn: bigmath.Clone(leg.AmountIn), AmountOut: bigmath.Clone(leg.AmountOut),
		MaxRate: bigmath.Clone(candidate.Rate), DiscountID: liquidlane.CloneHash(candidate.DiscountID)}, nil
}
