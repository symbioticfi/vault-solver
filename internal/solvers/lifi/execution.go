package lifi

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const (
	fillCompletionCapacity     = 128
	orderRecoverySeenCapacity  = 4_096
	orderInboxCapacity         = orderRecoverySeenCapacity
	orderRetryCapacity         = orderInboxCapacity
	orderDepositRetryCapacity  = 128
	maximumOrderRecoverySweeps = 8
	// Strategy failures are often deterministic for one input. Two retries preserve a
	// short transient window without allowing one order to hold readiness forever.
	maximumStrategyRecoveryAttempts = 3
	initialOrderRecoveryBackoff     = time.Second
	maximumOrderRecoveryBackoff     = 30 * time.Second
)

var (
	errOrderInboxFull   = errors.New("order inbox is full")
	errOrderInboxClosed = errors.New("order inbox is closed")
	errOrderRetryFull   = errors.New("order retry queue is full")
)

type pendingFill struct {
	order          *submittedOrder
	orderID        common.Hash
	reservationKey string
	result         <-chan txmanager.Result
}

type fillCompletion struct {
	fill   *pendingFill
	result txmanager.Result
}

func (s *Solver) runOrderFeed(
	ctx context.Context,
	routes []route,
	feedConnections chan<- context.Context,
) error {
	inbox := newOrderInbox(orderInboxCapacity)
	orders := make(chan *submittedOrder)
	workCtx, stopWork := context.WithCancel(context.WithoutCancel(ctx))
	defer stopWork()
	feedDone := make(chan error, 1)
	inboxDone := make(chan error, 1)
	workerDone := make(chan error, 1)
	workerInputDrained := make(chan struct{})
	go func() {
		feedDone <- s.feed.run(
			ctx,
			orderFeedConnectionHooks{
				beforeRead: func(context.Context) {
					inbox.beginRecovery()
					s.log.V(1).Info("order recovery started", "executor", s.cfg.Executor.Hex())
				},
				whileConnected: func(connectionCtx context.Context) {
					defer inbox.endRecovery()
					if !s.recoverOrdersUntilSuccess(connectionCtx, inbox) {
						return
					}
					select {
					case feedConnections <- connectionCtx:
					case <-connectionCtx.Done():
					}
				},
			},
			func(_ context.Context, msg orderMessage) {
				order := s.parseOrderMessage(msg)
				if err := inbox.enqueue(order); err != nil {
					fields := []any{"event", msg.Event}
					if order != nil {
						fields = append(fields,
							"orderId", order.OrderID,
							"onChainOrderId", order.OnChainOrderID,
							"quoteId", order.QuoteID,
						)
					}
					s.log.Error(err, "order feed: dropped order", fields...)
				}
			},
		)
	}()
	go func() { inboxDone <- inbox.run(workCtx, orders) }()
	go func() {
		workerDone <- s.newOrderWorker(workCtx, routes, orders, inbox.markRecoveryRetry, workerInputDrained).run()
	}()

	feedErr := <-feedDone
	inbox.closeInput()
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
		s.log.Info("order inbox drain timed out", "timeout", s.cfg.OrderServer.HTTPTimeout.String())
		stopWork()
	}
	inboxErr := <-inboxDone
	if !workerFinished {
		workerErr = <-workerDone
	}
	return preferLifecycleError(feedErr, preferLifecycleError(inboxErr, workerErr))
}

func (s *Solver) parseOrderMessage(msg orderMessage) *submittedOrder {
	order, err := parseSubmittedOrder(msg.Data, s.cfg, s.chainID)
	if err != nil {
		if errors.Is(err, errOrderForDifferentChain) {
			s.log.Info("order feed: ignored order for another chain", "event", msg.Event, "reason", err.Error())
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

func awaitFill(fill *pendingFill, completions chan<- fillCompletion) {
	result, ok := <-fill.result
	if !ok {
		result.Err = errors.New("transaction result channel closed without a result")
	}
	completions <- fillCompletion{fill: fill, result: result}
}
