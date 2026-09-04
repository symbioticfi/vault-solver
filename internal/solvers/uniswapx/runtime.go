package uniswapx

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/go-errors/errors"
	"golang.org/x/sync/errgroup"
)

func (solver *Solver) Run(ctx context.Context) error {
	routes, err := solver.prepare(ctx)
	if err != nil {
		return err
	}
	solver.log.Info("starting", "chainId", solver.chainID, "solverMode", solver.cfg.SolverMode,
		"reactor", solver.cfg.Reactor.Hex(), "executor", solver.cfg.Executor.Hex(),
		"routes", len(routes), "gasAccounting", solver.cfg.Gas != nil,
		"listen", solver.cfg.QuoteServer.ListenAddress, "orderApi", solver.cfg.OrderServer.BaseURL)

	server := solver.newQuoteHTTPServer()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return errors.Errorf("listen for quotes: %w", err)
	}
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error { return solver.serveQuoteServer(server, listener) })
	group.Go(func() error {
		<-groupContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(groupContext), 2*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	})
	group.Go(func() error { return solver.refreshLoop(groupContext, routes) })
	orders := make(chan *resolvedOrder, orderQueueCapacity)
	group.Go(func() error { return solver.orderLoop(groupContext, orders) })
	worker := &fillWorker{solver: solver, ctx: groupContext, routes: routes, orders: orders, now: time.Now}
	group.Go(worker.run)
	return group.Wait()
}

func (solver *Solver) serveQuoteServer(server *http.Server, listener net.Listener) error {
	err := server.Serve(listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return errors.Errorf("serve quote server: %w", err)
}
