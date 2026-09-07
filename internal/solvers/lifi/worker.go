package lifi

import (
	"context"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// orderWorker alone owns pending fills and deferred orders. Result waiters only
// deliver immutable completions; they cannot release capacity or renew quotes.
type orderWorker struct {
	solver      *Solver
	routes      []route
	pending     map[string]bool
	completions chan fillCompletion
	retries     *orderRetries
	generation  uint64
	barrier     chan struct{}
	onRetryable func(*submittedOrder, int)
	now         func() time.Time
}

func (s *Solver) runOrderWorker(ctx context.Context, routes []route, orders <-chan *submittedOrder,
	onRetryable func(*submittedOrder, int), inputDrained chan<- struct{},
) error {
	w := &orderWorker{
		solver: s, routes: routes, pending: make(map[string]bool),
		completions: make(chan fillCompletion, fillCompletionCapacity),
		retries:     newOrderRetries(orderRetryCapacity, orderDepositRetryCapacity),
		onRetryable: onRetryable, now: s.wallNow,
	}
	if w.now == nil {
		w.now = time.Now
	}
	stopCapacityMetrics := s.metrics.trackOrderQueue(orderQueueCapacityRetry, func() orderQueueSnapshot { return w.retries.snapshot().capacity })
	defer stopCapacityMetrics()
	stopDepositMetrics := s.metrics.trackOrderQueue(orderQueueDepositRetry, func() orderQueueSnapshot { return w.retries.snapshot().deposit })
	defer stopDepositMetrics()
	return w.run(ctx, orders, inputDrained)
}

func (w *orderWorker) releaseBarrier() {
	if w.barrier != nil && len(w.retries.capacity) == 0 {
		close(w.barrier)
		w.barrier = nil
	}
}

func (w *orderWorker) run(ctx context.Context, orders <-chan *submittedOrder, drained chan<- struct{}) error {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	done := ctx.Done()
	var runErr error
	for {
		if runErr == nil && ctx.Err() != nil {
			runErr, done, orders = ctx.Err(), nil, nil
			w.retries.clearCapacity()
			w.retries.clearDeposits()
			w.releaseBarrier()
		}
		if orders == nil && len(w.pending) == 0 && len(w.retries.capacity) == 0 && len(w.retries.deposits) == 0 {
			return runErr
		}
		input := orders
		// A barrier acknowledges only the generation that precedes it. Later input
		// cannot add new capacity retries before that generation has drained.
		if w.barrier != nil {
			input = nil
		}
		var retry <-chan time.Time
		timer.Stop()
		if at, ok := w.retries.nextDepositAt(); ok {
			timer.Reset(max(at.Sub(w.now()), 0))
			retry = timer.C
		}
		select {
		case <-done:
			// The next iteration stops intake, then waits only for admitted results.
		case completion := <-w.completions:
			w.complete(ctx, completion)
		case <-retry:
			order, err := w.retries.popDeposit(w.now())
			if err != nil {
				w.depositExpired(order, err)
			} else if order != nil {
				w.process(ctx, order, nil)
			}
		case order, ok := <-input:
			if !ok {
				orders = nil
				w.retries.clearDeposits()
				w.releaseBarrier()
				if drained != nil {
					close(drained)
					drained = nil
				}
			} else if ctx.Err() == nil {
				switch {
				case order.processed != nil:
					w.barrier = order.processed
					w.releaseBarrier()
				case w.retries.deposits[orderInboxKey(order)] != nil:
					w.solver.log.V(1).Info("order feed replay coalesced while awaiting on-chain deposit", "orderId", order.OrderID)
				default:
					w.process(ctx, order, nil)
				}
			}
		}
	}
}

func (w *orderWorker) process(ctx context.Context, order *submittedOrder, reservations *liquidlane.CapacityReservations) {
	result := w.solver.processOrderUsingReservations(ctx, w.routes, order, w.pending, reservations)
	outcome := w.retain(order, result)
	w.solver.metrics.observeOrderProcessing(outcome)
	w.releaseBarrier()
}

func (w *orderWorker) retain(order *submittedOrder, result orderProcessingResult) orderProcessingOutcome {
	if result.depositNotVisible {
		if err := w.retries.scheduleDeposit(order, w.now()); err != nil {
			w.solver.metrics.observeOrderQueueDrop(orderQueueDepositRetry, err)
			if errors.Is(err, errOrderDepositRetryFull) || errors.Is(err, errOrderDepositRetryKey) {
				w.solver.log.Error(err, "order deposit retry: dropped order", "orderId", order.OrderID)
			} else {
				w.solver.log.Info("order skipped: deposit did not become visible within retry bounds", "orderId", order.OrderID, "reason", err.Error())
			}
			return orderProcessingNotActionable
		}
		return result.outcome
	}
	w.retries.finishDeposit(order)
	if result.fill != nil {
		w.pending[result.fill.reservationKey] = true
		go awaitFill(result.fill, w.completions)
		return result.outcome
	}
	if result.retryable && w.onRetryable != nil {
		w.onRetryable(order, result.recoveryAttemptLimit)
	}
	if len(result.blockedOn) == 0 {
		return result.outcome
	}
	// Only a pending completion advances this generation; retaining a capacity
	// retry without any pending fill would leave the worker waiting forever.
	if len(w.pending) == 0 {
		return orderProcessingOther
	}
	if err := w.retries.enqueueCapacity(order, w.generation); err != nil {
		w.solver.metrics.observeOrderQueueDrop(orderQueueCapacityRetry, err)
		w.solver.log.Error(err, "order retry queue: dropped newest order", "orderId", order.OrderID)
		if errors.Is(err, errOrderRetryFull) {
			return orderProcessingCapacityDropped
		}
	}
	return result.outcome
}

func (w *orderWorker) depositExpired(order *submittedOrder, err error) {
	w.solver.metrics.observeOrderProcessing(orderProcessingNotActionable)
	w.solver.metrics.observeOrderQueueDrop(orderQueueDepositRetry, err)
	w.solver.log.Info("order skipped: deposit did not become visible within retry bounds", "orderId", order.OrderID, "reason", err.Error())
}

func (w *orderWorker) complete(ctx context.Context, completion fillCompletion) {
	w.solver.completeFill(w.pending, completion)
	w.generation++
	for ctx.Err() == nil {
		order := w.retries.popCapacity(w.generation)
		if order == nil {
			break
		}
		// Retry against the released capacity before advertising it. Other pending
		// reservations remain in the snapshot and cannot be double allocated.
		reservations := w.solver.capacity.SnapshotExcluding(completion.fill.reservationKey)
		w.process(ctx, order, &reservations)
	}
	if ctx.Err() != nil {
		w.retries.clearCapacity()
	}
	if w.solver.capacity.Delete(completion.fill.reservationKey) {
		w.solver.requestQuoteRefresh()
	}
	w.releaseBarrier()
}
