package lifi

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type runtimeTxSender struct {
	requests []txmanager.Request
	reject   bool
}

func (sender *runtimeTxSender) Send(
	_ context.Context,
	request txmanager.Request,
) txmanager.Result {
	sender.requests = append(sender.requests, request)
	if sender.reject {
		return txmanager.Result{Err: errors.New("not admitted"), NotAdmitted: true}
	}
	return txmanager.Result{Hash: common.HexToHash("0x01")}
}

type runtimeStrategy struct {
	plan *liquidlane.Plan
	err  error
}

func (runtimeStrategy) DecideQuotes(context.Context, QuoteInput) (QuoteOutput, error) {
	return QuoteOutput{}, nil
}

func (strategy runtimeStrategy) DecideFill(
	context.Context,
	FillInput,
) (FillDecision, error) {
	return FillDecision{Plan: strategy.plan}, strategy.err
}

func TestProcessOrderLifecycle(t *testing.T) {
	fixture := runtimeFixture(t)
	tests := []struct {
		name       string
		status     uint8
		strategy   runtimeStrategy
		reject     bool
		wantSend   bool
		wantAction orderAction
	}{
		{name: "deposited submits", status: lifiOrderStatusDeposited, strategy: runtimeStrategy{plan: fixture.plan}, wantSend: true},
		{name: "deposit propagation retries", status: lifiOrderStatusNone, strategy: runtimeStrategy{plan: fixture.plan}, wantAction: orderWaitDeposit},
		{name: "temporary strategy error retries recovery", status: lifiOrderStatusDeposited, strategy: runtimeStrategy{err: errors.New("temporary")}, wantAction: orderRetryStrategy},
		{name: "tx lane rejection releases fill", status: lifiOrderStatusDeposited, strategy: runtimeStrategy{plan: fixture.plan}, reject: true, wantSend: true},
		{name: "claimed is terminal", status: lifiOrderStatusClaimed, strategy: runtimeStrategy{plan: fixture.plan}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &runtimeTxSender{reject: test.reject}
			solver := fixture.solver(test.status, test.strategy, sender)
			action := solver.processOrder(t.Context(), fixture.routes, fixture.order)
			if (len(sender.requests) == 1) != test.wantSend || action != test.wantAction {
				t.Fatalf("action = %d, sends=%d, want %d/%t", action, len(sender.requests), test.wantAction, test.wantSend)
			}
			if solver.capacity.Len() != 0 {
				t.Fatal("completed synchronous fill leaked capacity")
			}
		})
	}
}

func TestWorkerOrderLifecycleCoordinatesRetryKinds(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	order := &submittedOrder{OrderID: "order-1", Order: inputsettler.StandardOrder{FillDeadline: uint32(now.Add(time.Minute).Unix())}}
	states := newOrderBook(4)
	if err := states.scheduleDeposit(order, now); err != nil {
		t.Fatal(err)
	}
	queued, err := states.enqueueCapacity(order)
	if err != nil {
		t.Fatal(err)
	}
	deposit, capacity := states.retryCounts()
	if queued || len(states.records) != 1 || deposit != 1 || capacity != 0 {
		t.Fatalf("state = %+v", states)
	}
	states.finishDeposit(order)
	queued, err = states.enqueueCapacity(order)
	if err != nil || !queued {
		t.Fatalf("enqueue capacity after deposit = (%t, %v)", queued, err)
	}
	if got := states.popCapacity(); got != order {
		t.Fatalf("capacity retry = %p, want %p", got, order)
	}
	deposit, capacity = states.retryCounts()
	if len(states.records) != 0 || deposit != 0 || capacity != 0 {
		t.Fatalf("terminal state retained: %+v", states)
	}
}

func TestWorkerStopsAfterInputClose(t *testing.T) {
	book := newOrderBook(1)
	book.closeInput()
	worker := (&Solver{}).newOrderWorker(t.Context(), nil, book, nil)
	if err := worker.run(); err != nil {
		t.Fatal(err)
	}
	if len(book.records) != 0 {
		t.Fatal("closed order book retained lifecycle state")
	}
}

func TestWorkerOrderLifecycleBoundsDepositRetry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	order := &submittedOrder{OrderID: "order-1", Order: inputsettler.StandardOrder{FillDeadline: uint32(now.Add(time.Second).Unix())}}
	states := newOrderBook(4)
	if err := states.scheduleDeposit(order, now); err != nil {
		t.Fatal(err)
	}
	states.mu.Lock()
	_, next := states.nextDepositLocked()
	states.mu.Unlock()
	if next == nil || !next.depositReady.Before(time.Unix(int64(order.Order.FillDeadline), 0)) {
		t.Fatalf("next = %+v, deadline = %d", next, order.Order.FillDeadline)
	}
	got, err := states.popDepositReady(time.Unix(int64(order.Order.FillDeadline), 0))
	if got != order || !errors.Is(err, errOrderDepositRetryExpired) || len(states.records) != 0 {
		t.Fatalf("pop = %p, err = %v, states = %d", got, err, len(states.records))
	}
}

func TestOrderBookKeepsRecoveryBudgetAcrossSweeps(t *testing.T) {
	order := &submittedOrder{OrderID: "order-1"}
	book := newOrderBook(1)
	book.beginRecovery()
	for attempt := 1; attempt <= 3; attempt++ {
		book.markRecoveryRetry(order, 3)
		retries := book.takeRecoveryRetries()
		if attempt < 3 {
			if len(retries) != 1 {
				t.Fatalf("attempt %d retries = %d, want 1", attempt, len(retries))
			}
			if err := book.enqueue(retries[0]); err != nil {
				t.Fatal(err)
			}
			book.accepted(<-book.orders)
		} else if len(retries) != 0 {
			t.Fatalf("attempt limit retained %d retries", len(retries))
		}
	}
	if !book.tryEndRecovery() {
		t.Fatal("recovery remained active after bounded retries")
	}
}

type lifiRuntimeFixture struct {
	cfg     *Config
	caller  common.Address
	tokenIn common.Address
	order   *submittedOrder
	routes  []route
	plan    *liquidlane.Plan
}

func runtimeFixture(t *testing.T) lifiRuntimeFixture {
	t.Helper()
	cfg := testLifiConfig()
	cfg.Gas = nil
	caller := common.HexToAddress("0x5555555555555555555555555555555555555555")
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	adapter := common.HexToAddress("0x9999999999999999999999999999999999999999")
	order := testSubmittedOrder(t, cfg, tokenIn, tokenOut)
	routeItem := route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter,
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	plan := &liquidlane.Plan{Routes: []liquidlane.PlanLeg{{
		RouteID: routeItem.ID, CapacityID: routeItem.CapacityID, Adapter: adapter,
		AmountIn: big.NewInt(1_000_000), ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut: big.NewInt(990_000), ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	return lifiRuntimeFixture{cfg: cfg, caller: caller, tokenIn: tokenIn, order: order, routes: []route{routeItem}, plan: plan}
}

func (fixture lifiRuntimeFixture) solver(
	status uint8,
	strategy runtimeStrategy,
	sender *runtimeTxSender,
) *Solver {
	return &Solver{
		cfg: fixture.cfg, chainID: 11155111, caller: fixture.caller,
		reader: fakeLifiReader{
			orderID: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			status:  status,
			fill: []liquidlane.FillQuote{{
				Inventory: liquidlane.Inventory{Route: fixture.routes[0], MaxAssets: big.NewInt(2_000_000)},
				AmountIn:  big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
			}},
		},
		planner: strategy, txm: sender, log: logr.Discard(), capacity: new(capacity.Book),
		now:          func(context.Context) (time.Time, error) { return time.Unix(1_700_000_000, 0), nil },
		wallNow:      func() time.Time { return time.Unix(1_700_000_000, 0) },
		maxFeePerGas: func(context.Context) (*big.Int, error) { return big.NewInt(1), nil },
	}
}
