package lifi

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
)

func (s *defaultPlanner) DecideFill(ctx context.Context, input FillInput) (FillDecision, error) {
	plan, err := s.buildFill(ctx, input)
	if err != nil || plan != nil || len(input.Reservations) == 0 {
		return FillDecision{Plan: plan}, err
	}
	unreserved := input
	unreserved.Reservations = nil
	plan, err = s.buildFill(ctx, unreserved)
	if err != nil || plan == nil {
		return FillDecision{}, err
	}
	return FillDecision{CapacityBlocked: capacityBlocked(input.Reservations, plan)}, nil
}

func (s *defaultPlanner) buildFill(_ context.Context, input FillInput) (*liquidlane.Plan, error) {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return nil, MarkPermanentFillDecisionError(errors.New("amountIn: must be positive"))
	}
	if input.OutputAmount == nil || input.OutputAmount.Sign() <= 0 {
		return nil, MarkPermanentFillDecisionError(errors.New("outputAmount: must be positive"))
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
	deadlineCutoff := uint32Time(validAfter)
	if input.Expires != 0 && input.Expires <= deadlineCutoff {
		input.Trace.Decline(
			"fill", "expiry-too-close",
			"expires", input.Expires,
			"validAfter", validAfter,
		)
		return nil, nil
	}
	if input.FillDeadline != 0 && input.FillDeadline <= deadlineCutoff {
		input.Trace.Decline(
			"fill", "fill-deadline-too-close",
			"fillDeadline", input.FillDeadline,
			"validAfter", validAfter,
		)
		return nil, nil
	}
	output, err := parseOutputContext(input.OutputAmount, input.OutputContext)
	if err != nil {
		return nil, MarkPermanentFillDecisionError(err)
	}
	allocation, err := s.policy.SolveFill(liquidplanning.PolicyFillTask{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		Quotes: input.Quotes, Reservations: input.Reservations, ValidAfter: validAfter,
		MaxRoutes:    input.RouteLimit(),
		MaxFeePerGas: input.MaxFeePerGas, GasPrices: input.GasPrices, GasSnapshot: input.GasSnapshot,
		GasEnvelope: liquidplanning.ExecutorGasEnvelope(), Trace: input.Trace,
	})
	if err != nil || allocation == nil {
		return nil, err
	}
	requiredAmountOut, ok := output.fill(input.Solver, input.ChainTime, allocation.MaxAmountOut())
	if !ok {
		input.Trace.Decline(
			"fill", "output-condition-not-satisfied",
			"requiredAmountOut", input.OutputAmount.String(),
			"maxAmountOut", allocation.MaxAmountOut().String(),
			"exclusive", output.exclusive,
			"exclusiveUntil", output.startTime,
		)
		return nil, nil
	}
	routes := allocation.Finalize(requiredAmountOut)
	if len(routes) == 0 {
		input.Trace.Decline(
			"fill", "finalization-failed",
			"requiredAmountOut", requiredAmountOut.String(),
			"maxAmountOut", allocation.MaxAmountOut().String(),
		)
		return nil, nil
	}
	return &liquidlane.Plan{Routes: routes}, nil
}

func capacityBlocked(
	reservations liquidlane.CapacityReservations,
	plan *liquidlane.Plan,
) bool {
	for _, route := range plan.Routes {
		if reserved := reservations[route.CapacityID]; reserved != nil && reserved.Sign() > 0 {
			return true
		}
	}
	return false
}
