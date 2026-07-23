package lifi

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/sync/errgroup"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const fillCompletionCapacity = 128

type pendingFill struct {
	order          *submittedOrder
	orderID        common.Hash
	reservationKey string
	reservations   []quoteReservation
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
	mu     sync.Mutex
	orders []*submittedOrder
	ready  chan struct{}
}

func newOrderInbox() *orderInbox {
	return &orderInbox{ready: make(chan struct{}, 1)}
}

func (q *orderInbox) enqueue(order *submittedOrder) {
	if order == nil {
		return
	}
	q.mu.Lock()
	q.orders = append(q.orders, order)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
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
	}
}

func (s *Solver) runOrderFeed(ctx context.Context, routes []route) error {
	inbox := newOrderInbox()
	orders := make(chan *submittedOrder)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.feed.run(gctx, func(_ context.Context, msg orderMessage) {
			order := s.parseOrderMessage(msg)
			inbox.enqueue(order)
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
			s.completeFill(ctx, &pending, completion)
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
			s.reserve(ctx, fill.reservationKey, fill.reservations)
			go awaitFill(ctx, fill, completions)
		}
	}
	return nil
}

func awaitFill(ctx context.Context, fill *pendingFill, completions chan<- fillCompletion) {
	select {
	case result := <-fill.result:
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

func (s *pendingFillState) reservedCapacity() map[liquidlane.CapacityID]*big.Int {
	reserved := make(map[liquidlane.CapacityID]*big.Int)
	if s == nil {
		return reserved
	}
	for _, fill := range s.byOrder {
		addReservations(reserved, fill.reservations)
	}
	return reserved
}

func (s *pendingFillState) add(fill *pendingFill) {
	s.byOrder[fill.reservationKey] = fill
}

func (s *pendingFillState) remove(key string) {
	delete(s.byOrder, key)
}
