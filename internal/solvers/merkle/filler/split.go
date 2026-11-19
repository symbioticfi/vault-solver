package filler

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

// Leg is one connector's share of a settlement: AmountIn of the input, for the payment the
// connector prices for it.
type Leg struct {
	Source

	AmountIn *big.Int
}

// Split plans amountIn across at most maxRoutes of the routes' connectors when no single one
// settles it: the same exact-input allocator quotes use, at fresh connector prices. Snapshot
// reservations are charged once per funding account and output asset. It returns nil when the
// connectors cannot pay minAmountOut together, and an error when that may be only because a
// connector could not be priced.
func (e *Engine) Split(ctx context.Context, routes []Source, amountIn, minAmountOut *big.Int, maxRoutes int, at Observation) ([]Leg, error) {
	distinct := make(map[common.Address]bool)
	for _, route := range routes {
		distinct[route.Venue] = true
	}
	maxRoutes = min(maxRoutes, len(distinct))
	if maxRoutes < 2 {
		return nil, nil
	}
	listings := make([]discounts.ListItem, len(routes))
	for i, route := range routes {
		listings[i] = route.Listing
	}
	// The connector reads quotes are priced from, taken now.
	inventory, skipped, err := e.Inventory(ctx, listings)
	if err != nil {
		return nil, errors.Errorf("price connectors for a split: %w", err)
	}
	inventory = greedy.FilterLiveInventory(inventory, at.Chain)
	available := greedy.AllocateInventoryCapacity(inventory, e.Reservations(inventory), 0)
	candidates := make([]liquidlane.QuoteCandidate, 0, len(available))
	executors := make(map[liquidlane.RouteID]common.Address, len(available))
	for _, item := range available {
		if candidate := greedy.NewQuoteCandidate(item, item.MaxAssets); candidate != nil {
			candidates = append(candidates, *candidate)
			executors[item.ID] = item.RecipeExecutor
		}
	}
	solution, err := greedy.SolveQuote(strategies.QuoteTask{
		ExactInput: amountIn, Candidates: candidates, MaxRoutes: maxRoutes,
		InputPolicy: strategies.RejectUncoveredInput,
	})
	if err == nil && (solution == nil || solution.AmountOut.Cmp(minAmountOut) < 0) && skipped != nil {
		// A connector that could not be read might have covered the rest: retry, do not decline.
		err = errors.Errorf("price connectors for a split: %w", skipped)
	}
	if err != nil || solution == nil || solution.AmountOut.Cmp(minAmountOut) < 0 {
		return nil, err
	}
	legs := make([]Leg, 0, len(solution.Allocations))
	for _, allocation := range solution.Allocations {
		c := allocation.Candidate
		legs = append(legs, Leg{
			Venue: c.Route.Adapter, RecipeExecutor: executors[c.Route.ID], DiscountID: *c.DiscountID, MaxAssets: c.MaxAmountOut,
			AmountIn: allocation.AmountIn,
		})
	}
	return legs, nil
}
