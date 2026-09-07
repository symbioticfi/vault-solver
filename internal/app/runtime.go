package app

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

type transactionLane interface {
	Start(context.Context)
	LaneReady() bool
	SubscribeLaneState() (<-chan struct{}, func())
}

// supervise owns every goroutine it starts. Solvers stop intake on ctx cancellation, while the
// detached transaction context continues accepting their bounded shutdown work and drains results.
func supervise(ctx context.Context, fail context.CancelCauseFunc, services []service,
	lane transactionLane, health *observability.Health, drain time.Duration, log logr.Logger,
) error {
	var background sync.WaitGroup
	stopLane := func() {}
	if lane != nil {
		laneCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		stopLane = cancel
		background.Go(func() { lane.Start(laneCtx) })
		changes, unsubscribe := lane.SubscribeLaneState()
		background.Go(func() {
			defer unsubscribe()
			watchReadiness(ctx, changes, lane.LaneReady, health.SetReady)
		})
	}
	defer func() { health.SetReady(false); stopLane(); background.Wait() }()
	if ctx.Err() == nil {
		health.SetReady(lane == nil || lane.LaneReady())
	}

	var workers sync.WaitGroup
	for _, svc := range services {
		workers.Go(func() {
			err := solver.Run(ctx, svc.Solver, svc.log)
			if err != nil {
				fail(err)
			} else if ctx.Err() == nil {
				fail(errors.Errorf("solver %q exited before shutdown", svc.Name()))
			}
		})
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	if lane != nil {
		monitorTransactionDrain(ctx.Done(), done, drain, func() {
			log.Info("solver drain deadline reached; stopping transaction lane", "timeout", drain)
			stopLane()
		})
	}
	<-done
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	return nil
}

func watchReadiness(ctx context.Context, changes <-chan struct{}, ready func() bool, set func(bool)) {
	defer set(false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			// A late lane notification must never reopen readiness during shutdown.
			if ctx.Err() == nil {
				set(ready())
			}
		}
	}
}

func monitorTransactionDrain(shutdown, done <-chan struct{}, timeout time.Duration, stop func()) {
	select {
	case <-done:
		return
	case <-shutdown:
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		stop()
	}
}

// serve joins the server goroutine even on an unexpected listener failure. Shutdown gets a fresh
// bounded context; Close releases idle or misbehaving connections if that deadline is exhausted.
func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.Errorf("serve observability: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			_ = server.Close()
		}
		<-done
		if err != nil {
			return errors.Errorf("shutdown observability: %w", err)
		}
		return nil
	}
}
