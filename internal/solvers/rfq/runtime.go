package rfq

import (
	"context"
	"net/http"
	"time"

	"github.com/go-errors/errors"
)

func (solver *Solver) Run(ctx context.Context) error {
	if err := solver.prepareAdapters(ctx); err != nil {
		return err
	}
	solver.log.Info("starting",
		"listenAddr", solver.cfg.ListenAddr,
		"executor", solver.cfg.Executor.Hex(),
		"reactor", solver.cfg.Reactor.Hex(),
		"solverMode", solver.cfg.SolverMode,
		"adapters", len(solver.cfg.Adapters),
		"backendUrl", solver.cfg.BackendURL,
	)

	httpServer := &http.Server{
		Addr: solver.cfg.ListenAddr, Handler: solver.server.handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()
	solver.log.Info("quote server listening", "addr", solver.cfg.ListenAddr)

	executionContext, stopExecution := context.WithCancel(ctx)
	executionDone := make(chan struct{})
	go func() {
		defer close(executionDone)
		solver.exec.run(executionContext, solver.cfg.PollInterval)
	}()

	var result error
	select {
	case <-ctx.Done():
		result = ctx.Err()
	case err := <-serverErrors:
		result = errors.Errorf("rfq: quote server failed: %w", err)
	}
	stopExecution()

	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), quoteServerShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		solver.log.Error(err, "quote server graceful shutdown failed")
		if closeErr := httpServer.Close(); closeErr != nil {
			solver.log.Error(closeErr, "quote server forced shutdown failed")
		}
	}
	<-executionDone
	return result
}

func (solver *Solver) prepareAdapters(ctx context.Context) error {
	if len(solver.cfg.Adapters) == 0 {
		return nil
	}
	resolved, err := solver.exec.reader.resolveVaults(ctx, solver.cfg.Adapters)
	if err != nil {
		startupErr := errors.Errorf("rfq: resolve recovery vaults: %w", err)
		solver.log.Error(startupErr, "adapter resolution failed",
			"solverMode", solver.cfg.SolverMode, "executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
		return startupErr
	}
	solver.exec.vaults = resolved
	solver.exec.reader.setQuoteAdapters(resolved)
	if !solver.cfg.restrictsToAdapters() {
		return nil
	}
	if err := solver.exec.reader.validateDirectAuthorization(ctx, solver.cfg.Executor, resolved); err != nil {
		startupErr := errors.Errorf("rfq: validate direct authorization: %w", err)
		solver.log.Error(startupErr, "external adapter authorization failed",
			"solverMode", solver.cfg.SolverMode, "executor", solver.cfg.Executor.Hex(), "adapters", solver.cfg.Adapters)
		return startupErr
	}
	return nil
}
