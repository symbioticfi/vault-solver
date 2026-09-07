package defaultstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func (s *Strategy) DecideQuote(_ context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	if len(input.Candidates) == 0 {
		return decline(), nil
	}
	maxRoutes := len(input.Candidates)
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	solution, err := planning.NewQuotePool(input.Candidates).Solve(planning.QuoteTask{
		ExactInput: input.AmountIn, MaxRoutes: maxRoutes, InputPolicy: planning.AbsorbUncoveredInput,
	})
	if err != nil {
		return types.QuoteOutput{}, err
	}
	if solution == nil || input.RequireSingleRoute && input.RequiredAmountOut != nil && solution.AmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return decline(), nil
	}
	legs := make([]types.QuoteLeg, len(solution.Allocations))
	for i, leg := range solution.Allocations {
		legs[i] = types.QuoteLeg{CandidateID: string(leg.Candidate.ID), AmountIn: leg.AmountIn, AmountOut: leg.AmountOut}
	}
	return types.QuoteOutput{Decision: types.DecisionQuote, QuotedAmountOut: solution.AmountOut, Legs: legs}, nil
}

func (s *Strategy) BuildFillPlan(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if len(input.Candidates) == 0 {
		return nil, nil
	}
	task, sources, err := rfqFillTask(input)
	if err != nil {
		return nil, err
	}
	solution, err := planning.SolveFill(task)
	if err != nil || solution == nil {
		return nil, err
	}
	quotedAmountOut := solution.MaxAmountOut()
	if input.RequiredAmountOut != nil && quotedAmountOut.Cmp(input.RequiredAmountOut) < 0 {
		if input.RequireSingleRoute {
			return nil, nil
		}
		return nil, errors.New("strategy output is below required amount out")
	}
	routes := solution.Finalize(quotedAmountOut)
	if len(routes) == 0 {
		return nil, nil
	}
	legs := make([]types.FillLeg, 0, len(routes))
	for _, route := range routes {
		source, ok := sources[route.CandidateID]
		if !ok {
			return nil, errors.Errorf("fill candidate %q lost its RFQ source", route.CandidateID)
		}
		legs = append(legs, types.FillLeg{
			Adapter: source.Route.Adapter, AmountIn: route.AmountIn, AmountOut: route.ExpectedAmountOut,
			MaxRate: bigmath.Clone(source.Rate), DiscountID: liquidlane.CloneHash(source.DiscountID),
		})
	}
	return &types.FillPlan{
		QuoteID: input.QuoteID, RequestID: input.RequestID,
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: bigmath.Clone(input.AmountIn),
		QuotedAmountOut: quotedAmountOut, Legs: legs,
	}, nil
}

func decline() types.QuoteOutput {
	return types.QuoteOutput{Decision: types.DecisionDecline, Reason: "no viable strategy"}
}

// The pure planner borrows candidate fields; the resulting fill plan owns its amounts.
func rfqFillTask(input types.FillInput) (planning.FillTask, map[liquidlane.CandidateID]liquidlane.QuoteCandidate, error) {
	quotes := make([]liquidlane.FillQuote, 0, len(input.Candidates))
	sources := make(map[liquidlane.CandidateID]liquidlane.QuoteCandidate, len(input.Candidates))
	for _, candidate := range input.Candidates {
		route := candidate.Route
		candidateID := liquidlane.NewCandidateID(route, candidate.DiscountID)
		if candidate.ID != candidateID {
			return planning.FillTask{}, nil, errors.Errorf("candidate %q has invalid identity", candidate.ID)
		}
		sources[candidateID] = candidate
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{
				Route: route, MaxAssets: candidate.MaxAmountOut,
				MaxRate: candidate.Rate, DiscountID: candidate.DiscountID,
				ValidUntil: candidate.ValidUntil,
			},
			AmountIn: input.AmountIn,
			MaxAmountOut: liquidlane.AmountOutForRate(
				input.AmountIn, candidate.Rate, route.TokenInDecimals, route.TokenOutDecimals,
			),
		})
	}
	maxRoutes := len(quotes)
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	return planning.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		Quotes: quotes, ValidAfter: input.Now, MaxRoutes: maxRoutes, InputPolicy: planning.AbsorbUncoveredInput,
	}, sources, nil
}
