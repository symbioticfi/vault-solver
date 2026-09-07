package defaultstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

func (s *Strategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if ready, err := input.CheckAmounts(s.policy.MinAmount); err != nil || !ready {
		return nil, err
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
	allocation, err := input.Allocate(s.policy, types.LiquidLaneGasEnvelope(), types.MaxRoutes)
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
