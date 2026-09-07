package defaultstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

func (s *Strategy) DecideQuote(_ context.Context, input types.QuoteInput) (*types.Quote, error) {
	if !input.QuoteExpiresAt.After(input.ChainTime) {
		return nil, errors.New("quoteExpiresAt must be after chainTime")
	}
	if (input.AmountIn == nil) == (input.AmountOut == nil) {
		return nil, errors.New("exactly one quote amount must be set")
	}
	requestedAmount := input.AmountIn
	if requestedAmount == nil {
		requestedAmount = input.AmountOut
	}
	if requestedAmount.Sign() <= 0 {
		return nil, errors.New("quote amount must be positive")
	}

	validAfter := input.QuoteExpiresAt.Add(s.policy.ExecutionBuffer)
	liveInventory := planning.FilterLiveInventory(input.Inventory, validAfter)
	pairInventory := make([]liquidlane.Inventory, 0, len(liveInventory))
	for _, item := range liveInventory {
		if item.TokenIn == input.TokenIn && item.TokenOut == input.TokenOut {
			pairInventory = append(pairInventory, item)
		}
	}
	inventory := planning.AllocateInventoryCapacity(
		pairInventory,
		input.Reservations,
		s.policy.InventoryReserveBps,
	)
	candidates := planning.NormalizeFixedInventory(inventory, s.policy.PriceBufferBps)
	if len(candidates) == 0 {
		input.Trace.Decline(
			"quote", "no-matching-routes",
			"tokenIn", input.TokenIn.Hex(),
			"tokenOut", input.TokenOut.Hex(),
			"inventory", len(input.Inventory),
			"liveInventory", len(liveInventory),
			"pairInventory", len(pairInventory),
			"allocatedInventory", len(inventory),
			"reservations", len(input.Reservations),
		)
		return nil, nil
	}

	pricing, err := planning.NewGasPricing(
		input.MaxFeePerGas,
		input.TokenOut,
		input.GasPrices,
		input.GasSnapshot,
		s.policy.InventoryReserveBps,
		types.LiquidLaneGasEnvelope(),
	)
	if err != nil {
		return nil, err
	}
	maxRoutes := types.MaxRoutes
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	solution, err := planning.NewQuotePool(candidates).Solve(planning.QuoteTask{
		ExactInput: input.AmountIn, ExactOutput: input.AmountOut,
		MaxRoutes: maxRoutes, MinInput: s.policy.MinAmount,
		OutputBufferBps: 2 * s.policy.PriceBufferBps,
		GasPricing:      &pricing,
		Trace:           input.Trace,
	})
	if err != nil || solution == nil {
		return nil, err
	}
	return &types.Quote{AmountIn: solution.AmountIn, AmountOut: solution.AmountOut}, nil
}
