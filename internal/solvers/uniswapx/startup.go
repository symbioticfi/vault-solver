package uniswapx

import (
	"context"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func (solver *Solver) prepare(ctx context.Context) ([]liquidlane.Route, error) {
	routes, err := solver.reader.ResolveRoutes(ctx, solver.cfg.Adapters)
	if err != nil {
		return nil, solver.startupFailure(errors.Errorf("resolve routes: %w", err), "adapter resolution failed",
			"solverMode", solver.cfg.SolverMode, "executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
	}
	if len(routes) == 0 && solver.cfg.restrictsToAdapters() {
		return nil, solver.startupFailure(errors.New("no LiquidLane routes resolved"), "adapter resolution failed",
			"solverMode", solver.cfg.SolverMode, "executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
	}
	if err := solver.reader.validateExecutorCode(ctx, solver.cfg.Executor); err != nil {
		return nil, solver.startupFailure(errors.Errorf("validate executor: %w", err),
			"executor validation failed", "executor", solver.cfg.Executor.Hex())
	}
	if err := solver.reader.validateExecutorCaller(ctx, solver.cfg.Executor, solver.solverAddress); err != nil {
		return nil, solver.startupFailure(errors.Errorf("validate executor caller: %w", err),
			"executor caller validation failed", "executor", solver.cfg.Executor.Hex(), "caller", solver.solverAddress.Hex())
	}
	if solver.cfg.restrictsToAdapters() {
		unauthorized, authErr := solver.reader.unauthorizedAdapters(ctx, solver.cfg.Executor, routes)
		if authErr != nil {
			return nil, solver.startupFailure(errors.Errorf("validate adapters: %w", authErr),
				"adapter validation failed", "solverMode", solver.cfg.SolverMode,
				"executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
		}
		if len(unauthorized) > 0 {
			return nil, solver.startupFailure(errors.Errorf(
				"validate adapters: executor %s is not authorized as direct filler for configured adapters: %v",
				solver.cfg.Executor.Hex(), unauthorized,
			), "adapter validation failed", "solverMode", solver.cfg.SolverMode,
				"executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
		}
	}
	if err := solver.reader.ValidateGasTokens(routes); err != nil {
		return nil, solver.startupFailure(errors.Errorf("validate adapter gas tokens: %w", err),
			"adapter validation failed", "executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
	}
	if _, err := solver.orders.openOrders(ctx, solver.chainID, &solver.cfg.Executor); err != nil {
		return nil, solver.startupFailure(errors.Errorf("validate exclusive order delivery: %w", err),
			"exclusive order delivery validation failed", "executor", solver.cfg.Executor.Hex(),
			"orderApi", solver.cfg.OrderServer.BaseURL)
	}
	solver.breaker.exclusiveUnknown.Store(true)
	solver.breaker.warmupUntil.Store(time.Now().Add(solver.cfg.QuoteServer.QuoteTTL).Unix())
	if err := solver.refreshQuoteState(ctx, routes); err != nil {
		return nil, solver.startupFailure(errors.Errorf("initial quote refresh: %w", err),
			"initial quote refresh failed", "routes", len(routes))
	}
	return routes, nil
}
