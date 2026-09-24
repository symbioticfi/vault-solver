// Package single evaluates complete, single-source LiquidLane alternatives.
// Candidate discovery and protocol lifecycle remain owned by the calling solver.
package single

import (
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

// SolveQuote compares executable quotes, including the caller's buffer and gas.
// Capacities must already reflect each source's available shared budget and own limit.
func SolveQuote(task strategies.QuoteTask) (*strategies.QuoteSolution, error) {
	var best *strategies.QuoteSolution
	task.MaxRoutes = 1
	task.InputPolicy = strategies.RejectUncoveredInput
	candidates := task.Candidates
	for _, candidate := range candidates {
		task.Candidates = []liquidlane.QuoteCandidate{candidate}
		quote, err := greedy.SolveQuote(task)
		if err != nil {
			return nil, err
		}
		if quote != nil && betterQuote(quote, best, task.ExactInput != nil) {
			best = quote
		}
	}
	return best, nil
}

func betterQuote(left, right *strategies.QuoteSolution, exactInput bool) bool {
	if right == nil {
		return true
	}
	comparison := left.AmountIn.Cmp(right.AmountIn)
	if exactInput {
		comparison = right.AmountOut.Cmp(left.AmountOut)
	}
	if comparison != 0 {
		return comparison < 0
	}
	return left.Allocations[0].Candidate.ID < right.Allocations[0].Candidate.ID
}

// SolveFill evaluates each full-amount source independently. Callers validate the
// required output and perform fresh signature resolution and preflight afterwards.
func SolveFill(task strategies.FillTask) (*strategies.FillSolution, error) {
	var best *strategies.FillSolution
	var bestID liquidlane.CandidateID
	task.MaxRoutes = 1
	task.InputPolicy = strategies.RejectUncoveredInput
	quotes := task.Quotes
	for _, quote := range quotes {
		task.Quotes = []liquidlane.FillQuote{quote}
		result, err := greedy.SolveFill(task)
		if err != nil {
			return nil, err
		}
		if result == nil {
			continue
		}
		id := liquidlane.NewCandidateID(quote.Route, quote.DiscountID)
		comparison := result.MaxAmountOut().Cmp(best.MaxAmountOut())
		if best == nil || comparison > 0 || comparison == 0 && id < bestID {
			best, bestID = result, id
		}
	}
	return best, nil
}
