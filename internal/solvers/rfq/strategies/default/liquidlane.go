package defaultstrategy

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func (s *Strategy) decideQuote(_ context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	if len(input.Candidates) == 0 {
		return decline("no quote candidates"), nil
	}
	out, reason, err := solveQuote(input, input.Candidates)
	if err != nil {
		return types.QuoteOutput{}, err
	}
	if out == nil {
		return decline(reason), nil
	}
	return *out, nil
}

func (s *Strategy) buildFillPlan(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if len(input.Candidates) == 0 {
		return nil, errors.New("no fill candidates")
	}
	task, sources, err := rfqFillTask(input, input.Candidates)
	if err != nil {
		return nil, err
	}
	var lastDecline func() string
	task.Trace, lastDecline = task.Trace.RecordDeclines()
	solution, err := liquidgreedy.SolveFill(task)
	if err != nil {
		return nil, err
	}
	if solution == nil {
		return nil, errors.Errorf("no fill solution for amountIn %s over %d candidates: %s",
			input.AmountIn, len(input.Candidates), lastDecline())
	}
	quotedAmountOut := solution.MaxAmountOut()
	if input.RequiredAmountOut != nil && quotedAmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return nil, errors.Errorf("fill output %s is below required amount out %s (singleRoute=%t)",
			quotedAmountOut, input.RequiredAmountOut, input.RequireSingleRoute)
	}
	routes := solution.Finalize(quotedAmountOut)
	if len(routes) == 0 {
		return nil, errors.Errorf("fill solution for amountOut %s finalized no routes", quotedAmountOut)
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

func decline(reason string) types.QuoteOutput {
	return types.QuoteOutput{Decision: types.DecisionDecline, Reason: reason}
}

// solveQuote returns the quote, or nil and the condition that declined it.
func solveQuote(
	input types.QuoteInput,
	candidates []liquidlane.QuoteCandidate,
) (*types.QuoteOutput, string, error) {
	maxRoutes := len(candidates)
	inputPolicy := strategies.AbsorbUncoveredInput
	if input.RequireFullCoverage {
		inputPolicy = strategies.RejectUncoveredInput
	}
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	trace, lastDecline := strategies.DecisionTrace(nil).RecordDeclines()
	solution, err := liquidgreedy.SolveQuote(strategies.QuoteTask{
		ExactInput: input.AmountIn, Candidates: candidates, MaxRoutes: maxRoutes,
		InputPolicy: inputPolicy, Trace: trace,
	})
	if err != nil {
		return nil, "", err
	}
	if solution == nil {
		return nil, "no quote solution: " + lastDecline(), nil
	}
	if input.RequireSingleRoute && input.RequiredAmountOut != nil &&
		solution.AmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return nil, "single-route output " + solution.AmountOut.String() +
			" is below required amount out " + input.RequiredAmountOut.String(), nil
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
	}, "", nil
}

func rfqFillTask(
	input types.FillInput,
	candidates []liquidlane.QuoteCandidate,
) (strategies.FillTask, map[liquidlane.CandidateID]liquidlane.QuoteCandidate, error) {
	quotes := make([]liquidlane.FillQuote, 0, len(candidates))
	sources := make(map[liquidlane.CandidateID]liquidlane.QuoteCandidate, len(candidates))
	for _, candidate := range candidates {
		route := candidate.Route
		candidateID := liquidlane.NewCandidateID(route, candidate.DiscountID)
		if candidate.ID != candidateID {
			return strategies.FillTask{}, nil, errors.Errorf("candidate %q has invalid identity", candidate.ID)
		}
		sources[candidateID] = candidate
		var pricing *liquidlane.QuoteCandidate
		if candidate.ExactPricing() {
			pricing = &candidate
		}
		quotes = append(quotes, liquidlane.FillQuote{
			Route: route, MaxAssets: liquidlane.CloneBig(candidate.MaxAmountOut),
			MaxRate: liquidlane.CloneBig(candidate.Rate), DiscountID: liquidlane.CloneHash(candidate.DiscountID),
			ValidUntil:   candidate.ValidUntil,
			Pricing:      pricing,
			AmountIn:     liquidlane.CloneBig(input.AmountIn),
			MaxAmountOut: candidate.AmountOutFor(input.AmountIn),
		})
	}
	maxRoutes := len(quotes)
	inputPolicy := strategies.AbsorbUncoveredInput
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	return strategies.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: liquidlane.CloneBig(input.AmountIn),
		Quotes: quotes, ValidAfter: input.Now, MaxRoutes: maxRoutes, InputPolicy: inputPolicy,
	}, sources, nil
}
