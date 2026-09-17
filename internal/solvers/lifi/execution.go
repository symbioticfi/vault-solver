package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
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

type pendingFillState struct {
	byOrder map[string]*pendingFill
}

type orderRecoveryResult struct {
	listed       int
	discovered   int
	processedGen uint64
}

// orderInbox keeps WebSocket delivery and REST recovery independent from slower on-chain planning.
// The feed and recovery sweep may produce concurrently; run is the only consumer.
type orderInbox struct {
	mu                sync.Mutex
	orders            []*submittedOrder
	delivering        *submittedOrder
	queued            map[string]bool
	recoverySeen      map[string]bool
	recoverySeenOrder []string
	recoverySeenNext  int
	recoveryOverflow  bool
	recoveryRetry     map[string]*submittedOrder
	recoveryAttempts  map[string]int
	recoveryGen       uint64
	closed            bool
	capacity          int
	ready             chan struct{}
	space             chan struct{}
}

func newOrderInbox(capacity int) *orderInbox {
	if capacity <= 0 {
		panic("lifi: order inbox capacity must be positive")
	}
	return &orderInbox{
		queued: make(map[string]bool), capacity: capacity,
		ready: make(chan struct{}, 1), space: make(chan struct{}, 1),
	}
}

func (q *orderInbox) enqueue(order *submittedOrder) error {
	if order == nil {
		return nil
	}
	key := orderInboxKey(order)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errOrderInboxClosed
	}
	if key != "" && q.recoverySeen[key] {
		q.mu.Unlock()
		return nil
	}
	if key != "" && q.queued[key] {
		q.markRecoverySeen(key)
		q.mu.Unlock()
		return nil
	}
	if len(q.orders) >= q.capacity {
		if q.recoverySeen != nil {
			q.recoveryOverflow = true
		}
		q.mu.Unlock()
		return errOrderInboxFull
	}
	if order.processed == nil {
		q.recoveryGen++
	} else {
		order.recoveryGen = q.recoveryGen
	}
	q.orders = append(q.orders, order)
	if key != "" {
		q.queued[key] = true
		q.markRecoverySeen(key)
	}
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
	return nil
}

func (q *orderInbox) closeInput() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *orderInbox) markRecoverySeen(key string) {
	if key == "" || q.recoverySeen == nil || q.recoverySeen[key] {
		return
	}
	if len(q.recoverySeenOrder) < orderRecoverySeenCapacity {
		q.recoverySeenOrder = append(q.recoverySeenOrder, key)
	} else {
		evicted := q.recoverySeenOrder[q.recoverySeenNext]
		delete(q.recoverySeen, evicted)
		q.recoverySeenOrder[q.recoverySeenNext] = key
		q.recoverySeenNext = (q.recoverySeenNext + 1) % orderRecoverySeenCapacity
	}
	q.recoverySeen[key] = true
}

func (q *orderInbox) enqueueWait(ctx context.Context, order *submittedOrder) error {
	for {
		err := q.enqueue(order)
		if !errors.Is(err, errOrderInboxFull) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.space:
		}
	}
}

func (q *orderInbox) waitUntilProcessed(ctx context.Context) (uint64, error) {
	barrier := &submittedOrder{processed: make(chan struct{})}
	if err := q.enqueueWait(ctx, barrier); err != nil {
		return 0, err
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-barrier.processed:
		return barrier.recoveryGen, nil
	}
}

func (q *orderInbox) beginRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.recoverySeen = make(map[string]bool)
	q.recoverySeenOrder = nil
	q.recoverySeenNext = 0
	q.recoveryOverflow = false
	q.recoveryRetry = make(map[string]*submittedOrder)
	q.recoveryAttempts = make(map[string]int)
	q.recoveryGen = 0
}

func (q *orderInbox) endRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.recoverySeen = nil
	q.recoverySeenOrder = nil
	q.recoverySeenNext = 0
	q.recoveryOverflow = false
	q.recoveryRetry = nil
	q.recoveryAttempts = nil
	q.recoveryGen = 0
}

// markRecoveryRetry re-queues an order for the next recovery sweep and reports whether it did, so
// the worker knows the order is still referenced and keeps its processing span open.
func (q *orderInbox) markRecoveryRetry(order *submittedOrder, attemptLimit int) bool {
	key := orderInboxKey(order)
	if key == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.recoverySeen == nil {
		return false
	}
	// A zero limit deliberately preserves unbounded recovery for chain, RPC, and
	// pre-admission failures. Positive limits count failures by stable order key, so
	// the budget survives both recovery sweeps and reconstructed REST order values.
	if attemptLimit > 0 {
		q.recoveryAttempts[key]++
		if q.recoveryAttempts[key] >= attemptLimit {
			return false
		}
	}
	q.recoveryRetry[key] = order
	q.recoveryGen++
	return true
}

func (q *orderInbox) takeRecoveryRetries() []*submittedOrder {
	q.mu.Lock()
	defer q.mu.Unlock()
	orders := make([]*submittedOrder, 0, len(q.recoveryRetry))
	retrying := make(map[string]bool, len(q.recoveryRetry))
	for key, order := range q.recoveryRetry {
		orders = append(orders, order)
		retrying[key] = true
		delete(q.recoverySeen, key)
	}
	q.recoveryRetry = make(map[string]*submittedOrder)
	seenOrder := make([]string, 0, len(q.recoverySeenOrder))
	for offset := range len(q.recoverySeenOrder) {
		index := (q.recoverySeenNext + offset) % len(q.recoverySeenOrder)
		key := q.recoverySeenOrder[index]
		if key != "" && !retrying[key] && q.recoverySeen[key] {
			seenOrder = append(seenOrder, key)
		}
	}
	q.recoverySeenOrder = seenOrder
	q.recoverySeenNext = 0
	return orders
}

func (q *orderInbox) tryEndRecovery(processedGen uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.recoveryOverflow || len(q.recoveryRetry) > 0 {
		q.recoveryOverflow = false
		return false
	}
	if q.recoveryGen != processedGen {
		return false
	}
	q.recoverySeen = nil
	q.recoverySeenOrder = nil
	q.recoverySeenNext = 0
	q.recoveryRetry = nil
	q.recoveryAttempts = nil
	q.recoveryGen = 0
	return true
}

func (q *orderInbox) run(ctx context.Context, out chan<- *submittedOrder) error {
	defer close(out)
	for {
		q.mu.Lock()
		if len(q.orders) == 0 {
			if q.closed {
				q.mu.Unlock()
				return nil
			}
			q.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-q.ready:
				continue
			}
		}
		order := q.orders[0]
		q.orders[0] = nil
		q.orders = q.orders[1:]
		q.delivering = order
		if len(q.orders) == 0 {
			q.orders = nil
		}
		q.mu.Unlock()
		select {
		case q.space <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			q.mu.Lock()
			q.delivering = nil
			q.mu.Unlock()
			return ctx.Err()
		case out <- order:
		}
		q.mu.Lock()
		q.delivering = nil
		if key := orderInboxKey(order); key != "" {
			delete(q.queued, key)
		}
		q.mu.Unlock()
	}
}

func (q *orderInbox) orderQueueSnapshot() orderQueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()

	var snapshot orderQueueSnapshot
	add := func(order *submittedOrder) {
		if order == nil || order.processed != nil {
			return
		}
		snapshot.backlog++
		snapshot.nearestDeadline = earlierOrderDeadlineUnix(snapshot.nearestDeadline, order)
	}
	add(q.delivering)
	for _, order := range q.orders {
		add(order)
	}
	return snapshot
}

func (q *orderInbox) recoveryRetryQueueSnapshot() orderQueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()

	var snapshot orderQueueSnapshot
	for _, order := range q.recoveryRetry {
		if order == nil || order.processed != nil {
			continue
		}
		snapshot.backlog++
		snapshot.nearestDeadline = earlierOrderDeadlineUnix(snapshot.nearestDeadline, order)
	}
	return snapshot
}

func orderInboxKey(order *submittedOrder) string {
	if order.dedupeKey != "" {
		return order.dedupeKey
	}
	if order.OnChainOrderID != "" {
		return strings.ToLower(strings.TrimSpace(order.OnChainOrderID))
	}
	return strings.ToLower(strings.TrimSpace(order.OrderID))
}

func (s *Solver) runOrderFeed(
	ctx context.Context,
	routes []route,
	feedConnections chan<- context.Context,
) error {
	inbox := newOrderInbox(orderInboxCapacity)
	stopInboxMetrics := s.metrics.trackOrderQueue(orderQueueInbox, inbox.orderQueueSnapshot)
	defer stopInboxMetrics()
	stopRecoveryRetryMetrics := s.metrics.trackOrderQueue(
		orderQueueRecoveryRetry,
		inbox.recoveryRetryQueueSnapshot,
	)
	defer stopRecoveryRetryMetrics()
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
				beforeRead: func(connectionCtx context.Context) {
					inbox.beginRecovery()
					observability.Log(connectionCtx).V(1).Info(
						"order recovery started", "executor", s.cfg.Executor.Hex())
				},
				whileConnected: func(connectionCtx context.Context) {
					defer inbox.endRecovery()
					if !s.recoverOrdersUntilSuccess(connectionCtx, inbox) {
						return
					}
					s.feed.markRecoveryReady(connectionCtx)
					select {
					case feedConnections <- connectionCtx:
					case <-connectionCtx.Done():
					}
				},
			},
			func(connectionCtx context.Context, msg orderMessage) {
				s.acceptOrderMessage(connectionCtx, inbox, msg)
			},
		)
	}()
	go func() { inboxDone <- inbox.run(workCtx, orders) }()
	go func() {
		workerDone <- s.runOrderWorker(workCtx, routes, orders, inbox.markRecoveryRetry, workerInputDrained)
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
		observability.Log(ctx).Info("order inbox drain timed out", "timeout", s.cfg.OrderServer.HTTPTimeout.String())
		stopWork()
	}
	inboxErr := <-inboxDone
	if !workerFinished {
		workerErr = <-workerDone
	}
	return preferLifecycleError(feedErr, preferLifecycleError(inboxErr, workerErr))
}

// acceptOrderMessage admits one live feed message to the inbox under its message span.
func (s *Solver) acceptOrderMessage(ctx context.Context, inbox *orderInbox, msg orderMessage) {
	_, _ = s.admitOrderMessage(ctx, msg, func(msgCtx context.Context, order *submittedOrder) error {
		err := inbox.enqueue(order)
		if err == nil {
			return nil
		}
		s.metrics.observeOrderQueueDrop(orderQueueInbox, err)
		observability.Log(msgCtx).Error(err, "order feed: dropped order",
			"event", msg.Event,
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
		)
		return err
	})
}

// admitOrderMessage spans one feed message as lifi.order.<event> (spec §6.4): it parses the order
// the message carries, stamps the span context on it so the worker continues this trace, and hands
// it to admit. The span covers parsing and queue admission alike and always ends here.
func (s *Solver) admitOrderMessage(
	ctx context.Context,
	msg orderMessage,
	admit func(context.Context, *submittedOrder) error,
) (order *submittedOrder, err error) {
	ctx, end := tracer.Start(ctx, orderMessageSpanName(msg.Event))
	defer func() { end(err) }()

	order, err = s.parseOrderMessage(ctx, msg)
	if order == nil {
		return nil, err
	}
	order.span = trace.SpanContextFromContext(ctx)
	return order, admit(ctx, order)
}

func (s *Solver) recoverOrdersUntilSuccess(
	ctx context.Context,
	inbox *orderInbox,
) bool {
	timer := observability.StartOperation(s.operationObservers().orderRecovery)
	outcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, outcome) }()

	backoff := initialOrderRecoveryBackoff
	recovered := make(map[string]bool)
	successfulSweeps := 0
	for {
		result, err := s.recoverOrders(ctx, inbox, recovered)
		if err == nil {
			successfulSweeps++
			observability.Log(ctx).V(1).Info(
				"order recovery sweep completed",
				"sweep", successfulSweeps,
				"listedOrders", result.listed,
				"discoveredOrders", result.discovered,
				"seenOrders", len(recovered),
			)
			if result.discovered == 0 && inbox.tryEndRecovery(result.processedGen) {
				outcome = observability.ExternalOperationSuccess
				observability.Log(ctx).Info(
					"order recovery completed", "listedOrders", result.listed, "seenOrders", len(recovered))
				return true
			}
			if successfulSweeps < maximumOrderRecoverySweeps {
				continue
			}
			err = errors.Errorf("order recovery did not converge after %d sweeps", successfulSweeps)
		}
		successfulSweeps = 0
		if ctx.Err() != nil {
			return false
		}
		recovered = make(map[string]bool)
		observability.Log(ctx).Error(err, "order recovery failed; retrying", "backoff", backoff.String())
		if !waitForRetry(ctx, backoff) {
			return false
		}
		backoff = min(2*backoff, maximumOrderRecoveryBackoff)
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Solver) recoverOrders(
	ctx context.Context,
	inbox *orderInbox,
	recovered map[string]bool,
) (orderRecoveryResult, error) {
	rawOrders, err := s.orders.listRecoverableOrders(ctx, s.cfg.Executor)
	if err != nil {
		return orderRecoveryResult{}, err
	}
	result := orderRecoveryResult{listed: len(rawOrders)}
	for _, order := range inbox.takeRecoveryRetries() {
		key := orderInboxKey(order)
		delete(recovered, key)
		if err := inbox.enqueueWait(ctx, order); err != nil {
			return orderRecoveryResult{}, errors.Errorf("re-enqueue recovery retry: %w", err)
		}
		if key != "" {
			recovered[key] = true
			result.discovered++
		}
	}
	for _, raw := range rawOrders {
		if ctx.Err() != nil {
			return orderRecoveryResult{}, ctx.Err()
		}
		discovered, err := s.recoverOrder(ctx, inbox, raw, recovered)
		if err != nil {
			return orderRecoveryResult{}, err
		}
		if discovered {
			result.discovered++
		}
	}
	result.processedGen, err = inbox.waitUntilProcessed(ctx)
	if err != nil {
		return orderRecoveryResult{}, errors.Errorf("wait for recovered orders: %w", err)
	}
	return result, nil
}

// recoverOrder admits one listed order to the inbox under its own message span and reports whether
// this sweep had not already seen it.
func (s *Solver) recoverOrder(
	ctx context.Context,
	inbox *orderInbox,
	raw json.RawMessage,
	recovered map[string]bool,
) (discovered bool, err error) {
	order, err := s.admitOrderMessage(ctx, orderMessage{Event: orderSubmitEvent, Data: raw},
		func(msgCtx context.Context, order *submittedOrder) error {
			key := orderInboxKey(order)
			if key != "" && recovered[key] {
				observability.Decline(msgCtx, "order_skipped", "already recovered by this sweep")
				return nil
			}
			if enqueueErr := inbox.enqueueWait(ctx, order); enqueueErr != nil {
				return errors.Errorf("enqueue recovered order: %w", enqueueErr)
			}
			if key != "" {
				recovered[key] = true
				discovered = true
			}
			return nil
		})
	if order == nil {
		// An undecodable or ignored listing is logged and metered by parseOrderMessage and
		// recorded on its message span. Skip it: one bad order must not abort the sweep.
		return false, nil
	}
	return discovered, err
}

// parseOrderMessage decodes one feed message under the message span already on ctx. A message this
// solver ignores yields a nil order and a nil error, recorded as a decline; only a message we could
// not understand is a failure.
func (s *Solver) parseOrderMessage(
	ctx context.Context,
	msg orderMessage,
) (*submittedOrder, error) {
	order, err := parseSubmittedOrder(msg.Data, s.cfg, s.chainID)
	if err != nil {
		fields := orderDiagnosticFields(msg.Data, err)
		fields = append(fields, "event", msg.Event)
		switch {
		case errors.Is(err, errOrderForDifferentChain):
			s.metrics.observeOrderParse("other_chain")
			observability.Decline(ctx, "order_ignored", "order is for another chain")
			observability.Log(ctx).Info("order feed: ignored order for another chain", append(fields, "reason", err.Error())...)
		case errors.Is(err, errNativeInputUnsupported):
			s.metrics.observeOrderParse("unsupported")
			observability.Decline(ctx, "order_ignored", "native input is not supported")
			observability.Log(ctx).Info("order feed: ignored unsupported order", append(fields, "reason", err.Error())...)
		case errors.Is(err, errOrderUnsupported):
			s.metrics.observeOrderParse("unsupported")
			observability.Decline(ctx, "order_ignored", "order is not fillable by this solver")
			observability.Log(ctx).V(1).Info("order feed: ignored unsupported order", append(fields, "reason", err.Error())...)
		default:
			// A message we cannot understand is a real failure, not an expected skip.
			s.metrics.observeOrderParse("invalid")
			observability.Log(ctx).Error(err, "order feed: ignored order", fields...)
			return nil, err
		}
		return nil, nil
	}
	observability.SetAttributes(ctx, orderAttrs(order)...)
	if isDutchAuctionContext(order.Output.Context) {
		s.metrics.observeOrderParse("unsupported")
		observability.Decline(ctx, "order_ignored", "Dutch auction orders are not supported")
		observability.Log(ctx).Info("order feed: ignored unsupported Dutch auction",
			"event", msg.Event,
			"orderId", order.OrderID,
			"onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID,
			"contextType", hexutil.Encode(order.Output.Context[:1]),
		)
		return nil, nil
	}
	observability.Log(ctx).Info("order received",
		"event", msg.Event,
		"orderStatus", order.OrderStatus,
		"orderId", order.OrderID,
		"onChainOrderId", order.OnChainOrderID,
		"quoteId", order.QuoteID,
		"inputSettler", order.InputSettler.Hex(),
		"tokenIn", order.TokenIn.Hex(),
		"tokenOut", order.TokenOut.Hex(),
		"amountIn", bigString(order.AmountIn),
		"requiredAmountOut", bigString(order.OutputAmount),
		"expires", order.Order.Expires,
		"fillDeadline", order.Order.FillDeadline,
	)
	return order, nil
}

func (s *Solver) runOrderWorker(
	ctx context.Context,
	routes []route,
	orders <-chan *submittedOrder,
	onRetryable func(*submittedOrder, int) bool,
	inputDrained chan<- struct{},
) error {
	pending := pendingFillState{byOrder: make(map[string]*pendingFill)}
	traces := newOrderTraces()
	completions := make(chan fillCompletion, fillCompletionCapacity)
	retries := newReservationRetryQueue(orderRetryCapacity)
	depositRetries := newOrderDepositRetryQueue(orderDepositRetryCapacity)
	stopCapacityRetryMetrics := s.metrics.trackOrderQueue(orderQueueCapacityRetry, retries.orderQueueSnapshot)
	defer stopCapacityRetryMetrics()
	stopDepositRetryMetrics := s.metrics.trackOrderQueue(orderQueueDepositRetry, depositRetries.orderQueueSnapshot)
	defer stopDepositRetryMetrics()
	var reservationReleaseGen uint64
	ctxDone := ctx.Done()
	var runErr error
	// A shutdown that drops queued retries still has to close their processing spans.
	defer func() { traces.finishAll(runErr) }()
	var recoveryBarrier chan struct{}
	retryNow := s.wallNow
	if retryNow == nil {
		retryNow = time.Now
	}
	depositRetryTimer := time.NewTimer(maximumOrderDepositRetryWindow)
	depositRetryTimer.Stop()
	defer depositRetryTimer.Stop()
	releaseRecoveryBarrier := func() {
		if recoveryBarrier == nil || retries.len() > 0 {
			return
		}
		close(recoveryBarrier)
		recoveryBarrier = nil
	}
	// The processing span belongs to the order, not to one pass through process: a replayed feed
	// message or a recovery sweep can re-enter while the first copy is still in flight, and that
	// pass must not close the span the live copy is still writing to. Close it only once nothing in
	// the worker still holds the order, and nothing has re-queued it through the inbox.
	finishOrderTrace := func(order *submittedOrder, err error) {
		if pending.containsOrder(order) || retries.contains(order) || depositRetries.contains(order) {
			return
		}
		if traces.isHeld(order) {
			return
		}
		traces.finish(order, err)
	}
	process := func(order *submittedOrder, reservations *liquidlane.CapacityReservations, stage string) {
		defer releaseRecoveryBarrier()

		tracked := traces.begin(ctx, order)
		orderCtx, endStage := tracked.attempt(tracked.context(ctx), stage)
		var attemptErr error
		defer func() {
			endStage(attemptErr)
			finishOrderTrace(order, attemptErr)
		}()

		var result orderProcessingResult
		if reservations == nil {
			result = s.processOrderWithPending(orderCtx, routes, order, &pending)
		} else {
			result = s.processOrderUsingReservations(orderCtx, routes, order, &pending, reservations)
		}
		attemptErr = result.err
		outcome := result.outcome
		// Observe after retry admission: a deferred attempt can still become a
		// bounded-queue drop before the worker retains it.
		defer func() { s.metrics.observeOrderProcessing(outcome) }()
		if result.depositNotVisible {
			err := depositRetries.schedule(order, retryNow())
			if err == nil {
				return
			}
			outcome = orderProcessingNotActionable
			s.metrics.observeOrderQueueDrop(orderQueueDepositRetry, err)
			if errors.Is(err, errOrderDepositRetryFull) || errors.Is(err, errOrderDepositRetryKey) {
				attemptErr = err
				observability.Log(orderCtx).Error(err, "order deposit retry: dropped order",
					"orderId", order.OrderID,
					"onChainOrderId", order.OnChainOrderID,
					"quoteId", order.QuoteID,
					"capacity", orderDepositRetryCapacity,
				)
				return
			}
			observability.Decline(orderCtx, "order_skipped", "deposit did not become visible within retry bounds")
			observability.Log(orderCtx).Info("order skipped: deposit did not become visible within retry bounds",
				"orderId", order.OrderID,
				"onChainOrderId", order.OnChainOrderID,
				"quoteId", order.QuoteID,
				"reason", err.Error(),
			)
			return
		}
		depositRetries.finish(order)
		if result.fill != nil {
			pending.add(result.fill)
			go awaitFill(result.fill, completions)
			return
		}
		if result.retryable && onRetryable != nil && onRetryable(order, result.recoveryAttemptLimit) {
			traces.hold(order)
		}
		// Invariant: a queued reservation retry implies a pending fill. Completions are
		// the only events that advance the retry generation and wake the worker.
		if len(result.blockedOn) == 0 || pending.len() == 0 {
			if len(result.blockedOn) > 0 {
				outcome = orderProcessingOther
			}
			return
		}
		queuedBefore := retries.len()
		if err := retries.enqueue(order, reservationReleaseGen); err != nil {
			if errors.Is(err, errOrderRetryFull) {
				outcome = orderProcessingCapacityDropped
			}
			attemptErr = err
			s.metrics.observeOrderQueueDrop(orderQueueCapacityRetry, err)
			observability.Log(orderCtx).Error(err, "order retry queue: dropped newest order",
				"orderId", order.OrderID,
				"onChainOrderId", order.OnChainOrderID,
				"quoteId", order.QuoteID,
				"capacity", orderRetryCapacity,
			)
			return
		}
		if retries.len() > queuedBefore {
			observability.Log(orderCtx).V(1).Info(
				"order fill deferred by pending capacity",
				"orderId", order.OrderID,
				"onChainOrderId", order.OnChainOrderID,
				"quoteId", order.QuoteID,
				"blockedCapacityGroups", len(result.blockedOn),
				"pendingFills", pending.len(),
				"retryQueue", retries.len(),
			)
		}
	}
	complete := func(completion fillCompletion) {
		filledCtx := traces.context(ctx, completion.fill.order)
		// The capacity release below runs after the processing span has ended, so take the fill's
		// logger while that span is still open rather than logging through a closed one.
		filledLog := observability.Log(filledCtx)
		completionErr := s.completeFill(filledCtx, &pending, completion)
		finishOrderTrace(completion.fill.order, completionErr)
		reservationReleaseGen++
		for ctx.Err() == nil {
			order := retries.popReady(reservationReleaseGen)
			if order == nil {
				break
			}
			observability.Log(traces.context(ctx, order)).V(1).Info(
				"order fill retry started",
				"orderId", order.OrderID,
				"onChainOrderId", order.OnChainOrderID,
				"quoteId", order.QuoteID,
				"pendingFills", pending.len(),
				"retryQueue", retries.len(),
			)
			reservations := s.capacity.SnapshotExcluding(completion.fill.reservationKey)
			process(order, &reservations, orderReserveStage)
		}
		if ctx.Err() != nil {
			retries.clear()
		}
		if s.releaseReservationWithoutRefresh(completion.fill.reservationKey) {
			filledLog.V(1).Info(
				"fill capacity released",
				"orderId", completion.fill.order.OrderID,
				"onChainOrderId", completion.fill.orderID.Hex(),
				"quoteId", completion.fill.order.QuoteID,
				"pendingFills", s.capacity.Len(),
			)
			s.requestQuoteRefresh()
		}
		releaseRecoveryBarrier()
	}
	for orders != nil || pending.len() > 0 || retries.len() > 0 || depositRetries.len() > 0 {
		if runErr == nil && ctx.Err() != nil {
			runErr = ctx.Err()
			ctxDone = nil
			orders = nil
			retries.clear()
			depositRetries.clear()
		}
		if runErr != nil && pending.len() == 0 {
			return runErr
		}
		orderInput := orders
		if runErr != nil || recoveryBarrier != nil {
			// Post-barrier orders belong to a later recovery generation and must not
			// extend the retry set protected by this barrier.
			orderInput = nil
		}
		var depositRetryC <-chan time.Time
		depositRetryTimer.Stop()
		if readyAt, ok := depositRetries.nextReadyAt(); ok {
			depositRetryTimer.Reset(max(readyAt.Sub(retryNow()), 0))
			depositRetryC = depositRetryTimer.C
		}
		select {
		case <-ctxDone:
			runErr = ctx.Err()
			ctxDone = nil
			orders = nil
			retries.clear()
			depositRetries.clear()
		case completion := <-completions:
			complete(completion)
		case <-depositRetryC:
			order, err := depositRetries.popReady(retryNow())
			if err != nil {
				orderCtx := traces.context(ctx, order)
				s.metrics.observeOrderProcessing(orderProcessingNotActionable)
				s.metrics.observeOrderQueueDrop(orderQueueDepositRetry, err)
				observability.Decline(
					orderCtx, "order_skipped", "deposit did not become visible within retry bounds",
				)
				observability.Log(orderCtx).Info(
					"order skipped: deposit did not become visible within retry bounds",
					"orderId", order.OrderID,
					"onChainOrderId", order.OnChainOrderID,
					"quoteId", order.QuoteID,
					"reason", err.Error(),
				)
				finishOrderTrace(order, nil)
				continue
			}
			if order != nil {
				process(order, nil, orderDepositStage)
			}
		case order, ok := <-orderInput:
			if !ok {
				orders = nil
				depositRetries.clear()
				releaseRecoveryBarrier()
				if inputDrained != nil {
					close(inputDrained)
					inputDrained = nil
				}
				continue
			}
			if ctx.Err() != nil {
				runErr = ctx.Err()
				ctxDone = nil
				orders = nil
				retries.clear()
				depositRetries.clear()
				continue
			}
			if order.processed != nil {
				recoveryBarrier = order.processed
				releaseRecoveryBarrier()
				continue
			}
			if depositRetries.contains(order) {
				orderCtx := traces.context(ctx, order)
				observability.Decline(orderCtx, "order_skipped", "replay of an order awaiting its deposit")
				observability.Log(orderCtx).V(1).Info(
					"order feed replay coalesced while awaiting on-chain deposit",
					"orderId", order.OrderID,
					"onChainOrderId", order.OnChainOrderID,
					"quoteId", order.QuoteID,
				)
				continue
			}
			process(order, nil, "")
		}
	}
	return runErr
}

func awaitFill(fill *pendingFill, completions chan<- fillCompletion) {
	result, ok := <-fill.result
	if !ok {
		result.Err = errors.New("transaction result channel closed without a result")
	}
	completions <- fillCompletion{fill: fill, result: result}
}

func (s *pendingFillState) len() int {
	if s == nil {
		return 0
	}
	return len(s.byOrder)
}

func (s *pendingFillState) contains(key string) bool {
	if s == nil {
		return false
	}
	_, ok := s.byOrder[key]
	return ok
}

// containsOrder reports whether a fill for this order is pending, matched by the order's stable
// queue key: a replayed copy of an order is a different value with the same key.
func (s *pendingFillState) containsOrder(order *submittedOrder) bool {
	if s == nil {
		return false
	}
	key := orderInboxKey(order)
	for _, fill := range s.byOrder {
		if orderInboxKey(fill.order) == key {
			return true
		}
	}
	return false
}

func (s *pendingFillState) add(fill *pendingFill) {
	s.byOrder[fill.reservationKey] = fill
}

func (s *pendingFillState) remove(key string) {
	delete(s.byOrder, key)
}
