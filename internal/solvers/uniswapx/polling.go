package uniswapx

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/observability"
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
	exclusiveTimer := observability.StartOperation(s.metrics.operation(exclusiveOrderPollOperation))
	now, exclusiveErr := s.pollSource(ctx, orderSourceExclusiveV2, &s.cfg.Executor, out)
	if exclusiveErr == nil {
		exclusiveErr = s.reconcileExclusivePoll(ctx, now)
	}
	if exclusiveErr != nil {
		s.markExclusiveStateUnknown()
		s.metrics.observePoll(string(orderSourceExclusiveV2), "failed")
		pollErrs = append(pollErrs, exclusiveErr)
	} else {
		s.recordExclusivePollSuccess(time.Now())
		s.metrics.observePoll(string(orderSourceExclusiveV2), "ok")
	}
	exclusiveTimer.Finish(ctx, observability.OutcomeForError(exclusiveErr))
	if s.cfg.OrderServer.PublicV2 {
		publicTimer := observability.StartOperation(s.metrics.operation(publicOrderPollOperation))
		_, publicErr := s.pollSource(ctx, orderSourcePublicV2, nil, out)
		if publicErr != nil {
			pollErrs = append(pollErrs, publicErr)
			s.metrics.observePoll(string(orderSourcePublicV2), "failed")
		} else {
			s.metrics.observePoll(string(orderSourcePublicV2), "ok")
		}
		publicTimer.Finish(ctx, observability.OutcomeForError(publicErr))
	}
	return errors.Join(pollErrs...)
}

func (s *Solver) reconcileExclusivePoll(ctx context.Context, now time.Time) error {
	if s.breaker.exclusiveUnknown.Load() {
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
	startup := s.breaker.lastExclusivePoll.Load() == 0
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
	wasUnknown := s.breaker.exclusiveUnknown.Swap(false)
	timestamp := now.Unix()
	s.breaker.lastExclusivePoll.Store(timestamp)
	s.metrics.exclusivePollSucceeded(now)
	if wasUnknown {
		s.requestQuoteRefresh()
	}
}

func (s *Solver) claim(hash common.Hash, now time.Time) bool {
	if !s.ledger.claim(hash, now) {
		return false
	}
	s.beginFillPlanning()
	return true
}

func (s *Solver) retry(hash common.Hash, now time.Time, failed bool) {
	order := s.ledger.retry(hash, now, s.cfg.OrderServer.PollInterval, failed)
	s.log.V(1).Info(
		"order retry scheduled",
		"orderHash", hash.Hex(),
		"failed", failed,
		"attempt", order.attempts,
		"backoff", order.retryAt.Sub(now),
		"retryAt", order.retryAt.Unix(),
	)
}
