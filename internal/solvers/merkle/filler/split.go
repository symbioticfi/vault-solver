package filler

import (
	"context"
	"fmt"
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

	AmountIn  *big.Int
	AmountOut *big.Int // Estimated payout used for queued reservations.
}

// Split plans amountIn across at most maxRoutes of the routes' connectors when no single one
// settles it: the same exact-input allocator quotes use, at fresh connector prices. Snapshot
// reservations are charged once per funding account and output asset. When the connectors
// cannot pay minAmountOut together it returns ErrNotFillable naming the failed check, and an
// unclassified error when that may be only because a connector could not be priced.
func (e *Engine) Split(ctx context.Context, routes []Source, amountIn, minAmountOut *big.Int, maxRoutes int, at Observation, orderID string, price InventoryReader) ([]Leg, error) {
	distinct := make(map[common.Address]bool)
	for _, route := range routes {
		distinct[route.Venue] = true
	}
	maxRoutes = min(maxRoutes, len(distinct))
	if maxRoutes < 2 {
		return nil, errors.Errorf("%w: a split needs two connectors, %d distinct listed", ErrNotFillable, len(distinct))
	}
	listings := make([]discounts.ListItem, len(routes))
	for i, route := range routes {
		listings[i] = route.Listing
	}
	// The connector reads quotes are priced from, taken now.
	inventory, skipped, err := price(ctx, listings)
	if err != nil {
		return nil, errors.Errorf("price connectors for a split: %w", err)
	}
	priced := len(inventory)
	inventory = greedy.FilterLiveInventory(inventory, at.Chain)
	available := greedy.AllocateInventoryCapacity(inventory, e.Spends.ReservationsExcluding(inventory, orderID), 0)
	candidates := make([]liquidlane.QuoteCandidate, 0, len(available))
	executors := make(map[liquidlane.RouteID]common.Address, len(available))
	for _, item := range available {
		if candidate := greedy.NewQuoteCandidate(item, item.MaxAssets); candidate != nil {
			candidates = append(candidates, *candidate)
			executors[item.ID] = item.RecipeExecutor
		}
	}
	trace, lastDecline := strategies.DecisionTrace(nil).RecordDeclines()
	solution, err := greedy.SolveQuote(strategies.QuoteTask{
		ExactInput: amountIn, Candidates: candidates, MaxRoutes: maxRoutes,
		InputPolicy: strategies.RejectUncoveredInput, Trace: trace,
	})
	if err == nil && (solution == nil || solution.AmountOut.Cmp(minAmountOut) < 0) && skipped != nil {
		// A connector that could not be read might have covered the rest: retry, do not decline.
		err = errors.Errorf("price connectors for a split: %w", skipped)
	}
	if err != nil {
		return nil, err
	}
	funnel := fmt.Sprintf("%d of %d connectors priced, %d live, %d with unreserved capacity, %d quotable",
		priced, len(routes), len(inventory), len(available), len(candidates))
	if solution == nil {
		return nil, errors.Errorf("%w: split found no allocation (%s): %s", ErrNotFillable, funnel, lastDecline())
	}
	if solution.AmountOut.Cmp(minAmountOut) < 0 {
		return nil, errors.Errorf("%w: split pays %s below required amount out %s (%s)", ErrNotFillable, solution.AmountOut, minAmountOut, funnel)
	}
	legs := make([]Leg, 0, len(solution.Allocations))
	for _, allocation := range solution.Allocations {
		c := allocation.Candidate
		legs = append(legs, Leg{
			Venue: c.Route.Adapter, RecipeExecutor: executors[c.Route.ID],
			Account: c.Route.Vault, DiscountID: *c.DiscountID, MaxAssets: c.MaxAmountOut,
			AmountIn: allocation.AmountIn, AmountOut: allocation.AmountOut,
		})
	}
	return legs, nil
}
