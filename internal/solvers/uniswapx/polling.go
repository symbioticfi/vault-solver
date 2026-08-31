package uniswapx

import (
	"context"
	"maps"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

type trackedOrder struct {
	completedAt time.Time
	retryAt     time.Time
	inFlight    bool
	attempts    int
}

func (s *Solver) orderLoop(ctx context.Context, out chan<- *resolvedOrder) error {
	defer close(out)
	ticker := time.NewTicker(s.cfg.OrderServer.PollInterval)
	defer ticker.Stop()
	for {
		if err := s.pollOrders(ctx, out); err != nil && !errors.Is(err, context.Canceled) {
			s.log.Error(err, "order poll failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Solver) pollOrders(ctx context.Context, out chan<- *resolvedOrder) error {
	var pollErrs []error
	now, err := s.pollSource(ctx, orderSourceExclusiveV2, &s.cfg.Executor, out)
	if err != nil {
		s.markExclusiveStateUnknown()
		s.observePoll(string(orderSourceExclusiveV2), "failed")
		pollErrs = append(pollErrs, err)
	} else if err := s.reconcileExclusivePoll(ctx, now); err != nil {
		s.markExclusiveStateUnknown()
		s.observePoll(string(orderSourceExclusiveV2), "failed")
		pollErrs = append(pollErrs, err)
	} else {
		s.recordExclusivePollSuccess(time.Now())
		s.observePoll(string(orderSourceExclusiveV2), "ok")
	}
	if s.cfg.OrderServer.PublicV2 {
		if _, err := s.pollSource(ctx, orderSourcePublicV2, nil, out); err != nil {
			pollErrs = append(pollErrs, err)
			s.observePoll(string(orderSourcePublicV2), "failed")
		} else {
			s.observePoll(string(orderSourcePublicV2), "ok")
		}
	}
	return errors.Join(pollErrs...)
}

func (s *Solver) reconcileExclusivePoll(ctx context.Context, now time.Time) error {
	if s.exclusiveStateUnknown.Load() {
		if err := s.recoverRecentExclusive(ctx, now); err != nil {
			return err
		}
	}
	if err := s.sweepExclusive(ctx, now); err != nil {
		return errors.Errorf("reconcile exclusive orders: %w", err)
	}
	return nil
}

func (s *Solver) recoverRecentExclusive(ctx context.Context, now time.Time) error {
	startup := s.lastExclusivePoll.Load() == 0
	lookback := max(time.Hour, 2*s.cfg.Breaker.Window)
	createdAfter := now.Add(-lookback)
	entries, err := s.orders.recentOrders(ctx, s.chainID, s.cfg.Executor, createdAfter)
	if err != nil {
		return errors.Errorf("poll recent exclusive orders: %w", err)
	}
	for _, entry := range entries {
		if entry.OrderStatus == orderStatusOpen {
			continue
		}
		obligation, obligationErr := exclusiveObligationFromEntry(entry, s.cfg, s.chainID)
		if obligationErr != nil {
			if errors.Is(obligationErr, errDifferentExclusiveFiller) {
				continue
			}
			return errors.Errorf(
				"track terminal exclusive order %q: %w",
				entry.OrderHash,
				obligationErr,
			)
		}
		obligation.recoveredAtStart = startup
		s.trackExclusiveObligation(obligation, entry.QuoteID, now)
	}
	s.log.V(1).Info(
		"recent exclusive history reconciled",
		"orders", len(entries),
		"createdAfter", createdAfter.Unix(),
		"startup", startup,
	)
	return nil
}

func (s *Solver) pollSource(
	ctx context.Context,
	source orderSource,
	filler *common.Address,
	out chan<- *resolvedOrder,
) (time.Time, error) {
	entries, err := s.orders.openOrders(ctx, s.chainID, filler)
	if err != nil && len(entries) == 0 {
		return time.Time{}, errors.Errorf("poll %s orders: %w", source, err)
	}
	s.log.V(1).Info(
		"orders polled",
		"source", source,
		"orders", len(entries),
		"partialError", err != nil,
	)
	now, nowErr := s.reader.latestBlockTime(ctx)
	if nowErr != nil {
		return time.Time{}, errors.Errorf("read chain time for %s orders: %w", source, nowErr)
	}
	for _, entry := range entries {
		order, parseErr := parseAndResolveV2Order(entry, source, s.cfg, s.chainID, now)
		if parseErr != nil {
			if source == orderSourceExclusiveV2 {
				obligation, obligationErr := exclusiveObligationFromEntry(entry, s.cfg, s.chainID)
				if obligationErr != nil {
					return now, errors.Errorf(
						"rejected exclusive order %q cannot be tracked: parse: %v; obligation: %w",
						entry.OrderHash,
						parseErr,
						obligationErr,
					)
				}
				s.trackExclusiveObligation(obligation, entry.QuoteID, now)
			}
			s.log.V(1).Info("order rejected", "error", parseErr, "source", source,
				"orderHash", entry.OrderHash, "quoteId", entry.QuoteID)
			continue
		}
		s.trackExclusive(order, now)
		if !s.claim(order.Hash, now) {
			s.log.V(1).Info(
				"order skipped: already handled or awaiting retry",
				"source", source,
				"orderHash", order.Hash.Hex(),
				"quoteId", order.QuoteID,
			)
			continue
		}
		s.log.V(1).Info(
			"order queued for fill",
			"source", source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"tokenIn", order.TokenIn.Hex(),
			"tokenOut", order.TokenOut.Hex(),
			"amountIn", order.AmountIn.String(),
			"amountOut", order.AmountOut.String(),
			"deadline", order.Deadline,
		)
		select {
		case out <- order:
		case <-ctx.Done():
			s.endFillPlanning()
			s.retry(order.Hash, now, false)
			return time.Time{}, ctx.Err()
		}
	}
	if err != nil {
		return time.Time{}, errors.Errorf("poll %s orders: %w", source, err)
	}
	return now, nil
}

func (s *Solver) recordExclusivePollSuccess(now time.Time) {
	wasUnknown := s.exclusiveStateUnknown.Swap(false)
	timestamp := now.Unix()
	s.lastExclusivePoll.Store(timestamp)
	if s.metrics != nil {
		s.metrics.exclusivePoll.Set(float64(timestamp))
	}
	if wasUnknown {
		s.requestQuoteRefresh()
	}
}

func (s *Solver) claim(hash common.Hash, now time.Time) bool {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	maps.DeleteFunc(s.orderState, func(_ common.Hash, order trackedOrder) bool {
		return (!order.completedAt.IsZero() && now.Sub(order.completedAt) > time.Hour) ||
			(!order.retryAt.IsZero() && now.Sub(order.retryAt) > time.Hour)
	})
	order := s.orderState[hash]
	if !order.completedAt.IsZero() || order.inFlight || order.retryAt.After(now) {
		return false
	}
	order.retryAt = time.Time{}
	s.beginFillPlanning()
	order.inFlight = true
	s.orderState[hash] = order
	return true
}

func (s *Solver) retry(hash common.Hash, now time.Time, failed bool) {
	s.orderMu.Lock()
	order := s.orderState[hash]
	order.inFlight = false
	backoff := s.cfg.OrderServer.PollInterval
	if failed {
		order.attempts++
		backoff *= time.Duration(1 << min(order.attempts-1, 5))
		backoff = min(backoff, 30*time.Second)
	}
	order.retryAt = now.Add(backoff)
	s.orderState[hash] = order
	s.orderMu.Unlock()
	s.log.V(1).Info(
		"order retry scheduled",
		"orderHash", hash.Hex(),
		"failed", failed,
		"attempt", order.attempts,
		"backoff", backoff,
		"retryAt", order.retryAt.Unix(),
	)
}

func (s *Solver) complete(hash common.Hash, now time.Time) {
	s.orderMu.Lock()
	s.orderState[hash] = trackedOrder{completedAt: now}
	s.orderMu.Unlock()
}
