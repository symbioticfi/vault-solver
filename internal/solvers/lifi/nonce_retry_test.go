package lifi

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Removing the worker's timed re-enqueue would leave an open order unsent forever on a healthy feed.
func TestOrderWorkerRetriesUncertainNonceWithoutFeedReplay(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{
		txmanager.OutcomeNonceConsumed, txmanager.OutcomeNonceConflict, txmanager.OutcomeAbandoned,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			fixture := nonceRetryFixture(t, outcome)
			solver, routes, order, txm := fixture.solver, fixture.routes, fixture.order, fixture.txm
			reg := prometheus.NewRegistry()
			var err error
			solver.metrics, err = newLIFIMetrics(reg, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			runUntilNonceRetry(t, solver, routes, order, txm)
			if len(txm.reqs) != 2 || solver.capacity.Len() != 0 {
				t.Fatalf("reconciled fill: sends=%d reservations=%d, want 2/0", len(txm.reqs), solver.capacity.Len())
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
		})
	}
}

// A failed status read at completion or on the following retry must not consume the order.
func TestOrderWorkerRetainsUncertainOrderAcrossStatusReadFailures(t *testing.T) {
	fixture := nonceRetryFixture(t, txmanager.OutcomeNonceConsumed)
	solver, routes, order, txm := fixture.solver, fixture.routes, fixture.order, fixture.txm
	reader := solver.reader.(fakeLifiReader)
	var reads atomic.Int32
	reader.statusFn = func() (uint8, error) {
		switch reads.Add(1) {
		case 3, 4:
			return 0, errors.New("status RPC unavailable")
		default:
			return lifiOrderStatusDeposited, nil
		}
	}
	solver.reader = reader
	runUntilNonceRetry(t, solver, routes, order, txm)
	if len(txm.reqs) != 2 || reads.Load() < 6 || solver.capacity.Len() != 0 {
		t.Fatalf("recovery after status failures: sends=%d reads=%d reservations=%d", len(txm.reqs), reads.Load(), solver.capacity.Len())
	}
}

// The immediate reconciliation must retire settled orders, before any retry sends another transaction.
func TestOrderWorkerReconcilesTerminalStatusAfterUncertainNonce(t *testing.T) {
	for _, status := range []uint8{lifiOrderStatusClaimed, lifiOrderStatusRefunded} {
		t.Run(strconv.Itoa(int(status)), func(t *testing.T) {
			fixture := nonceRetryFixture(t, txmanager.OutcomeNonceConsumed)
			solver, routes, order, txm := fixture.solver, fixture.routes, fixture.order, fixture.txm
			reader := solver.reader.(fakeLifiReader)
			var reads int
			reader.statusFn = func() (uint8, error) {
				reads++
				if reads > 2 {
					return status, nil
				}
				return lifiOrderStatusDeposited, nil
			}
			solver.reader = reader
			orders := make(chan *submittedOrder, 1)
			orders <- order
			close(orders)
			if err := solver.runOrderWorker(t.Context(), routes, orders, nil, nil); err != nil {
				t.Fatal(err)
			}
			if len(txm.reqs) != 1 || reads != 3 || solver.capacity.Len() != 0 {
				t.Fatalf("terminal reconciliation: sends=%d reads=%d reservations=%d, want 1/3/0", len(txm.reqs), reads, solver.capacity.Len())
			}
		})
	}
}

type nonceRetryTestSetup struct {
	solver *Solver
	routes []route
	order  *submittedOrder
	txm    *fakeLifiTxSender
}

func nonceRetryFixture(t *testing.T, outcome txmanager.Outcome) nonceRetryTestSetup {
	t.Helper()
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	txm := &fakeLifiTxSender{hold: true}
	txm.onSend = func(attempt int, result chan<- txmanager.Result) {
		if attempt == 1 {
			result <- txmanager.Result{Hash: common.HexToHash("0x1234"), Outcome: outcome, Err: nonceOutcomeError(outcome)}
			return
		}
		result <- txm.fillResult()
	}
	solver := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)
	solver.wallNow = time.Now
	order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	return nonceRetryTestSetup{solver: solver, routes: testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), order: order, txm: txm}
}

func runUntilNonceRetry(t *testing.T, solver *Solver, routes []route, order *submittedOrder, txm *fakeLifiTxSender) {
	t.Helper()
	retried := make(chan struct{})
	onSend := txm.onSend
	txm.onSend = func(attempt int, result chan<- txmanager.Result) {
		onSend(attempt, result)
		if attempt == 2 {
			close(retried)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	orders := make(chan *submittedOrder, 1)
	orders <- order
	done := make(chan error, 1)
	go func() { done <- solver.runOrderWorker(ctx, routes, orders, nil, nil) }()
	select {
	case <-retried:
	case <-time.After(3 * time.Second):
		cancel()
		<-done
		t.Fatal("open order was not retried without a feed replay")
	}
	close(orders)
	if err := <-done; err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}
}

func TestNonceRetryQueueKeepsDeadlineAndBackoffAcrossSubmissions(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	deadline := now.Add(time.Minute)
	order := &submittedOrder{OrderID: "retry"}
	queue := newOrderDepositRetryQueue(1)
	if err := queue.scheduleBefore(order, now, deadline); err != nil {
		t.Fatal(err)
	}
	firstAt, _ := queue.nextReadyAt()
	if firstAt.Sub(now) != 250*time.Millisecond {
		t.Fatalf("first retry offset = %s, want 250ms", firstAt.Sub(now))
	}
	if got, err := queue.popReady(firstAt); err != nil || got != order {
		t.Fatalf("first retry = %v/%v", got, err)
	}
	// Another signed attempt has no active timer, but its state stays tracked until settlement.
	if !queue.contains(&submittedOrder{OrderID: "retry"}) {
		t.Fatal("admitted retry lost its replay-coalescing state")
	}
	if err := queue.scheduleBefore(order, firstAt, deadline.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	secondAt, _ := queue.nextReadyAt()
	if secondAt.Sub(firstAt) != 500*time.Millisecond {
		t.Fatalf("second retry backoff = %s, want 500ms", secondAt.Sub(firstAt))
	}
	if got, err := queue.popReady(deadline); got != order || !errors.Is(err, errOrderNonceRetryExpired) {
		t.Fatalf("original deadline retry = %v/%v, want expired", got, err)
	}
	if queue.len() != 0 {
		t.Fatal("expired nonce retry remained tracked")
	}
}

func TestNonceRetryQueueContinuesBeyondDepositWindow(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "retry"}
	queue := newOrderDepositRetryQueue(1)
	if err := queue.scheduleBefore(order, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	checkAt := now.Add(31 * time.Second)
	if got, err := queue.popReady(checkAt); got != order || err != nil {
		t.Fatalf("nonce retry after deposit window = %v/%v, want ready", got, err)
	}
	if err := queue.scheduleBefore(order, checkAt, time.Time{}); err != nil {
		t.Fatalf("nonce retry discarded its deadline bound: %v", err)
	}
}

func TestSubmitFillMapsNonceRetryDeadlineFromChainTime(t *testing.T) {
	fixture := nonceRetryFixture(t, txmanager.OutcomeNonceConsumed)
	solver, order, txm := fixture.solver, fixture.order, fixture.txm
	wall := time.Unix(1_700_000_000, 0)
	chainNow := wall.Add(time.Hour)
	order.Order.Expires = uint32(chainNow.Add(2 * time.Second).Unix())
	order.Order.FillDeadline = order.Order.Expires
	solver.wallNow = func() time.Time { return wall }
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID: "route-1", CapacityID: "capacity-1",
		Adapter:  solver.reader.(fakeLifiReader).fill[0].Adapter,
		AmountIn: order.AmountIn, ExpectedAmountOut: big.NewInt(1_000_000), MinAmountOut: big.NewInt(990_000),
		ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	calldata, err := buildFillCalldata(*order, common.HexToHash("0x1"), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	fill, err := solver.submitFill(t.Context(), order, plan, calldata, nil, chainNow, wall)
	if err != nil {
		t.Fatal(err)
	}
	want := wall.Add(2 * time.Second)
	if fill == nil || !fill.retryDeadline.Equal(want) || len(txm.reqs) != 1 {
		t.Fatalf("nonce retry deadline = %+v, want %s", fill, want)
	}
	queue := newOrderDepositRetryQueue(1)
	if err := queue.scheduleBefore(order, want, fill.retryDeadline); !errors.Is(err, errOrderNonceRetryExpired) {
		t.Fatalf("retry after mapped deadline = %v, want expired", err)
	}
}

func TestOrderWorkerUncertainShutdownDoesNotReconcileStatus(t *testing.T) {
	fixture := nonceRetryFixture(t, txmanager.OutcomeNonceConsumed)
	solver, routes, order, txm := fixture.solver, fixture.routes, fixture.order, fixture.txm
	var logs []string
	solver.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
	ctx, cancel := context.WithCancel(solverContext(t, solver))
	defer cancel()
	reads := 0
	reader := solver.reader.(fakeLifiReader)
	reader.statusFn = func() (uint8, error) {
		reads++
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return lifiOrderStatusDeposited, nil
	}
	solver.reader = reader
	onSend := txm.onSend
	txm.onSend = func(attempt int, result chan<- txmanager.Result) {
		onSend(attempt, result)
		cancel()
	}
	orders := make(chan *submittedOrder, 1)
	orders <- order
	err := solver.runOrderWorker(ctx, routes, orders, nil, nil)
	if !errors.Is(err, context.Canceled) || reads != 2 || solver.capacity.Len() != 0 {
		t.Fatalf("uncertain shutdown: err=%v status reads=%d reservations=%d, want canceled/2/0", err, reads, solver.capacity.Len())
	}
	if strings.Contains(strings.Join(logs, "\n"), "reconciliation failed") {
		t.Fatalf("expected shutdown logged a status failure: %v", logs)
	}
}

func TestUncertainStatusReconciliationTreatsCancellationAsSkip(t *testing.T) {
	var logs []string
	solver := &Solver{cfg: testLifiConfig(), log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})}
	ctx, cancel := context.WithCancel(solverContext(t, solver))
	defer cancel()
	solver.reader = fakeLifiReader{statusFn: func() (uint8, error) {
		cancel()
		return 0, ctx.Err()
	}}
	fill := &pendingFill{order: &submittedOrder{OrderID: "canceled"}, orderID: common.HexToHash("0x1")}
	retry, err := solver.reconcileUncertainFill(ctx, fill)
	if retry || err != nil || strings.Contains(strings.Join(logs, "\n"), "reconciliation failed") {
		t.Fatalf("canceled status read: retry=%t err=%v logs=%v", retry, err, logs)
	}
}

func TestNonceRetryRetainsTimerWhenCapacityQueueIsFull(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	order := &submittedOrder{OrderID: "uncertain"}
	nonceRetries := newOrderDepositRetryQueue(1)
	if err := nonceRetries.scheduleBefore(order, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	firstAt := now.Add(250 * time.Millisecond)
	if got, err := nonceRetries.popReady(firstAt); got != order || err != nil {
		t.Fatalf("first retry = %v/%v, want ready", got, err)
	}
	capacityRetries := newReservationRetryQueue(1)
	if err := capacityRetries.enqueue(&submittedOrder{OrderID: "already-waiting"}, 0); err != nil {
		t.Fatal(err)
	}
	err := capacityRetries.enqueueWithNonceRetry(order, 0, nonceRetries, firstAt)
	if err != nil {
		t.Fatalf("retained capacity admission error = %v, want nil", err)
	}
	readyAt, ok := nonceRetries.nextReadyAt()
	if !ok || readyAt.Sub(firstAt) != 500*time.Millisecond {
		t.Fatalf("capacity overflow stranded nonce retry: ready=%t deadline=%s, want timer after 500ms", ok, readyAt)
	}
	if got, err := nonceRetries.popReady(readyAt); got != order || err != nil {
		t.Fatalf("overflow recovery = %v/%v, want ready", got, err)
	}
}

func TestOrderWorkerRetainsUncertainOrderAcrossUnknownStatuses(t *testing.T) {
	fixture := nonceRetryFixture(t, txmanager.OutcomeNonceConsumed)
	solver, routes, order, txm := fixture.solver, fixture.routes, fixture.order, fixture.txm
	reader := solver.reader.(fakeLifiReader)
	var reads atomic.Int32
	reader.statusFn = func() (uint8, error) {
		switch reads.Add(1) {
		case 3, 4, 6:
			return 255, nil
		default:
			return lifiOrderStatusDeposited, nil
		}
	}
	solver.reader = reader
	runUntilNonceRetry(t, solver, routes, order, txm)
	if len(txm.reqs) != 2 || reads.Load() < 8 {
		t.Fatalf("recovery after unknown statuses: sends=%d reads=%d", len(txm.reqs), reads.Load())
	}
}
