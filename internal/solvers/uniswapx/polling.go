package uniswapx

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

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
	if s.cfg.OrderServer.Sources.ExclusiveV2 {
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
	}
	if s.cfg.OrderServer.Sources.PublicV2 {
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
	lookback := s.exclusiveRecoveryLookback()
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

func (s *Solver) exclusiveRecoveryLookback() time.Duration {
	return max(time.Hour, 2*s.cfg.Breaker.Window)
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
		order, parseErr := parseAndResolveOrder(entry, source, s.cfg, s.chainID, now)
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
				obligation.liveObserved = true
				if s.trackExclusiveObligation(obligation, entry.QuoteID, now) {
					s.observeExclusiveWin()
				}
			}
			s.log.V(1).Info("order rejected", "error", parseErr, "source", source,
				"orderHash", entry.OrderHash, "quoteId", entry.QuoteID)
			continue
		}
		if s.trackExclusive(order, now) {
			s.observeExclusiveWin()
		}
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
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for key, filledAt := range s.filled {
		if now.Sub(filledAt) > time.Hour {
			delete(s.filled, key)
		}
	}
	for key, retryAt := range s.retryAt {
		if now.Sub(retryAt) > time.Hour {
			delete(s.retryAt, key)
			delete(s.attempts, key)
		}
	}
	if _, exists := s.filled[hash]; exists {
		return false
	}
	if s.inFlight[hash] {
		return false
	}
	if retryAt, exists := s.retryAt[hash]; exists && retryAt.After(now) {
		return false
	}
	delete(s.retryAt, hash)
	s.beginFillPlanning()
	s.inFlight[hash] = true
	return true
}

func (s *Solver) retry(hash common.Hash, now time.Time, failed bool) {
	s.stateMu.Lock()
	delete(s.inFlight, hash)
	backoff := s.cfg.OrderServer.PollInterval
	attempt := s.attempts[hash]
	if failed {
		attempt++
		s.attempts[hash] = attempt
		shift := min(attempt-1, 5)
		backoff *= time.Duration(1 << shift)
		backoff = min(backoff, 30*time.Second)
	}
	retryAt := now.Add(backoff)
	s.retryAt[hash] = retryAt
	s.stateMu.Unlock()
	s.log.V(1).Info(
		"order retry scheduled",
		"orderHash", hash.Hex(),
		"failed", failed,
		"attempt", attempt,
		"backoff", backoff,
		"retryAt", retryAt.Unix(),
	)
}

func (s *Solver) complete(hash common.Hash, now time.Time) {
	s.stateMu.Lock()
	delete(s.retryAt, hash)
	delete(s.inFlight, hash)
	delete(s.attempts, hash)
	s.filled[hash] = now
	s.stateMu.Unlock()
}
