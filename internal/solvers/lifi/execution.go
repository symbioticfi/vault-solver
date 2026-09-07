package lifi

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
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
	plannedSurplus *big.Int
	result         <-chan txmanager.Result
}

type fillCompletion struct {
	fill   *pendingFill
	result txmanager.Result
}

type orderRecoveryResult struct {
	listed       int
	discovered   int
	processedGen uint64
}

// The feed owns intake. Inbox and worker outlive a disconnected feed just long
// enough to transfer buffered work; admitted transactions are always joined.
func (s *Solver) runOrderFeed(ctx context.Context, routes []route, connections chan<- context.Context) error {
	inbox := newOrderInbox(orderInboxCapacity)
	stopInbox := s.metrics.trackOrderQueue(orderQueueInbox, inbox.orderQueueSnapshot)
	defer stopInbox()
	stopRetries := s.metrics.trackOrderQueue(orderQueueRecoveryRetry, inbox.recoveryRetryQueueSnapshot)
	defer stopRetries()
	work, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	orders := make(chan *submittedOrder)
	drained := make(chan struct{})
	inboxResult, workerResult := make(chan error, 1), make(chan error, 1)
	go func() { inboxResult <- inbox.run(work, orders) }()
	go func() { workerResult <- s.runOrderWorker(work, routes, orders, inbox.markRecoveryRetry, drained) }()
	feedErr := s.feed.run(ctx, orderFeedConnectionHooks{
		beforeRead: func(context.Context) { inbox.beginRecovery() },
		whileConnected: func(connection context.Context) {
			defer inbox.endRecovery()
			if !s.recoverOrdersUntilSuccess(connection, inbox) {
				return
			}
			s.feed.markRecoveryReady(connection)
			select {
			case connections <- connection:
			case <-connection.Done():
			}
		},
	}, func(_ context.Context, message orderMessage) {
		order := s.parseOrderMessage(message)
		if err := inbox.enqueue(order); err != nil {
			s.metrics.observeOrderQueueDrop(orderQueueInbox, err)
			log := s.log.WithValues("event", message.Event)
			if order != nil {
				log = s.orderLogger(order, common.Hash{}).WithValues("event", message.Event)
			}
			log.Error(err, "order feed: dropped order")
		}
	})
	inbox.closeInput()
	// Stop only pre-admission work on timeout. The worker continues to consume all
	// txmanager results even after its context is canceled.
	timeout := time.NewTimer(s.cfg.OrderServer.HTTPTimeout)
	defer timeout.Stop()
	var workerErr error
	select {
	case <-drained:
		workerErr = <-workerResult
	case workerErr = <-workerResult:
	case <-timeout.C:
		s.log.Info("order inbox drain timed out", "timeout", s.cfg.OrderServer.HTTPTimeout)
		cancel()
		workerErr = <-workerResult
	}
	return preferLifecycleError(feedErr, preferLifecycleError(<-inboxResult, workerErr))
}

// Readiness requires a quiet completed generation, not merely a successful REST
// response. A racing WebSocket delivery or retry invalidates that generation.
func (s *Solver) recoverOrdersUntilSuccess(ctx context.Context, inbox *orderInbox) bool {
	timer := observability.StartOperation(s.operationObservers().orderRecovery)
	outcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, outcome) }()
	for delay := initialOrderRecoveryBackoff; ctx.Err() == nil; delay = min(delay*2, maximumOrderRecoveryBackoff) {
		seen := make(map[string]bool)
		var failure error
		for sweep := 1; sweep <= maximumOrderRecoverySweeps; sweep++ {
			result, err := s.recoverOrders(ctx, inbox, seen)
			if err != nil {
				failure = err
				break
			}
			s.log.V(1).Info("order recovery sweep completed", "sweep", sweep,
				"listedOrders", result.listed, "discoveredOrders", result.discovered, "seenOrders", len(seen))
			if result.discovered == 0 && inbox.tryEndRecovery(result.processedGen) {
				outcome = observability.ExternalOperationSuccess
				s.log.Info("order recovery completed", "listedOrders", result.listed, "seenOrders", len(seen))
				return true
			}
		}
		if ctx.Err() != nil {
			return false
		}
		if failure == nil {
			failure = errors.Errorf("order recovery did not converge after %d sweeps", maximumOrderRecoverySweeps)
		}
		s.log.Error(failure, "order recovery failed; retrying", "backoff", delay)
		if !waitForRetry(ctx, delay) {
			return false
		}
	}
	return false
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	wait := time.NewTimer(delay)
	defer wait.Stop()
	select {
	case <-wait.C:
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}

func (s *Solver) recoverOrders(ctx context.Context, inbox *orderInbox, seen map[string]bool) (orderRecoveryResult, error) {
	raw, err := s.orders.listRecoverableOrders(ctx, s.cfg.Executor)
	if err != nil {
		return orderRecoveryResult{}, err
	}
	result := orderRecoveryResult{listed: len(raw)}
	// Retries precede new records and explicitly reopen their deduplication entry.
	deliver := func(order *submittedOrder, retry bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if order == nil {
			return nil
		}
		key := orderInboxKey(order)
		if retry {
			delete(seen, key)
		} else if key != "" && seen[key] {
			return nil
		}
		if err := inbox.enqueueWait(ctx, order); err != nil {
			return errors.Errorf("deliver recovery order: %w", err)
		}
		if key != "" {
			seen[key] = true
			result.discovered++
		}
		return nil
	}
	for _, order := range inbox.takeRecoveryRetries() {
		if err := deliver(order, true); err != nil {
			return orderRecoveryResult{}, err
		}
	}
	for _, item := range raw {
		if err := deliver(s.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: item}), false); err != nil {
			return orderRecoveryResult{}, err
		}
	}
	generation, err := inbox.waitUntilProcessed(ctx)
	if err != nil {
		return orderRecoveryResult{}, errors.Errorf("wait for recovered orders: %w", err)
	}
	result.processedGen = generation
	return result, nil
}

func (s *Solver) parseOrderMessage(msg orderMessage) *submittedOrder {
	log := s.log.WithValues("event", msg.Event)
	order, err := parseSubmittedOrder(msg.Data, s.cfg, s.chainID)
	switch {
	case errors.Is(err, errOrderForDifferentChain):
		log.Info("order feed: ignored order for another chain", "reason", err.Error())
	case errors.Is(err, errOrderUnsupported):
		log.V(1).Info("order feed: ignored unsupported order", "reason", err.Error())
	case err != nil:
		log.Error(err, "order feed: ignored order")
	default:
		log = log.WithValues("orderId", order.OrderID, "onChainOrderId", order.OnChainOrderID, "quoteId", order.QuoteID)
		if isDutchAuctionContext(order.Output.Context) {
			log.Info("order feed: ignored unsupported Dutch auction", "contextType", hexutil.Encode(order.Output.Context[:1]))
			return nil
		}
		log.Info("order received", "orderStatus", order.OrderStatus,
			"inputSettler", order.InputSettler.Hex(), "tokenIn", order.TokenIn.Hex(), "tokenOut", order.TokenOut.Hex(),
			"amountIn", bigString(order.AmountIn), "requiredAmountOut", bigString(order.OutputAmount),
			"expires", order.Order.Expires, "fillDeadline", order.Order.FillDeadline)
		return order
	}
	return nil
}

func awaitFill(fill *pendingFill, completions chan<- fillCompletion) {
	result, ok := <-fill.result
	if !ok {
		result = txmanager.Result{Outcome: txmanager.OutcomeTrackingStopped, Err: errors.New("transaction result channel closed without a result")}
	}
	completions <- fillCompletion{fill: fill, result: result}
}
