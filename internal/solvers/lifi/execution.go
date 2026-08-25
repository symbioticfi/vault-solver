package lifi

import (
	"context"
	"strings"
	"sync"
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
	q.signalReady()
	return nil
}

func (q *orderInbox) closeInput() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.signalReady()
}

func (q *orderInbox) signalReady() {
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
	q.clearRecovery()
	q.recoverySeen = make(map[string]bool)
	q.recoveryRetry = make(map[string]*submittedOrder)
	q.recoveryAttempts = make(map[string]int)
}

func (q *orderInbox) endRecovery() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.clearRecovery()
}

func (q *orderInbox) clearRecovery() {
	q.recoverySeen = nil
	q.recoverySeenOrder = nil
	q.recoverySeenNext = 0
	q.recoveryOverflow = false
	q.recoveryRetry = nil
	q.recoveryAttempts = nil
	q.recoveryGen = 0
}

func (q *orderInbox) markRecoveryRetry(order *submittedOrder, attemptLimit int) {
	key := orderInboxKey(order)
	if key == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.recoverySeen == nil {
		return
	}
	// A zero limit deliberately preserves unbounded recovery for chain, RPC, and
	// pre-admission failures. Positive limits count failures by stable order key, so
	// the budget survives both recovery sweeps and reconstructed REST order values.
	if attemptLimit > 0 {
		q.recoveryAttempts[key]++
		if q.recoveryAttempts[key] >= attemptLimit {
			return
		}
	}
	q.recoveryRetry[key] = order
	q.recoveryGen++
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
	q.clearRecovery()
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
			return ctx.Err()
		case out <- order:
		}
		if key := orderInboxKey(order); key != "" {
			q.mu.Lock()
			delete(q.queued, key)
			q.mu.Unlock()
		}
	}
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

func (s *Solver) recoverOrdersUntilSuccess(
	ctx context.Context,
	inbox *orderInbox,
) bool {
	backoff := initialOrderRecoveryBackoff
	recovered := make(map[string]bool)
	successfulSweeps := 0
	for {
		result, err := s.recoverOrders(ctx, inbox, recovered)
		if err == nil {
			successfulSweeps++
			s.log.V(1).Info(
				"order recovery sweep completed",
				"sweep", successfulSweeps,
				"listedOrders", result.listed,
				"discoveredOrders", result.discovered,
				"seenOrders", len(recovered),
			)
			if result.discovered == 0 && inbox.tryEndRecovery(result.processedGen) {
				s.log.Info("order recovery completed", "listedOrders", result.listed, "seenOrders", len(recovered))
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
		s.log.Error(err, "order recovery failed; retrying", "backoff", backoff.String())
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
		order := s.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw})
		if order == nil {
			continue
		}
		key := orderInboxKey(order)
		if key != "" && recovered[key] {
			continue
		}
		if err := inbox.enqueueWait(ctx, order); err != nil {
			return orderRecoveryResult{}, errors.Errorf("enqueue recovered order: %w", err)
		}
		if key != "" {
			recovered[key] = true
			result.discovered++
		}
	}
	result.processedGen, err = inbox.waitUntilProcessed(ctx)
	if err != nil {
		return orderRecoveryResult{}, errors.Errorf("wait for recovered orders: %w", err)
	}
	return result, nil
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
		"inputSettler", order.InputSettler.Hex(),
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
