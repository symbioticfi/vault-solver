package uniswapx

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
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
		timer := observability.StartOperation(s.operations.exclusiveOrderPoll)
		outcome := observability.ExternalOperationError
		now, err := s.pollSource(ctx, orderSourceExclusiveV2, &s.cfg.Executor, out)
		if err != nil {
			if !now.IsZero() {
				outcome = observability.ExternalOperationDegraded
			}
			s.markExclusiveStateUnknown()
			s.observePoll(string(orderSourceExclusiveV2), "failed")
			pollErrs = append(pollErrs, err)
		} else if err := s.reconcileExclusivePoll(ctx, now); err != nil {
			s.markExclusiveStateUnknown()
			s.observePoll(string(orderSourceExclusiveV2), "failed")
			pollErrs = append(pollErrs, err)
		} else {
			outcome = observability.ExternalOperationSuccess
			s.recordExclusivePollSuccess(time.Now())
			s.observePoll(string(orderSourceExclusiveV2), "ok")
		}
		timer.Finish(ctx, outcome)
	}
	if s.cfg.OrderServer.Sources.PublicV2 {
		timer := observability.StartOperation(s.operations.publicOrderPoll)
		outcome := observability.ExternalOperationError
		now, err := s.pollSource(ctx, orderSourcePublicV2, nil, out)
		if err != nil {
			if !now.IsZero() {
				outcome = observability.ExternalOperationDegraded
			}
			pollErrs = append(pollErrs, err)
			s.observePoll(string(orderSourcePublicV2), "failed")
		} else {
			outcome = observability.ExternalOperationSuccess
			s.observePoll(string(orderSourcePublicV2), "ok")
		}
		timer.Finish(ctx, outcome)
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
	if err != nil {
		return errors.Errorf("poll recent exclusive orders: %w", err)
	}
	return nil
}

func (s *Solver) exclusiveRecoveryLookback() time.Duration {
	return max(time.Hour, 2*s.cfg.Breaker.Window)
}

// pollSource roots one uniswapx.orders.poll trace per poll and spans each accepted order under it.
func (s *Solver) pollSource(
	ctx context.Context,
	source orderSource,
	filler *common.Address,
	out chan<- *resolvedOrder,
) (now time.Time, err error) {
	ctx, end := tracer.Start(ctx, "uniswapx.orders.poll")
	defer func() { end(err) }()

	// Derived from the base logger, never from a caller's already-derived one: TraceLogger appends
	// trace_id/span_id unconditionally, so re-deriving would stamp them twice per line.
	log := observability.TraceLogger(ctx, s.log)
	entries, err := s.orders.openOrders(ctx, s.chainID, filler)
	if err != nil && len(entries) == 0 {
		return time.Time{}, errors.Errorf("poll %s orders: %w", source, err)
	}
	log.V(1).Info(
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
			log.V(1).Info("order rejected", "error", parseErr, "source", source,
				"orderHash", entry.OrderHash, "quoteId", entry.QuoteID)
			continue
		}
		if s.trackExclusive(order, now) {
			s.observeExclusiveWin()
		}
		if !s.claim(order.Hash, now) {
			log.V(1).Info(
				"order skipped: already handled or awaiting retry",
				"source", source,
				"orderHash", order.Hash.Hex(),
				"quoteId", order.QuoteID,
			)
			continue
		}
		if trackErr := s.trackOrder(ctx, order, out); trackErr != nil {
			s.endFillPlanning()
			s.retry(order.Hash, now, false)
			return time.Time{}, trackErr
		}
	}
	if err != nil {
		return now, errors.Errorf("poll %s orders: %w", source, err)
	}
	return now, nil
}

// trackOrder spans an accepted order from claim to enqueue, links it back to the quote that won it
// (spec §12), and rides its span context on the order so the fill continues this trace.
func (s *Solver) trackOrder(ctx context.Context, order *resolvedOrder, out chan<- *resolvedOrder) (err error) {
	attrs := []attribute.KeyValue{
		observability.AttrOrderHash.String(order.Hash.Hex()),
		observability.AttrQuoteID.String(order.QuoteID),
	}
	var links []trace.Link
	if link, ok := s.quoteLink(order.QuoteID); ok {
		links = append(links, link)
		attrs = append(attrs, observability.AttrQuoteTraceID.String(link.SpanContext.TraceID().String()))
	}
	ctx, end := tracer.StartLinked(ctx, "uniswapx.order.track", links, attrs...)
	defer func() { end(err) }()

	log := observability.TraceLogger(ctx, s.log)
	if len(links) > 0 {
		log = log.WithValues("quoteTraceId", links[0].SpanContext.TraceID().String())
	} else {
		// Best effort (spec §12): the quote span is gone — restart, eviction, or it was never ours.
		// The fill proceeds identically; only the link is lost.
		trace.SpanFromContext(ctx).AddEvent("link_miss",
			trace.WithAttributes(attribute.String("key", order.QuoteID)))
	}
	order.span = trace.SpanContextFromContext(ctx)
	log.V(1).Info(
		"order queued for fill",
		"source", order.Source,
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
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Solver) recordExclusivePollSuccess(now time.Time) {
	wasUnknown := s.exclusiveStateUnknown.Swap(false)
	timestamp := now.Unix()
	s.lastExclusivePoll.Store(timestamp)
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
