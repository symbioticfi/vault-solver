package defaultstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

func (s *Strategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return nil, errors.New("amountIn: must be positive")
	}
	if input.OutputAmount == nil || input.OutputAmount.Sign() <= 0 {
		return nil, errors.New("outputAmount: must be positive")
	}
	if input.AmountIn.Cmp(s.policy.MinAmount) < 0 {
		input.Trace.Decline(
			"fill", "amount-below-minimum",
			"amountIn", input.AmountIn.String(),
			"minAmount", s.policy.MinAmount.String(),
		)
		return nil, nil
	}
	validAfter := input.ChainTime.Add(s.policy.ExecutionBuffer)
	if input.Deadline != 0 && int64(input.Deadline) <= validAfter.Unix() {
		input.Trace.Decline(
			"fill", "deadline-too-close",
			"deadline", input.Deadline,
			"validAfter", validAfter,
		)
		return nil, nil
	}
	maxRoutes := types.MaxRoutes
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	gasPricing, err := planning.NewGasPricing(
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
	allocation, err := planning.SolveFill(planning.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		Quotes: input.Quotes, Reservations: input.Reservations, ValidAfter: validAfter,
		MaxRoutes: maxRoutes, PriceBufferBps: s.policy.PriceBufferBps,
		InventoryReserveBps: s.policy.InventoryReserveBps,
		GasPricing:          &gasPricing,
		Trace:               input.Trace,
	})
	if err != nil || allocation == nil {
		return nil, err
	}
	maxAmountOut := allocation.MaxAmountOut()
	if input.OutputAmount.Cmp(maxAmountOut) > 0 {
		input.Trace.Decline(
			"fill", "required-output-exceeds-capacity",
			"requiredAmountOut", input.OutputAmount.String(),
			"maxAmountOut", maxAmountOut.String(),
		)
		return nil, nil
	}
	routes := allocation.Finalize(input.OutputAmount)
	if len(routes) == 0 {
		input.Trace.Decline(
			"fill", "finalization-failed",
			"requiredAmountOut", input.OutputAmount.String(),
			"maxAmountOut", maxAmountOut.String(),
		)
		return nil, nil
	}
	return &types.FillPlan{Routes: routes}, nil
}
