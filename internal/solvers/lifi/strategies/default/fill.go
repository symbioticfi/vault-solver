package defaultstrategy

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func (s *Strategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if input.AmountIn == nil || input.AmountIn.Sign() <= 0 {
		return nil, errors.New("amountIn: must be positive")
	}
	if input.OutputAmount == nil || input.OutputAmount.Sign() <= 0 {
		return nil, errors.New("outputAmount: must be positive")
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
		return nil, err
	}
	maxRoutes := types.MaxRoutes
	if input.RequireSingleRoute {
		maxRoutes = 1
	}
	solution, err := s.solveGreedyFill(input, validAfter, maxRoutes)
	if err != nil || solution == nil {
		return nil, err
	}
	requiredAmountOut, ok := output.fill(input.Solver, input.ChainTime, solution.maxAmountOut)
	if !ok {
		return nil, nil
	}
	routes := solution.buildRoutes(requiredAmountOut)
	if len(routes) == 0 {
		return nil, nil
	}
	return &types.FillPlan{Routes: routes}, nil
}
