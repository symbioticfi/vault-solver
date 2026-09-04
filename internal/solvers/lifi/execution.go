package lifi

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	orderBookCapacity          = 4_096
	orderRetryCapacity         = orderBookCapacity
	orderDepositRetryCapacity  = 128
	maximumOrderRecoverySweeps = 8
	// Strategy failures are often deterministic for one input. Two retries preserve a
	// short transient window without allowing one order to hold readiness forever.
	maximumStrategyRecoveryAttempts = 3
	initialOrderRecoveryBackoff     = time.Second
	maximumOrderRecoveryBackoff     = 30 * time.Second
)

var (
	errOrderBookFull   = errors.New("order book input is full")
	errOrderBookClosed = errors.New("order book input is closed")
	errOrderRetryFull  = errors.New("order retry queue is full")
)

func (s *Solver) runOrderFeed(
	ctx context.Context,
	routes []route,
	feedConnections chan<- context.Context,
) error {
	orders := newOrderBook(orderBookCapacity)
	workCtx, stopWork := context.WithCancel(context.WithoutCancel(ctx))
	defer stopWork()
	feedDone := make(chan error, 1)
	workerDone := make(chan error, 1)
	workerInputDrained := make(chan struct{})
	go func() {
		feedDone <- s.feed.run(ctx, s.recoveryHooks(orders, feedConnections), func(_ context.Context, msg orderMessage) {
			s.enqueueFeedMessage(orders, msg)
		})
	}()
	go func() {
		workerDone <- s.newOrderWorker(workCtx, routes, orders, workerInputDrained).run()
	}()

	feedErr := <-feedDone
	orders.closeInput()
	drainTimer := time.NewTimer(s.cfg.OrderServer.HTTPTimeout)
	workerFinished := false
	var workerErr error
	select {
	case <-workerInputDrained:
		_ = drainTimer.Stop()
	case workerErr = <-workerDone:
		workerFinished = true
		_ = drainTimer.Stop()
	case <-drainTimer.C:
		s.log.Info("order book drain timed out", "timeout", s.cfg.OrderServer.HTTPTimeout.String())
		stopWork()
	}
	if !workerFinished {
		workerErr = <-workerDone
	}
	return preferLifecycleError(feedErr, workerErr)
}

func (s *Solver) recoveryHooks(
	orders *orderBook,
	connected chan<- context.Context,
) orderFeedConnectionHooks {
	return orderFeedConnectionHooks{
		beforeRead: func(context.Context) {
			s.metrics.feedState(true, false)
			orders.beginRecovery()
			s.log.V(1).Info("order recovery started", "executor", s.cfg.Executor.Hex())
		},
		whileConnected: func(connectionCtx context.Context) {
			defer s.metrics.feedState(false, false)
			defer orders.endRecovery()
			timer := observability.StartOperation(s.metrics.operation(orderRecoveryOperation))
			recovered := s.recoverOrdersUntilSuccess(connectionCtx, orders)
			outcome := observability.ExternalOperationSuccess
			if !recovered {
				outcome = observability.ExternalOperationError
			}
			timer.Finish(connectionCtx, outcome)
			if !recovered {
				return
			}
			s.metrics.feedState(true, true)
			select {
			case connected <- connectionCtx:
			case <-connectionCtx.Done():
			}
		},
	}
}

func (s *Solver) enqueueFeedMessage(orders *orderBook, message orderMessage) {
	order := s.parseOrderMessage(message)
	if err := orders.enqueue(order); err != nil {
		if errors.Is(err, errOrderBookFull) {
			s.metrics.queueDrop("inbox")
		}
		fields := []any{"event", message.Event}
		if order != nil {
			fields = append(fields,
				"orderId", order.OrderID,
				"onChainOrderId", order.OnChainOrderID,
				"quoteId", order.QuoteID,
			)
		}
		s.log.Error(err, "order feed: dropped order", fields...)
	}
}

func (s *Solver) parseOrderMessage(msg orderMessage) *submittedOrder {
	order, err := parseSubmittedOrder(msg.Data, s.cfg, s.chainID)
	if err != nil {
		if errors.Is(err, errOrderForDifferentChain) {
			s.log.Info("order feed: ignored order for another chain", "event", msg.Event, "reason", err.Error())
			return nil
		}
		if errors.Is(err, errOrderUnsupported) {
			s.log.V(1).Info("order feed: ignored unsupported order", "event", msg.Event, "reason", err.Error())
			return nil
		}
		s.log.Error(err, "order feed: ignored order", "event", msg.Event)
		return nil
	}
	if isDutchAuctionContext(order.Output.Context) {
		s.log.Info("order feed: ignored unsupported Dutch auction",
			"event", msg.Event,
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"contextType", hexutil.Encode(order.Output.Context[:1]),
		)
		return nil
	}
	s.log.Info("order received",
		"event", msg.Event,
		"orderStatus", order.OrderStatus,
		"orderId", order.OrderID,
		"onChainOrderId", order.OnChainOrderID,
		"quoteId", order.QuoteID,
		"inputSettler", s.cfg.InputSettler.Hex(),
		"tokenIn", order.TokenIn.Hex(),
		"tokenOut", order.TokenOut.Hex(),
		"amountIn", bigString(order.AmountIn),
		"requiredAmountOut", bigString(order.OutputAmount),
		"expires", order.Order.Expires,
		"fillDeadline", order.Order.FillDeadline,
	)
	return order
}
