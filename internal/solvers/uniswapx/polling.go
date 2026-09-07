package uniswapx

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// One poller serializes source snapshots and claims. An exclusive snapshot is
// publishable only after terminal history and existing obligations reconcile.
func (s *Solver) orderLoop(ctx context.Context, out chan<- *resolvedOrder) error {
	defer close(out)
	tick := time.NewTicker(s.cfg.OrderServer.PollInterval)
	defer tick.Stop()
	for ctx.Err() == nil {
		if err := s.pollOrders(ctx, out); err != nil && ctx.Err() == nil {
			s.log.Error(err, "order poll failed")
		}
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
	}
	return ctx.Err()
}

func (s *Solver) pollOrders(ctx context.Context, out chan<- *resolvedOrder) error {
	sources := []struct {
		enabled  bool
		name     orderSource
		filler   *common.Address
		observer *observability.OperationObserver
	}{
		{s.cfg.OrderServer.Sources.ExclusiveV2, orderSourceExclusiveV2, &s.cfg.Executor, s.operations.exclusiveOrderPoll},
		{s.cfg.OrderServer.Sources.PublicV2, orderSourcePublicV2, nil, s.operations.publicOrderPoll},
	}
	var failures []error
	for _, source := range sources {
		if !source.enabled {
			continue
		}
		timer := observability.StartOperation(source.observer)
		chainTime, err := s.pollSource(ctx, source.name, source.filler, out)
		outcome := observability.ExternalOperationSuccess
		// A read can deliver valid partial records while still withholding readiness.
		if err != nil {
			outcome = observability.ExternalOperationError
			if !chainTime.IsZero() {
				outcome = observability.ExternalOperationDegraded
			}
		} else if source.name == orderSourceExclusiveV2 {
			err = s.reconcileExclusivePoll(ctx, chainTime)
			if err != nil {
				outcome = observability.ExternalOperationError
			}
		}
		if err == nil {
			if source.name == orderSourceExclusiveV2 {
				s.recordExclusivePollSuccess(time.Now())
			}
			s.observePoll(string(source.name), "ok")
		} else {
			if source.name == orderSourceExclusiveV2 {
				s.markExclusiveStateUnknown()
			}
			s.observePoll(string(source.name), "failed")
			failures = append(failures, err)
		}
		timer.Finish(ctx, outcome)
	}
	return errors.Join(failures...)
}

func (s *Solver) reconcileExclusivePoll(ctx context.Context, now time.Time) error {
	var err error
	if s.exclusiveStateUnknown.Load() {
		err = s.recoverRecentExclusive(ctx, now)
	}
	if err == nil {
		err = s.sweepExclusive(ctx, now)
	}
	if err != nil {
		return errors.Errorf("reconcile exclusive orders: %w", err)
	}
	return nil
}

func (s *Solver) recoverRecentExclusive(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-s.exclusiveRecoveryLookback())
	initial := s.lastExclusivePoll.Load() == 0
	records, readErr := s.orders.recentOrders(ctx, s.chainID, s.cfg.Executor, cutoff)
	for _, record := range records {
		if record.OrderStatus == orderStatusOpen {
			continue
		}
		obligation, err := exclusiveObligationFromEntry(record, s.cfg, s.chainID)
		if errors.Is(err, errDifferentExclusiveFiller) {
			continue
		}
		if err != nil {
			return errors.Errorf("track terminal exclusive order %q: %w", record.OrderHash, err)
		}
		obligation.recoveredAtStart = initial
		s.trackExclusiveObligation(obligation, record.QuoteID)
	}
	s.log.V(1).Info("recent exclusive history reconciled", "orders", len(records), "createdAfter", cutoff.Unix(), "startup", initial)
	if readErr != nil {
		return errors.Errorf("poll recent exclusive orders: %w", readErr)
	}
	return nil
}

func (s *Solver) exclusiveRecoveryLookback() time.Duration {
	return max(2*s.cfg.Breaker.Window, time.Hour)
}

func (s *Solver) pollSource(ctx context.Context, source orderSource, filler *common.Address, out chan<- *resolvedOrder) (time.Time, error) {
	records, readErr := s.orders.openOrders(ctx, s.chainID, filler)
	if len(records) == 0 && readErr != nil {
		return time.Time{}, errors.Errorf("poll %s orders: %w", source, readErr)
	}
	now, err := s.reader.latestBlockTime(ctx)
	if err != nil {
		return time.Time{}, errors.Errorf("read chain time for %s orders: %w", source, err)
	}
	s.cleanupOrderHistory(now)
	s.log.V(1).Info("orders polled", "source", source, "orders", len(records), "partialError", readErr != nil)
	for _, record := range records {
		order, err := s.observePolledOrder(record, source, now)
		if err != nil {
			return now, err
		}
		if order == nil || !s.claim(order.Hash, now) {
			continue
		}
		s.log.V(1).Info("order queued for fill", "source", source, "orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID, "amountIn", order.AmountIn, "amountOut", order.AmountOut, "deadline", order.Deadline)
		select {
		case out <- order:
		case <-ctx.Done():
			s.endFillPlanning()
			s.retry(order.Hash, now, false)
			return time.Time{}, ctx.Err()
		}
	}
	if readErr != nil {
		return now, errors.Errorf("poll %s orders: %w", source, readErr)
	}
	return now, nil
}

// A rejected executable order can still impose an exclusive obligation. Parsing
// failure must never erase that obligation or reopen quoting after a bad snapshot.
func (s *Solver) observePolledOrder(record orderEntry, source orderSource, now time.Time) (*resolvedOrder, error) {
	order, parseErr := parseAndResolveOrder(record, source, s.cfg, s.chainID, now)
	if parseErr == nil {
		if s.trackExclusive(order) {
			s.observeExclusiveWin()
		}
		return order, nil
	}
	if source == orderSourceExclusiveV2 {
		obligation, err := exclusiveObligationFromEntry(record, s.cfg, s.chainID)
		if err != nil {
			return nil, errors.Errorf("rejected exclusive order %q cannot be tracked: parse: %v; obligation: %w", record.OrderHash, parseErr, err)
		}
		obligation.liveObserved = true
		if s.trackExclusiveObligation(obligation, record.QuoteID) {
			s.observeExclusiveWin()
		}
	}
	s.log.V(1).Info("order rejected", "error", parseErr, "source", source, "orderHash", record.OrderHash, "quoteId", record.QuoteID)
	return nil, nil
}

func (s *Solver) recordExclusivePollSuccess(now time.Time) {
	changed := s.exclusiveStateUnknown.Swap(false)
	s.lastExclusivePoll.Store(now.Unix())
	if changed {
		s.requestQuoteRefresh()
	}
}
