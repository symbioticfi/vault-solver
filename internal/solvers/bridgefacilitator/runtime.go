package bridgefacilitator

import (
	"context"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

func (solver *Solver) Run(ctx context.Context) error {
	if err := solver.refreshTargetsAndHydrate(ctx); err != nil {
		return err
	}
	if solver.cfg.Targets != nil && len(solver.targets) == 0 {
		return errors.Errorf(
			"no configured adapter passed startup validation (must resolve and accept this solver %s as an authorized offer signer via ERC-1271); see per-adapter warnings above",
			solver.signerAddr.Hex(),
		)
	}

	solver.log.Info("starting",
		"adapters", len(solver.targets),
		"apiBaseUrl", solver.cfg.APIBaseURL,
		"discover", solver.cfg.Intervals.Discover.String(),
	)
	var wg sync.WaitGroup
	wg.Go(func() { solver.offerLoop(ctx) })
	wg.Go(func() { solver.redeemLoop(ctx) })
	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (solver *Solver) offerLoop(ctx context.Context) {
	discover := time.NewTicker(solver.cfg.Intervals.Discover)
	reconcile := time.NewTicker(solver.cfg.Intervals.Reconcile)
	defer discover.Stop()
	defer reconcile.Stop()
	solver.discoverAndOffer(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-discover.C:
			if err := solver.refreshTargetsAndHydrate(ctx); err != nil {
				solver.log.Error(err, "refresh adapters; keeping last-known-good targets")
			}
			solver.discoverAndOffer(ctx)
		case <-reconcile.C:
			solver.reconcile(ctx)
		}
	}
}

func (solver *Solver) redeemLoop(ctx context.Context) {
	ticker := time.NewTicker(solver.cfg.Intervals.RedeemPoll)
	defer ticker.Stop()
	solver.redeemAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			solver.redeemAll(ctx)
		}
	}
}

func (solver *Solver) redeemAll(ctx context.Context) {
	targets := solver.redeemable.Load()
	if targets == nil {
		return
	}
	timer := observability.StartOperation(solver.metrics.operation(redeemableRefreshOperation))
	ready, successful := 0, 0
	for _, target := range *targets {
		count, err := solver.redeemReady(ctx, target)
		ready += count
		if err != nil {
			solver.log.Error(err, "redeem target", "adapter", target.Adapter.Hex())
			continue
		}
		successful++
	}
	outcome := observability.Completeness(successful, len(*targets))
	if outcome == observability.ExternalOperationSuccess {
		solver.metrics.state("redeemable", ready)
	}
	timer.Finish(ctx, outcome)
}

func (solver *Solver) reconcile(ctx context.Context) {
	timer := observability.StartOperation(solver.metrics.operation(activeRefreshOperation))
	total, successful := 0, 0
	for _, target := range solver.targets {
		state, err := solver.reader.liquidityAndExposure(ctx, target.Adapter)
		if err != nil {
			solver.log.Error(err, "reconcile", "adapter", target.Adapter.Hex())
			continue
		}
		successful++
		total += state.openCount
		solver.log.Info("reconcile", "adapter", target.Adapter.Hex(),
			"openRequests", state.openCount, "fundable", state.fundable.String())
	}
	outcome := observability.Completeness(successful, len(solver.targets))
	if outcome == observability.ExternalOperationSuccess {
		solver.metrics.state("active_requests", total)
	}
	timer.Finish(ctx, outcome)
}
