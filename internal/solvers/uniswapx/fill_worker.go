package uniswapx

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type pendingUniswapFill struct {
	order  *resolvedOrder
	result <-chan txmanager.Result
}

type uniswapFillCompletion struct {
	fill   *pendingUniswapFill
	result txmanager.Result
}

// fillWorker owns pending-fill and shutdown state on its run goroutine.
type fillWorker struct {
	solver *Solver
	ctx    context.Context
	routes []liquidlane.Route
	orders <-chan *resolvedOrder

	completions chan uniswapFillCompletion
	pending     map[common.Hash]*pendingUniswapFill
	ctxDone     <-chan struct{}
	shutdownErr error
}

func (s *Solver) newFillWorker(
	ctx context.Context,
	routes []liquidlane.Route,
	orders <-chan *resolvedOrder,
) *fillWorker {
	return &fillWorker{
		solver:      s,
		ctx:         ctx,
		routes:      routes,
		orders:      orders,
		completions: make(chan uniswapFillCompletion, orderQueueCapacity),
		pending:     make(map[common.Hash]*pendingUniswapFill),
		ctxDone:     ctx.Done(),
	}
}

func (w *fillWorker) run() error {
	for w.orders != nil || len(w.pending) > 0 {
		select {
		case <-w.ctxDone:
			w.beginShutdown(w.ctx.Err())
		case completion := <-w.completions:
			w.complete(completion)
		case order, ok := <-w.orders:
			w.accept(order, ok)
		}
	}
	return w.shutdownErr
}

func (w *fillWorker) beginShutdown(err error) {
	w.shutdownErr = err
	w.ctxDone = nil
}

func (w *fillWorker) complete(completion uniswapFillCompletion) {
	delete(w.pending, completion.fill.order.Hash)
	w.solver.completePendingFill(completion)
}

func (w *fillWorker) accept(order *resolvedOrder, ok bool) {
	if !ok {
		w.orders = nil
		return
	}
	if w.shutdownErr != nil || w.ctx.Err() != nil {
		if w.shutdownErr == nil {
			w.beginShutdown(w.ctx.Err())
		}
		w.solver.endFillPlanning()
		w.solver.retry(order.Hash, time.Now(), false)
		return
	}
	if !w.solver.txm.Available() {
		w.solver.endFillPlanning()
		w.solver.retry(order.Hash, time.Now(), false)
		w.solver.log.V(1).Info(
			"order fill deferred while transaction nonce lane is paused",
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
		)
		return
	}
	w.solver.log.V(1).Info(
		"order fill planning started",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
	)
	chainObservedAt := time.Now()
	now, err := w.solver.reader.latestBlockTime(w.ctx)
	if err != nil {
		w.solver.endFillPlanning()
		w.solver.retry(order.Hash, time.Now(), false)
		w.solver.log.Error(err, "order fill: read current chain time", "orderHash", order.Hash.Hex())
		return
	}
	fill, err := w.solver.startFill(w.ctx, w.routes, order, now, chainObservedAt)
	w.solver.endFillPlanning()
	if err != nil {
		w.solver.retry(order.Hash, now, errors.Is(err, errFillPreflight))
		if errors.Is(err, errFillPreflight) {
			w.solver.recordOrderFillFailure(order, now)
		}
		if errors.Is(err, errOrderNotFillable) {
			w.solver.log.V(1).Info("order not fillable yet", "source", order.Source,
				"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
			return
		}
		w.solver.log.Error(err, "order fill preparation failed", "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
		return
	}
	w.pending[order.Hash] = fill
	go awaitUniswapFill(fill, w.completions)
}

// Once txmanager accepts a fill, shutdown may stop new admission but must not drop its terminal result.
func awaitUniswapFill(
	fill *pendingUniswapFill,
	out chan<- uniswapFillCompletion,
) {
	result, ok := <-fill.result
	if !ok {
		result.Err = errors.New("transaction result channel closed without a result")
	}
	out <- uniswapFillCompletion{fill: fill, result: result}
}
