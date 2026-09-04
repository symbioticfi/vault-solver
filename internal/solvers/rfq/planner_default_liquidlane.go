package rfq

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
)

func (s *defaultPlanner) DecideQuote(_ context.Context, input QuoteInput) (QuoteOutput, error) {
	if len(input.Candidates) == 0 {
		return decline(), nil
	}
	out, err := solveQuote(input, input.Candidates)
	if err != nil {
		return QuoteOutput{}, err
	}
	if out == nil {
		return decline(), nil
	}
	return *out, nil
}

func (s *defaultPlanner) BuildFillPlan(_ context.Context, input FillInput) (*liquidlane.Plan, error) {
	if len(input.Candidates) == 0 {
		return nil, nil
	}
	task, err := rfqFillTask(input, input.Candidates)
	if err != nil {
		return nil, err
	}
	solution, err := liquidplanning.SolveFill(task)
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
	return &liquidlane.Plan{Routes: routes}, nil
}

func decline() QuoteOutput {
	return QuoteOutput{Decision: DecisionDecline, Reason: "no viable strategy"}
}

func solveQuote(
	input QuoteInput,
	candidates []liquidlane.QuoteCandidate,
) (*QuoteOutput, error) {
	maxRoutes := input.RouteLimit()
	inputPolicy := liquidplanning.AbsorbUncoveredInput
	solution, err := liquidplanning.SolveQuote(liquidplanning.QuoteTask{
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
	legs := make([]QuoteLeg, 0, len(solution.Allocations))
	for _, leg := range solution.Allocations {
		legs = append(legs, QuoteLeg{
			CandidateID: string(leg.Candidate.ID),
			AmountIn:    liquidlane.CloneBig(leg.AmountIn), AmountOut: liquidlane.CloneBig(leg.AmountOut),
		})
	}
	return &QuoteOutput{
		Decision: DecisionQuote, QuotedAmountOut: liquidlane.CloneBig(solution.AmountOut), Legs: legs,
	}, nil
}

func rfqFillTask(
	input FillInput,
	candidates []liquidlane.QuoteCandidate,
) (liquidplanning.FillTask, error) {
	quotes := make([]liquidlane.FillQuote, 0, len(candidates))
	for _, candidate := range candidates {
		route := candidate.Route
		candidateID := liquidlane.NewCandidateID(route, candidate.DiscountID)
		if candidate.ID != candidateID {
			return liquidplanning.FillTask{}, errors.Errorf("candidate %q has invalid identity", candidate.ID)
		}
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
	maxRoutes := input.RouteLimit()
	inputPolicy := liquidplanning.AbsorbUncoveredInput
	return liquidplanning.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: liquidlane.CloneBig(input.AmountIn),
		Quotes: quotes, ValidAfter: input.Now, MaxRoutes: maxRoutes, InputPolicy: inputPolicy,
	}, nil
}
