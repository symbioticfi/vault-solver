package defaultstrategy

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

func (s *Strategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return nil, errors.Errorf("amountIn %v: must be positive", input.AmountIn)
	}
	if input.OutputAmount == nil || input.OutputAmount.Sign() <= 0 {
		return nil, errors.Errorf("outputAmount %v: must be positive", input.OutputAmount)
	}
	if input.AmountIn.Cmp(s.minAmount) < 0 {
		return nil, errors.Errorf("%w: amount in %s is below minimum amount %s",
			types.ErrFillDeclined, input.AmountIn, s.minAmount)
	}
	validAfter := input.ChainTime.Add(s.executionBuffer)
	if input.Deadline != 0 && int64(input.Deadline) <= validAfter.Unix() {
		return nil, errors.Errorf("%w: order deadline %d is not after execution valid-after %d (chain time %d + buffer %s)",
			types.ErrFillDeclined, input.Deadline, validAfter.Unix(), input.ChainTime.Unix(), s.executionBuffer)
	}
	maxRoutes := types.MaxRoutes
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	gasPricing, err := s.fillPricing(input)
	if err != nil {
		return nil, err
	}
	trace, lastDecline := input.Trace.RecordDeclines()
	allocation, err := s.solveFill(strategies.FillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		Quotes: input.Quotes, Reservations: input.Reservations, ValidAfter: validAfter,
		CapacityLimits: input.CapacityLimits,
		MaxRoutes:      maxRoutes, PriceBufferBps: s.cfg.PriceBufferBps,
		InventoryReserveBps: s.cfg.InventoryReserveBps,
		GasPricing:          &gasPricing,
		Trace:               trace,
	})
	if err != nil {
		return nil, err
	}
	if allocation == nil {
		return nil, errors.Errorf("%w: no allocation over %d fill quotes: %s",
			types.ErrFillDeclined, len(input.Quotes), lastDecline())
	}
	maxAmountOut := allocation.MaxAmountOut()
	if input.OutputAmount.Cmp(maxAmountOut) > 0 {
		return nil, errors.Errorf("%w: required amount out %s exceeds max allocatable amount out %s",
			types.ErrFillDeclined, input.OutputAmount, maxAmountOut)
	}
	routes := allocation.Finalize(input.OutputAmount)
	if len(routes) == 0 {
		return nil, errors.Errorf("%w: finalizing required amount out %s against max %s produced no routes",
			types.ErrFillDeclined, input.OutputAmount, maxAmountOut)
	}
	return &types.FillPlan{Routes: routes}, nil
}

func (s *Strategy) priceFillGas(input types.FillInput) (strategies.GasPricing, error) {
	return strategies.NewGasPricing(input.MaxFeePerGas, input.TokenOut,
		input.GasPrices, input.GasSnapshot, s.cfg.InventoryReserveBps, types.LiquidLaneGasEnvelope())
}
