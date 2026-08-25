package lifi

import (
	"context"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// orderWorker owns mutable fill, retry, and barrier state for one worker goroutine.
type orderWorker struct {
	solver       *Solver
	ctx          context.Context
	routes       []route
	orders       <-chan *submittedOrder
	onRetryable  func(*submittedOrder, int)
	inputDrained chan<- struct{}

	pending               map[string]*pendingFill
	completions           chan fillCompletion
	retries               *reservationRetryQueue
	depositRetries        *orderDepositRetryQueue
	reservationReleaseGen uint64
	ctxDone               <-chan struct{}
	runErr                error
	recoveryBarrier       chan struct{}
	retryNow              func() time.Time
	depositRetryTimer     *time.Timer
}

func (solver *Solver) newOrderWorker(
	ctx context.Context,
	routes []route,
	orders <-chan *submittedOrder,
	onRetryable func(*submittedOrder, int),
	inputDrained chan<- struct{},
) *orderWorker {
	retryNow := solver.wallNow
	if retryNow == nil {
		retryNow = time.Now
	}
	depositRetryTimer := time.NewTimer(maximumOrderDepositRetryWindow)
	depositRetryTimer.Stop()
	return &orderWorker{
		solver:            solver,
		ctx:               ctx,
		routes:            routes,
		orders:            orders,
		onRetryable:       onRetryable,
		inputDrained:      inputDrained,
		pending:           make(map[string]*pendingFill),
		completions:       make(chan fillCompletion, fillCompletionCapacity),
		retries:           newReservationRetryQueue(orderRetryCapacity),
		depositRetries:    newOrderDepositRetryQueue(orderDepositRetryCapacity),
		ctxDone:           ctx.Done(),
		retryNow:          retryNow,
		depositRetryTimer: depositRetryTimer,
	}
}

func (w *orderWorker) run() error {
	defer w.depositRetryTimer.Stop()
	for w.active() {
		if w.runErr == nil && w.ctx.Err() != nil {
			w.beginShutdown(w.ctx.Err())
		}
		if w.runErr != nil && len(w.pending) == 0 {
			return w.runErr
		}
		orderInput := w.orders
		// A recovery barrier prevents later generations from extending its protected retry set.
		if w.runErr != nil || w.recoveryBarrier != nil {
			orderInput = nil
		}
		select {
		case <-w.ctxDone:
			w.beginShutdown(w.ctx.Err())
		case completion := <-w.completions:
			w.complete(completion)
		case <-w.depositRetryChannel():
			w.retryDeposit()
		case order, ok := <-orderInput:
			w.acceptOrder(order, ok)
		}
	}
	return w.runErr
}

func (w *orderWorker) active() bool {
	return w.orders != nil || len(w.pending) > 0 || w.retries.len() > 0 || w.depositRetries.len() > 0
}

func (w *orderWorker) beginShutdown(err error) {
	w.runErr = err
	w.ctxDone = nil
	w.orders = nil
	w.retries.clear()
	w.depositRetries.clear()
}

func (w *orderWorker) depositRetryChannel() <-chan time.Time {
	w.depositRetryTimer.Stop()
	readyAt, ok := w.depositRetries.nextReadyAt()
	if !ok {
		return nil
	}
	w.depositRetryTimer.Reset(max(readyAt.Sub(w.retryNow()), 0))
	return w.depositRetryTimer.C
}

func (w *orderWorker) retryDeposit() {
	order, err := w.depositRetries.popReady(w.retryNow())
	if err != nil {
		w.solver.log.Info("order skipped: deposit did not become visible within retry bounds",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"reason", err.Error(),
		)
		return
	}
	if order != nil {
		w.process(order, nil)
	}
}

func (w *orderWorker) acceptOrder(order *submittedOrder, ok bool) {
	if !ok {
		w.orders = nil
		w.depositRetries.clear()
		w.releaseRecoveryBarrier()
		if w.inputDrained != nil {
			close(w.inputDrained)
			w.inputDrained = nil
		}
		return
	}
	if w.ctx.Err() != nil {
		w.beginShutdown(w.ctx.Err())
		return
	}
	if order.processed != nil {
		w.recoveryBarrier = order.processed
		w.releaseRecoveryBarrier()
		return
	}
	if w.depositRetries.contains(order) {
		w.solver.log.V(1).Info("order feed replay coalesced while awaiting on-chain deposit",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
		)
		return
	}
	w.process(order, nil)
}

func (w *orderWorker) process(order *submittedOrder, reservations *liquidlane.CapacityReservations) {
	defer w.releaseRecoveryBarrier()

	var result orderProcessingResult
	if reservations == nil {
		result = w.solver.processOrderWithPending(w.ctx, w.routes, order, w.pending)
	} else {
		result = w.solver.processOrderUsingReservations(w.ctx, w.routes, order, w.pending, reservations)
	}
	if result.depositNotVisible {
		if err := w.depositRetries.schedule(order, w.retryNow()); err != nil {
			if errors.Is(err, errOrderDepositRetryFull) || errors.Is(err, errOrderDepositRetryKey) {
				w.solver.log.Error(err, "order deposit retry: dropped order",
					"orderId", order.OrderID,
					"onChainOrderId", order.OnChainOrderID,
					"quoteId", order.QuoteID,
					"capacity", orderDepositRetryCapacity,
				)
			} else {
				w.solver.log.Info("order skipped: deposit did not become visible within retry bounds",
					"orderId", order.OrderID,
					"onChainOrderId", order.OnChainOrderID,
					"quoteId", order.QuoteID,
					"reason", err.Error(),
				)
			}
		}
		return
	}
	w.depositRetries.finish(order)
	if result.fill != nil {
		w.pending[result.fill.reservationKey] = result.fill
		go awaitFill(result.fill, w.completions)
		return
	}
	if result.retryable && w.onRetryable != nil {
		w.onRetryable(order, result.recoveryAttemptLimit)
	}
	// A reservation retry is released only by a pending fill completion advancing the generation.
	if len(result.blockedOn) == 0 || len(w.pending) == 0 {
		return
	}
	queuedBefore := w.retries.len()
	if err := w.retries.enqueue(order, w.reservationReleaseGen); err != nil {
		w.solver.log.Error(err, "order retry queue: dropped newest order",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"capacity", orderRetryCapacity,
		)
	} else if w.retries.len() > queuedBefore {
		w.solver.log.V(1).Info(
			"order fill deferred by pending capacity",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"blockedCapacityGroups", len(result.blockedOn),
			"pendingFills", len(w.pending),
			"retryQueue", w.retries.len(),
		)
	}
}

func (w *orderWorker) complete(completion fillCompletion) {
	w.solver.completeFill(w.pending, completion)
	w.reservationReleaseGen++
	for w.ctx.Err() == nil {
		order := w.retries.popReady(w.reservationReleaseGen)
		if order == nil {
			break
		}
		w.solver.log.V(1).Info(
			"order fill retry started",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"pendingFills", len(w.pending),
			"retryQueue", w.retries.len(),
		)
		reservations := w.solver.capacity.SnapshotExcluding(completion.fill.reservationKey)
		w.process(order, &reservations)
	}
	if w.ctx.Err() != nil {
		w.retries.clear()
	}
	if w.solver.capacity.Delete(completion.fill.reservationKey) {
		w.solver.log.V(1).Info(
			"fill capacity released",
			"orderId", completion.fill.order.OrderID,
			"onChainOrderId", completion.fill.orderID.Hex(),
			"quoteId", completion.fill.order.QuoteID,
			"pendingFills", w.solver.capacity.Len(),
		)
		w.solver.requestQuoteRefresh()
	}
	w.releaseRecoveryBarrier()
}

func (w *orderWorker) releaseRecoveryBarrier() {
	if w.recoveryBarrier == nil || w.retries.len() > 0 {
		return
	}
	close(w.recoveryBarrier)
	w.recoveryBarrier = nil
}
