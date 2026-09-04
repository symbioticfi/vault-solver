package uniswapx

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
)

func (s *defaultPlanner) DecideQuote(_ context.Context, input QuoteInput) (*Quote, error) {
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
	liveInventory := liquidplanning.FilterLiveInventory(input.Inventory, validAfter)
	pairInventory := make([]liquidlane.Inventory, 0, len(liveInventory))
	for _, item := range liveInventory {
		if item.TokenIn == input.TokenIn && item.TokenOut == input.TokenOut {
			pairInventory = append(pairInventory, item)
		}
	}
	inventory, candidates := s.policy.QuoteCandidates(pairInventory, input.Reservations)
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

	pricing, err := liquidplanning.NewGasPricing(
		input.MaxFeePerGas,
		input.TokenOut,
		input.GasPrices,
		input.GasSnapshot,
		s.policy.InventoryReserveBps,
		liquidplanning.ExecutorGasEnvelope(),
	)
	if err != nil {
		return nil, err
	}
	solution, err := liquidplanning.SolveQuote(liquidplanning.QuoteTask{
		ExactInput: input.AmountIn, ExactOutput: input.AmountOut,
		Candidates: candidates, MaxRoutes: input.RouteLimit(), MinInput: s.policy.MinAmount,
		OutputBufferBps: 2 * s.policy.PriceBufferBps,
		GasPricing:      &pricing,
		Trace:           input.Trace,
	})
	if err != nil || solution == nil {
		return nil, err
	}
	return &Quote{AmountIn: solution.AmountIn, AmountOut: solution.AmountOut}, nil
}
