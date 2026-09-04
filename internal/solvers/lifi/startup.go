package lifi

import (
	"context"

	"github.com/go-errors/errors"
)

func (solver *Solver) prepare(ctx context.Context) ([]route, error) {
	routes, err := solver.reader.ResolveRoutes(ctx, solver.cfg.Adapters)
	if err != nil {
		return nil, solver.startupFailure(errors.Errorf("lifi: resolve routes: %w", err), "adapter resolution failed",
			"solverMode", solver.cfg.SolverMode, "executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
	}
	if len(routes) == 0 {
		return nil, solver.startupFailure(errors.New("lifi: no quoteable routes resolved from configured adapters"),
			"adapter resolution failed", "solverMode", solver.cfg.SolverMode,
			"executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
	}
	if err := solver.reader.ValidateGasTokens(routes); err != nil {
		return nil, solver.startupFailure(errors.Errorf("lifi: validate gas oracles: %w", err),
			"gas oracle validation failed", "routes", len(routes), "gasAccounting", solver.cfg.Gas != nil)
	}
	if err := solver.reader.validateExecutor(
		ctx, solver.cfg.Executor, solver.cfg.InputSettler, solver.cfg.OutputSettler, solver.caller,
	); err != nil {
		return nil, solver.startupFailure(errors.Errorf("lifi: validate executor: %w", err),
			"executor validation failed", "executor", solver.cfg.Executor.Hex(), "caller", solver.caller.Hex(),
			"inputSettler", solver.cfg.InputSettler.Hex(), "outputSettler", solver.cfg.OutputSettler.Hex())
	}
	if err := solver.reader.validateZeroGovernanceFee(ctx, solver.cfg.InputSettler); err != nil {
		return nil, solver.startupFailure(errors.Errorf("lifi: validate governance fee: %w", err),
			"governance fee validation failed", "inputSettler", solver.cfg.InputSettler.Hex())
	}
	if !solver.cfg.usesDiscounts() {
		if err := solver.reader.validateDirectAuthorization(ctx, solver.cfg.Executor, routes); err != nil {
			return nil, solver.startupFailure(errors.Errorf("lifi: validate direct authorization: %w", err),
				"external adapter authorization failed", "solverMode", solver.cfg.SolverMode,
				"executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
		}
	}
	if err := solver.orders.validateExecutorRegistration(ctx, solver.cfg.Executor); err != nil {
		return nil, solver.startupFailure(err, "executor registration validation failed",
			"executor", solver.cfg.Executor.Hex(), "baseUrl", solver.cfg.OrderServer.BaseURL)
	}
	if err := solver.orders.ensureSupportedContracts(
		ctx, solver.chainID, solver.cfg.InputSettler, solver.cfg.OutputSettler,
	); err != nil {
		return nil, solver.startupFailure(err, "supported contract reconciliation failed", "chainId", solver.chainID,
			"inputSettler", solver.cfg.InputSettler.Hex(), "outputSettler", solver.cfg.OutputSettler.Hex())
	}
	return routes, nil
}

func (solver *Solver) startupFailure(err error, message string, keysAndValues ...any) error {
	solver.log.Error(err, message, keysAndValues...)
	return err
}
