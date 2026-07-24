package defaultstrategy

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func (s *Strategy) decideQuote(_ context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	if len(input.Candidates) == 0 {
		return decline(), nil
	}
	out, err := solveQuote(input, input.Candidates)
	if err != nil {
		return types.QuoteOutput{}, err
	}
	if out == nil {
		return decline(), nil
	}
	return *out, nil
}

func (s *Strategy) buildFillPlan(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if len(input.Candidates) == 0 {
		return nil, nil
	}
	task, sources, err := rfqFillTask(input, input.Candidates)
	if err != nil {
		return nil, err
	}
	solution, err := liquidgreedy.SolveFill(task)
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
			MaxRate: liquidlane.CloneBig(source.Rate), DiscountID: liquidlane.CloneHash(source.DiscountID),
		})
	}
	return &types.FillPlan{
		QuoteID: input.QuoteID, RequestID: input.RequestID,
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: liquidlane.CloneBig(input.AmountIn),
		QuotedAmountOut: quotedAmountOut, Legs: legs,
	}, nil
}

func decline() types.QuoteOutput {
	return types.QuoteOutput{Decision: types.DecisionDecline, Reason: "no viable strategy"}
}

func solveQuote(
	input types.QuoteInput,
	candidates []liquidlane.QuoteCandidate,
) (*types.QuoteOutput, error) {
	maxRoutes := len(candidates)
	inputPolicy := liquidgreedy.AbsorbUncoveredInput
	if input.RequireSingleRoute {
		maxRoutes = 1
		inputPolicy = liquidgreedy.RejectUncoveredInput
	}
	solution, err := liquidgreedy.SolveQuote(liquidgreedy.QuoteTask{
		ExactInput: input.AmountIn, Candidates: candidates, MaxRoutes: maxRoutes,
		InputPolicy: inputPolicy,
	})
	if err != nil || solution == nil {
		return nil, err
	}
	if input.RequireSingleRoute && input.RequiredAmountOut != nil &&
		solution.AmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return nil, nil
	}
	legs := make([]types.QuoteLeg, 0, len(solution.Allocations))
	for _, leg := range solution.Allocations {
		legs = append(legs, types.QuoteLeg{
			CandidateID: string(leg.Candidate.ID),
			AmountIn:    liquidlane.CloneBig(leg.AmountIn), AmountOut: liquidlane.CloneBig(leg.AmountOut),
		})
	}
	return &types.QuoteOutput{
		Decision: types.DecisionQuote, QuotedAmountOut: liquidlane.CloneBig(solution.AmountOut), Legs: legs,
	}, nil
}

func rfqFillTask(
	input types.FillInput,
	candidates []liquidlane.QuoteCandidate,
) (liquidgreedy.FillTask, map[liquidlane.CandidateID]liquidlane.QuoteCandidate, error) {
	quotes := make([]liquidlane.FillQuote, 0, len(candidates))
	sources := make(map[liquidlane.CandidateID]liquidlane.QuoteCandidate, len(candidates))
	for _, candidate := range candidates {
		route := candidate.Route
		candidateID := liquidlane.NewCandidateID(route, candidate.DiscountID)
		if candidate.ID != candidateID {
			return liquidgreedy.FillTask{}, nil, errors.Errorf("candidate %q has invalid identity", candidate.ID)
		}
		sources[candidateID] = candidate
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{
				Route: route, MaxAssets: liquidlane.CloneBig(candidate.MaxAmountOut),
				MaxRate: liquidlane.CloneBig(candidate.Rate), DiscountID: liquidlane.CloneHash(candidate.DiscountID),
				ValidUntil: candidate.ValidUntil,
			},
			AmountIn: liquidlane.CloneBig(input.AmountIn),
			MaxAmountOut: liquidlane.AmountOutForRate(
				input.AmountIn, candidate.Rate, route.TokenInDecimals, route.TokenOutDecimals,
			),
		})
	}
	maxRoutes := len(quotes)
	inputPolicy := liquidgreedy.AbsorbUncoveredInput
	if input.RequireSingleRoute {
		maxRoutes = 1
		inputPolicy = liquidgreedy.RejectUncoveredInput
	}
	return liquidgreedy.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: liquidlane.CloneBig(input.AmountIn),
		Quotes: quotes, ValidAfter: input.Now, MaxRoutes: maxRoutes, InputPolicy: inputPolicy,
	}, sources, nil
}
