package defaultstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func (s *Strategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if ready, err := input.CheckAmounts(s.policy.MinAmount); err != nil || !ready {
		return nil, types.MarkPermanentFillDecisionError(err)
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
		return nil, types.MarkPermanentFillDecisionError(err)
	}
	allocation, err := input.Allocate(s.policy, types.LiquidLaneGasEnvelope(), types.MaxRoutes)
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
