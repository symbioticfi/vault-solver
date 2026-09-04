package lifi

import (
	"context"
	"time"

	"github.com/go-errors/errors"
)

// orderWorker owns mutable fill, retry, and barrier state for one worker goroutine.
type orderWorker struct {
	solver       *Solver
	ctx          context.Context
	routes       []route
	orders       *orderBook
	inputDrained chan<- struct{}

	recoveryBarrier chan struct{}
	retryNow        func() time.Time
}

func (solver *Solver) newOrderWorker(
	ctx context.Context,
	routes []route,
	orders *orderBook,
	inputDrained chan<- struct{},
) *orderWorker {
	retryNow := solver.wallNow
	if retryNow == nil {
		retryNow = time.Now
	}
	return &orderWorker{
		solver:       solver,
		ctx:          ctx,
		routes:       routes,
		orders:       orders,
		inputDrained: inputDrained,
		retryNow:     retryNow,
	}
}

func (w *orderWorker) run() error {
	retry := time.NewTicker(initialOrderDepositRetryBackoff)
	defer retry.Stop()
	for {
		input := (<-chan *submittedOrder)(w.orders.orders)
		if w.recoveryBarrier != nil {
			input = nil
		}
		select {
		case <-w.ctx.Done():
			w.orders.clear()
			w.solver.metrics.queues(orderQueueMetrics{})
			return w.ctx.Err()
		case <-retry.C:
			w.retryDeposit()
			w.retryCapacity()
			w.solver.metrics.queues(w.orders.metricsSnapshot())
		case order, ok := <-input:
			if !ok {
				w.finishInput()
				return nil
			}
			w.orders.accepted(order)
			w.acceptOrder(order)
		}
	}
}

func (w *orderWorker) retryDeposit() {
	order, err := w.orders.popDepositReady(w.retryNow())
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
		w.process(order)
	}
}

func (w *orderWorker) retryCapacity() {
	if order := w.orders.popCapacity(); order != nil {
		w.process(order)
	}
}

func (w *orderWorker) acceptOrder(order *submittedOrder) {
	if order == nil {
		return
	}
	if w.ctx.Err() != nil {
		return
	}
	if order.processed != nil {
		w.recoveryBarrier = order.processed
		w.releaseRecoveryBarrier()
		return
	}
	if w.orders.containsDeposit(order) {
		w.solver.log.V(1).Info("order feed replay coalesced while awaiting on-chain deposit",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
		)
		return
	}
	w.process(order)
}

func (w *orderWorker) finishInput() {
	w.orders.clear()
	w.solver.metrics.queues(orderQueueMetrics{})
	w.releaseRecoveryBarrier()
	if w.inputDrained != nil {
		close(w.inputDrained)
		w.inputDrained = nil
	}
}

func (w *orderWorker) process(order *submittedOrder) {
	defer w.releaseRecoveryBarrier()
	action := w.solver.processOrder(w.ctx, w.routes, order)
	if action == orderWaitDeposit {
		w.deferForDeposit(order)
		return
	}
	w.orders.finishDeposit(order)
	switch action {
	case orderDone, orderWaitDeposit:
	case orderRetry:
		w.orders.markRecoveryRetry(order, 0)
	case orderRetryStrategy:
		w.orders.markRecoveryRetry(order, maximumStrategyRecoveryAttempts)
	case orderWaitCapacity:
		w.solver.metrics.order("deferred")
		if err := w.deferUntilCapacity(order); err != nil {
			w.solver.metrics.queueDrop("capacity_retry")
			w.solver.log.Error(err, "order retry queue: dropped newest order",
				"orderId", order.OrderID,
				"onChainOrderId", order.OnChainOrderID,
				"quoteId", order.QuoteID,
				"capacity", orderRetryCapacity,
			)
		}
	}
}

func (w *orderWorker) deferForDeposit(order *submittedOrder) {
	w.solver.metrics.order("deferred")
	err := w.orders.scheduleDeposit(order, w.retryNow())
	if err == nil {
		return
	}
	fields := []any{
		"orderId", order.OrderID,
		"onChainOrderId", order.OnChainOrderID,
		"quoteId", order.QuoteID,
	}
	if errors.Is(err, errOrderDepositRetryFull) || errors.Is(err, errOrderDepositRetryKey) {
		w.solver.metrics.queueDrop("deposit_retry")
		w.solver.log.Error(err, "order deposit retry: dropped order", append(fields, "capacity", orderDepositRetryCapacity)...)
		return
	}
	w.solver.log.Info("order skipped: deposit did not become visible within retry bounds",
		append(fields, "reason", err.Error())...)
}

func (w *orderWorker) deferUntilCapacity(order *submittedOrder) error {
	queued, err := w.orders.enqueueCapacity(order)
	if err != nil {
		return err
	}
	if queued {
		_, retryCount := w.orders.retryCounts()
		w.solver.log.V(1).Info(
			"order fill deferred by pending capacity",
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"pendingFills", w.solver.capacity.Len(),
			"retryQueue", retryCount,
		)
	}
	return nil
}

func (w *orderWorker) releaseRecoveryBarrier() {
	depositRetries, capacityRetries := w.orders.retryCounts()
	if w.recoveryBarrier == nil || capacityRetries > 0 || depositRetries > 0 {
		return
	}
	close(w.recoveryBarrier)
	w.recoveryBarrier = nil
}
