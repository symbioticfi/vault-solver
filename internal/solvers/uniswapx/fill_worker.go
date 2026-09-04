package uniswapx

import (
	"context"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// fillWorker is the single order consumer. Transaction submission is deliberately synchronous:
// txmanager already serializes the process nonce lane, while the poller continues buffering orders.
type fillWorker struct {
	solver *Solver
	ctx    context.Context
	routes []liquidlane.Route
	orders <-chan *resolvedOrder
	now    func() time.Time
}

func (w *fillWorker) run() error {
	for {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case order, ok := <-w.orders:
			if !ok {
				return nil
			}
			w.fill(order)
		}
	}
}

func (w *fillWorker) fill(order *resolvedOrder) {
	defer w.solver.endFillPlanning()
	if !w.solver.txm.Available() {
		w.solver.retry(order.Hash, w.now(), false)
		return
	}
	observedAt := w.now()
	chainTime, err := w.solver.reader.latestBlockTime(w.ctx)
	retryAt := observedAt
	if err == nil {
		retryAt = chainTime
		err = w.solver.startFill(w.ctx, w.routes, order, chainTime, observedAt)
	}
	if err == nil {
		return
	}
	w.solver.retry(order.Hash, retryAt, errors.Is(err, errFillPreflight))
	if errors.Is(err, errFillPreflight) {
		w.solver.recordOrderFillFailure(order, retryAt)
	}
	if !errors.Is(err, errOrderNotFillable) {
		w.solver.log.Error(err, "order fill preparation failed", "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
	}
}
