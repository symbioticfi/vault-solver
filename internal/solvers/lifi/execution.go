package lifi

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"golang.org/x/sync/errgroup"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const (
	fillCompletionCapacity = 128
	orderInboxCapacity     = 1_024
)

var errOrderInboxFull = errors.New("order inbox is full")

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

// orderInbox keeps the WebSocket reader independent from slower on-chain planning.
// The feed is the only producer and run is the only consumer.
type orderInbox struct {
	mu       sync.Mutex
	orders   []*submittedOrder
	queued   map[string]bool
	capacity int
	ready    chan struct{}
}

func newOrderInbox(capacity int) *orderInbox {
	if capacity <= 0 {
		panic("lifi: order inbox capacity must be positive")
	}
	return &orderInbox{
		queued: make(map[string]bool), capacity: capacity, ready: make(chan struct{}, 1),
	}
}

func (q *orderInbox) enqueue(order *submittedOrder) error {
	if order == nil {
		return nil
	}
	key := orderInboxKey(order)
	q.mu.Lock()
	if key != "" && q.queued[key] {
		q.mu.Unlock()
		return nil
	}
	if len(q.orders) >= q.capacity {
		q.mu.Unlock()
		return errOrderInboxFull
	}
	q.orders = append(q.orders, order)
	if key != "" {
		q.queued[key] = true
	}
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
	return nil
}

func (q *orderInbox) run(ctx context.Context, out chan<- *submittedOrder) error {
	defer close(out)
	for {
		q.mu.Lock()
		if len(q.orders) == 0 {
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
	if order.OnChainOrderID != "" {
		return order.OnChainOrderID
	}
	return order.OrderID
}

func (s *Solver) runOrderFeed(ctx context.Context, routes []route) error {
	inbox := newOrderInbox(orderInboxCapacity)
	orders := make(chan *submittedOrder)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.feed.run(gctx, func(_ context.Context, msg orderMessage) {
			order := s.parseOrderMessage(msg)
			if err := inbox.enqueue(order); err != nil {
				s.log.Error(err, "order feed: dropped order", "event", msg.Event)
			}
		})
	})
	g.Go(func() error { return inbox.run(gctx, orders) })
	g.Go(func() error { return s.runOrderWorker(gctx, routes, orders) })
	return g.Wait()
}

func (s *Solver) parseOrderMessage(msg orderMessage) *submittedOrder {
	order, err := parseSubmittedOrder(msg.Data, s.cfg, s.chainID)
	if err != nil {
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
	)
	return order
}

func (s *Solver) runOrderWorker(
	ctx context.Context,
	routes []route,
	orders <-chan *submittedOrder,
) error {
	pending := pendingFillState{byOrder: make(map[string]*pendingFill)}
	completions := make(chan fillCompletion, fillCompletionCapacity)
	for orders != nil || pending.len() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case completion := <-completions:
			s.completeFill(&pending, completion)
		case order, ok := <-orders:
			if !ok {
				orders = nil
				continue
			}
			fill := s.processOrderWithPending(ctx, routes, order, &pending)
			if fill == nil {
				continue
			}
			pending.add(fill)
			go awaitFill(ctx, fill, completions)
		}
	}
	return nil
}

func awaitFill(ctx context.Context, fill *pendingFill, completions chan<- fillCompletion) {
	select {
	case result, ok := <-fill.result:
		if !ok {
			result.Err = errors.New("transaction result channel closed without a result")
		}
		select {
		case completions <- fillCompletion{fill: fill, result: result}:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
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

func (s *pendingFillState) add(fill *pendingFill) {
	s.byOrder[fill.reservationKey] = fill
}

func (s *pendingFillState) remove(key string) {
	delete(s.byOrder, key)
}
