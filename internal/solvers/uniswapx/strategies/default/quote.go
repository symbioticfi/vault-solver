package defaultstrategy

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
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

	validAfter := input.QuoteExpiresAt.Add(s.executionBuffer)
	inventory := liquidgreedy.AllocateInventoryCapacity(
		liquidgreedy.FilterLiveInventory(input.Inventory, validAfter), input.Reservations, s.cfg.InventoryReserveBps,
	)
	candidates := make([]liquidlane.QuoteCandidate, 0, len(inventory))
	for _, item := range inventory {
		if item.TokenIn != input.TokenIn || item.TokenOut != input.TokenOut {
			continue
		}
		candidate := liquidgreedy.NewQuoteCandidate(
			item,
			liquidgreedy.QuoteCapacity(item, s.cfg.PriceBufferBps),
		)
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	pricing, err := liquidstrategies.NewGasPricing(
		input.MaxFeePerGas,
		input.TokenOut,
		input.GasPrices,
		input.GasSnapshot,
		s.cfg.InventoryReserveBps,
		types.LiquidLaneGasEnvelope(),
	)
	if err != nil {
		return nil, err
	}
	maxRoutes := types.MaxRoutes
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	solution, err := liquidgreedy.SolveQuote(liquidgreedy.QuoteTask{
		ExactInput: input.AmountIn, ExactOutput: input.AmountOut,
		Candidates: candidates, MaxRoutes: maxRoutes, MinInput: s.minAmount,
		OutputBufferBps: 2 * s.cfg.PriceBufferBps,
		GasPricing:      &pricing,
	})
	if err != nil || solution == nil {
		return nil, err
	}
	return &types.Quote{AmountIn: solution.AmountIn, AmountOut: solution.AmountOut}, nil
}
