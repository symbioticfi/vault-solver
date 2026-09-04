package lifi

import (
	"context"

	"github.com/go-errors/errors"
)

func (solver *Solver) Run(ctx context.Context) error {
	routes, err := solver.prepare(ctx)
	if err != nil {
		return err
	}
	solver.log.Info("starting",
		"chainId", solver.chainID, "strategy", solver.cfg.Strategy.Name,
		"adapters", len(solver.cfg.Adapters), "routes", len(routes),
		"baseUrl", solver.cfg.OrderServer.BaseURL, "wsUrl", solver.cfg.OrderServer.WSURL,
		"quoteRefreshMode", solver.cfg.QuoteRefreshMode, "quoteInterval", solver.cfg.QuoteInterval.String(),
		"quoteTTL", solver.cfg.QuoteTTL.String(), "solverMode", solver.cfg.SolverMode,
		"privateDiscounts", solver.cfg.usesDiscounts(), "gasAccounting", solver.cfg.Gas != nil,
		"tokensToQuote", solver.cfg.TokenPolicy.Scope(), "executor", solver.cfg.Executor.Hex(),
		"caller", solver.caller.Hex(), "inputSettler", solver.cfg.InputSettler.Hex(),
		"outputSettler", solver.cfg.OutputSettler.Hex(),
	)
	solver.quoteRefresh = make(chan struct{}, 1)
	return solver.runLoops(ctx, routes)
}

func (solver *Solver) runLoops(ctx context.Context, routes []route) error {
	feedConnections := make(chan context.Context)
	feedContext, stopFeed := context.WithCancel(context.WithoutCancel(ctx))
	defer stopFeed()
	quoteContext, stopQuotes := context.WithCancel(ctx)
	defer stopQuotes()

	feedDone := make(chan error, 1)
	quoteDone := make(chan error, 1)
	go func() { feedDone <- solver.runOrderFeed(feedContext, routes, feedConnections) }()
	go func() { quoteDone <- solver.quoteLoop(quoteContext, routes, solver.quoteRefresh, feedConnections) }()
	select {
	case quoteErr := <-quoteDone:
		stopFeed()
		return preferLifecycleError(quoteErr, <-feedDone)
	case feedErr := <-feedDone:
		stopQuotes()
		return preferLifecycleError(feedErr, <-quoteDone)
	}
}

func preferLifecycleError(primary, secondary error) error {
	if primary != nil && !errors.Is(primary, context.Canceled) {
		return primary
	}
	if secondary != nil && !errors.Is(secondary, context.Canceled) {
		return secondary
	}
	return primary
}
