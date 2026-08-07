package defaultstrategy

import (
	"context"

	"github.com/go-errors/errors"

	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func (s *Strategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return nil, types.MarkPermanentFillDecisionError(errors.New("amountIn: must be positive"))
	}
	if input.OutputAmount == nil || input.OutputAmount.Sign() <= 0 {
		return nil, types.MarkPermanentFillDecisionError(errors.New("outputAmount: must be positive"))
	}
	if input.AmountIn.Cmp(s.minAmount) < 0 {
		return nil, nil
	}
	validAfter := input.ChainTime.Add(s.executionBuffer)
	deadlineCutoff := uint32Time(validAfter)
	if input.Expires != 0 && input.Expires <= deadlineCutoff {
		return nil, nil
	}
	if input.FillDeadline != 0 && input.FillDeadline <= deadlineCutoff {
		return nil, nil
	}
	output, err := parseOutputContext(input.OutputAmount, input.OutputContext)
	if err != nil {
		return nil, types.MarkPermanentFillDecisionError(err)
	}
	maxRoutes := types.MaxRoutes
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	gasPricing, err := liquidstrategies.NewGasPricing(
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
	allocation, err := liquidgreedy.SolveFill(liquidgreedy.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		Quotes: input.Quotes, Reservations: input.Reservations, ValidAfter: validAfter,
		MaxRoutes: maxRoutes, PriceBufferBps: s.cfg.PriceBufferBps,
		InventoryReserveBps: s.cfg.InventoryReserveBps,
		GasPricing:          &gasPricing,
	})
	if err != nil || allocation == nil {
		return nil, err
	}
	requiredAmountOut, ok := output.fill(input.Solver, input.ChainTime, allocation.MaxAmountOut())
	if !ok {
		return nil, nil
	}
	routes := allocation.Finalize(requiredAmountOut)
	if len(routes) == 0 {
		return nil, nil
	}
	return &types.FillPlan{Routes: routes}, nil
}

// DecideFillWithoutReservations lets the LI.FI worker distinguish a capacity-blocked order from
// any other terminal nil decision without issuing a second decision to external strategies.
func (s *Strategy) DecideFillWithoutReservations(
	ctx context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	input.Reservations = nil
	return s.DecideFill(ctx, input)
}
