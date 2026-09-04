package app

import (
	"context"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
)

// TransactionLane is the process-owned nonce lane shared by transaction-submitting integrations.
type TransactionLane interface {
	ValidateFeeHeadroom() error
	Initialize(context.Context) error
	Start(context.Context)
	SubscribeLaneState() (<-chan struct{}, func())
	LaneReady() bool
}

type RunConfig struct {
	ConfigSource            string
	RequiresTransactionLane bool
	ValidateTxConfig        func() error
	PendingTimeout          time.Duration
	ReplacementInterval     time.Duration
}

// Run owns transaction-lane startup, integration cancellation, readiness, and bounded drain.
func Run(
	ctx context.Context,
	cfg RunConfig,
	lane TransactionLane,
	integrations []Integration,
	ready func(bool),
	log logr.Logger,
) error {
	if cfg.RequiresTransactionLane {
		if lane == nil {
			return errors.New("transaction lane is required but not configured")
		}
		if err := cfg.ValidateTxConfig(); err != nil {
			return errors.Errorf("invalid config %q: %w", cfg.ConfigSource, err)
		}
		if err := lane.ValidateFeeHeadroom(); err != nil {
			return errors.Errorf("invalid config %q: txManager: %w", cfg.ConfigSource, err)
		}
		if err := lane.Initialize(ctx); err != nil {
			return errors.Errorf("initialize tx manager: %w", err)
		}
	}

	stopLane, laneDone := startLane(ctx, cfg.RequiresTransactionLane, lane)
	if stopLane != nil {
		defer func() { stopLane(); <-laneDone }()
	}

	ready(true)
	group, groupCtx := errgroup.WithContext(ctx)
	var observer sync.WaitGroup
	if cfg.RequiresTransactionLane {
		changes, unsubscribe := lane.SubscribeLaneState()
		observer.Go(func() {
			defer unsubscribe()
			for {
				select {
				case <-changes:
					ready(lane.LaneReady())
				case <-groupCtx.Done():
					ready(false)
					return
				}
			}
		})
	}
	for _, integration := range integrations {
		group.Go(func() error { return runOne(groupCtx, integration, log) })
	}

	drained := make(chan struct{})
	monitorDone := monitorDrain(groupCtx, drained, stopLane, drainTimeout(cfg, integrations), log)
	err := group.Wait()
	close(drained)
	<-monitorDone
	observer.Wait()
	if err != nil {
		return err
	}
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	return nil
}

func runOne(ctx context.Context, integration Integration, log logr.Logger) error {
	logger := log.WithValues("solver", integration.Name())
	logger.Info("solver started")
	err := integration.Run(ctx)
	if err == nil || errors.Is(err, context.Canceled) {
		logger.Info("solver stopped")
		return nil
	}
	return errors.Errorf("solver %q: %w", integration.Name(), err)
}

func startLane(parent context.Context, enabled bool, lane TransactionLane) (context.CancelFunc, <-chan struct{}) {
	if !enabled {
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	done := make(chan struct{})
	go func() { defer close(done); lane.Start(ctx) }()
	return cancel, done
}

func monitorDrain(
	ctx context.Context,
	drained <-chan struct{},
	stop context.CancelFunc,
	timeout time.Duration,
	log logr.Logger,
) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if stop == nil {
			return
		}
		select {
		case <-ctx.Done():
		case <-drained:
			return
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			log.Info("solver shutdown timed out; stopping tx manager", "timeout", timeout.String())
			stop()
		case <-drained:
		}
	}()
	return done
}

func drainTimeout(cfg RunConfig, integrations []Integration) time.Duration {
	longest := time.Duration(0)
	for _, integration := range integrations {
		if preparer, ok := integration.(ShutdownPreparer); ok {
			longest = max(longest, preparer.ShutdownPreparationTimeout())
		}
	}
	return longest + cfg.PendingTimeout + cfg.ReplacementInterval
}
