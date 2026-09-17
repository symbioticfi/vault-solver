package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/api/lifiorder"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const (
	tracingStrategyName   = "default"
	tracingOrderID        = "intent-1"
	tracingQuoteID        = "quote-1"
	tracingOnChainOrderID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func orderMessageSpan() string { return "lifi.order." + orderSubmitEvent }

// neverAdmit fails the test if a message this solver should ignore reaches queue admission.
func neverAdmit(t *testing.T) func(context.Context, *submittedOrder) error {
	t.Helper()
	return func(_ context.Context, order *submittedOrder) error {
		t.Errorf("ignored message reached admission: %+v", order)
		return nil
	}
}

// tracingOrderFixture drives one feed message from the inbox through the order worker to a
// completed transaction, the way the solver's goroutines do in production.
type tracingOrderFixture struct {
	solver *Solver
	txm    *fakeLifiTxSender
	routes []route
	raw    []byte
}

func newTracingOrderFixture(t *testing.T) *tracingOrderFixture {
	t.Helper()
	setup := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("new strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	solver := newProcessTestSolver(
		setup.cfg, setup.caller, txm, strategy,
		setup.tokenIn, setup.tokenOut, setup.adapter, lifiOrderStatusDeposited,
	)
	solver.cfg.Strategy = StrategyConfig{Name: tracingStrategyName}
	return &tracingOrderFixture{
		solver: solver,
		txm:    txm,
		routes: testResolvedRoutes(setup.tokenIn, setup.tokenOut, setup.adapter),
		raw:    testOrderJSON(t, setup.cfg, setup.tokenIn, setup.tokenOut),
	}
}

// context stands in for Solver.Run, which stores the solver logger on the context it passes down.
func (f *tracingOrderFixture) context(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), f.solver.log)
}

// admit drives one feed message through the message span into the inbox.
func (f *tracingOrderFixture) admit(t *testing.T, inbox *orderInbox) error {
	t.Helper()
	order, err := f.solver.admitOrderMessage(
		f.context(t), orderMessage{Event: orderSubmitEvent, Data: f.raw},
		func(_ context.Context, order *submittedOrder) error { return inbox.enqueue(order) },
	)
	if order == nil {
		t.Fatal("feed message was rejected")
	}
	return err
}

func (f *tracingOrderFixture) run(t *testing.T) {
	t.Helper()
	inbox := newOrderInbox(4)
	if err := f.admit(t, inbox); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	inbox.closeInput()
	orders := make(chan *submittedOrder)
	inboxDone := make(chan error, 1)
	go func() { inboxDone <- inbox.run(t.Context(), orders) }()
	if err := f.solver.runOrderWorker(f.context(t), f.routes, orders, nil, nil); err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}
	if err := <-inboxDone; err != nil {
		t.Fatalf("order inbox: %v", err)
	}
}

// TestOrderTraceSpansMessageThroughFill pins the order trace: the feed message names the order, the
// worker's processing span continues that trace across the detached work context, and the stages
// down to the transaction outcome belong to it (spec §6.4, §9.4).
func TestOrderTraceSpansMessageThroughFill(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)

	fixture.run(t)

	tracetest.RequireSpans(t, rec,
		orderMessageSpan(), "lifi.order.process", "lifi.order.plan",
		"lifi.order.submit", "lifi.order.complete",
	)
	message := tracetest.Ended(t, rec, orderMessageSpan())
	if got := tracetest.Attr(message, "solver"); got != Name {
		t.Fatalf("message span solver = %q, want %q", got, Name)
	}
	if got := tracetest.Attr(message, "order.id"); got != tracingOrderID {
		t.Fatalf("message span order.id = %q, want %q", got, tracingOrderID)
	}
	if got := tracetest.Attr(message, "order.onchain_id"); got != tracingOnChainOrderID {
		t.Fatalf("message span order.onchain_id = %q, want %q", got, tracingOnChainOrderID)
	}
	if got := tracetest.Attr(message, "quote.id"); got != tracingQuoteID {
		t.Fatalf("message span quote.id = %q, want %q", got, tracingQuoteID)
	}
	process := tracetest.Ended(t, rec, "lifi.order.process")
	if got, want := process.SpanContext().TraceID(), message.SpanContext().TraceID(); got != want {
		t.Fatalf("process span trace = %s, want the message trace %s", got, want)
	}
	if got, want := process.Parent().SpanID(), message.SpanContext().SpanID(); got != want {
		t.Fatalf("process span parent = %s, want the message span %s", got, want)
	}
	if got := tracetest.Attr(process, "order.id"); got != tracingOrderID {
		t.Fatalf("process span order.id = %q, want %q", got, tracingOrderID)
	}
	if got := tracetest.Attr(process, "quote.id"); got != tracingQuoteID {
		t.Fatalf("process span quote.id = %q, want %q", got, tracingQuoteID)
	}
	wantHash := fixture.txm.fillResult().Hash.Hex()
	if got := tracetest.Attr(process, "tx.hash"); got != wantHash {
		t.Fatalf("process span tx.hash = %q, want %s", got, wantHash)
	}
	complete := tracetest.Ended(t, rec, "lifi.order.complete")
	if got := tracetest.Attr(complete, "tx.hash"); got != wantHash {
		t.Fatalf("complete span tx.hash = %q, want %s", got, wantHash)
	}
	if got := tracetest.Attr(complete, "tx.outcome"); got != string(txmanager.OutcomeConfirmed) {
		t.Fatalf("complete span tx.outcome = %q, want confirmed", got)
	}
	if got := tracetest.Attr(tracetest.Ended(t, rec, "lifi.order.plan"), "strategy.name"); got != tracingStrategyName {
		t.Fatalf("plan span strategy.name = %q, want %q", got, tracingStrategyName)
	}
	submit := tracetest.Ended(t, rec, "lifi.order.submit")
	if got, want := submit.Parent().SpanID(), process.SpanContext().SpanID(); got != want {
		t.Fatalf("submit span parent = %s, want the process span %s", got, want)
	}
}

// An order kind this solver never fills is expected feed traffic, not a failure: the message span
// declines instead of erroring.
func TestOrderTraceDeclinesUnsupportedOrder(t *testing.T) {
	rec := tracetest.Install(t)
	cfg := testLifiConfig()
	raw := mutatedTestOrderJSON(t, cfg, func(body map[string]any) {
		mapField(t, body, "meta")["orderStatus"] = "Refunded"
	})
	solver := &Solver{cfg: cfg, chainID: 11155111, log: logr.Discard()}

	order, err := solver.admitOrderMessage(
		t.Context(), orderMessage{Event: orderSubmitEvent, Data: raw}, neverAdmit(t),
	)

	if order != nil || err != nil {
		t.Fatalf("admitOrderMessage() = %+v, %v, want an ignored order", order, err)
	}
	span := tracetest.Ended(t, rec, orderMessageSpan())
	if span.Status().Code == codes.Error {
		t.Fatalf("an unsupported order must not be an error span: %v", span.Status())
	}
	if !tracetest.HasEvent(span, "declined") {
		t.Fatalf("message span has no declined event: %v", span.Events())
	}
	if tracetest.HasEvent(span, "exception") {
		t.Fatalf("an unsupported order recorded an exception: %v", span.Events())
	}
}

// The other half of the classification: a message we could not understand is a real failure and has
// to surface as an error span with the cause recorded.
func TestOrderTraceRecordsMalformedMessage(t *testing.T) {
	rec := tracetest.Install(t)
	solver := &Solver{cfg: testLifiConfig(), chainID: 11155111, log: logr.Discard()}

	order, err := solver.admitOrderMessage(
		t.Context(), orderMessage{Event: orderSubmitEvent, Data: []byte(`{`)}, neverAdmit(t),
	)

	if order != nil || err == nil {
		t.Fatalf("admitOrderMessage() = %+v, %v, want a rejected message", order, err)
	}
	span := tracetest.Ended(t, rec, orderMessageSpan())
	if span.Status().Code != codes.Error {
		t.Fatalf("message span status = %v, want an error", span.Status())
	}
	if !tracetest.HasEvent(span, "exception") {
		t.Fatalf("malformed message recorded no exception: %v", span.Events())
	}
}

// A chain read this solver retries later is a real failure of the attempt, so the processing span
// records it rather than ending clean.
func TestOrderProcessTraceRecordsChainFailure(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	statusErr := errors.New("temporary status failure")
	fixture.solver.reader = fakeLifiReader{statusErr: statusErr}

	fixture.run(t)

	process := tracetest.Ended(t, rec, "lifi.order.process")
	if process.Status().Code != codes.Error {
		t.Fatalf("process span status = %v, want an error", process.Status())
	}
	if !tracetest.HasEvent(process, "exception") {
		t.Fatalf("failed order recorded no exception: %v", process.Events())
	}
	if !strings.Contains(process.Status().Description, statusErr.Error()) {
		t.Fatalf("process span status %q does not name the chain failure", process.Status().Description)
	}
}

// An order whose deposit has not propagated yet is retried under the same processing span: one
// process span for the order, one deposit stage span per attempt (spec §9.4).
func TestOrderDepositRetriesShareOneProcessSpan(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	fixture.solver.wallNow = time.Now
	statuses := []uint8{lifiOrderStatusNone, lifiOrderStatusNone, lifiOrderStatusDeposited, lifiOrderStatusDeposited}
	var reads atomic.Int32
	reader := fixture.solver.reader.(fakeLifiReader)
	reader.statusFn = func() (uint8, error) {
		index := min(int(reads.Add(1)-1), len(statuses)-1)
		return statuses[index], nil
	}
	fixture.solver.reader = reader
	// Intake stays open until the fill is submitted: closing it drops queued deposit retries.
	submitted := make(chan struct{}, 1)
	fixture.txm.onSend = func(int, chan<- txmanager.Result) { submitted <- struct{}{} }
	orders := make(chan *submittedOrder, 1)
	order, err := fixture.solver.admitOrderMessage(
		t.Context(), orderMessage{Event: orderSubmitEvent, Data: fixture.raw},
		func(context.Context, *submittedOrder) error { return nil },
	)
	if order == nil || err != nil {
		t.Fatalf("admitOrderMessage() = %+v, %v", order, err)
	}
	orders <- order
	done := make(chan error, 1)
	go func() { done <- fixture.solver.runOrderWorker(t.Context(), fixture.routes, orders, nil, nil) }()

	select {
	case <-submitted:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not fill after the deposit became visible")
	}
	close(orders)
	if err := <-done; err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}

	if got := len(tracetest.AllEnded(rec, "lifi.order.process")); got != 1 {
		t.Fatalf("process spans = %d, want one shared by every retry", got)
	}
	process := tracetest.Ended(t, rec, "lifi.order.process")
	deposits := tracetest.AllEnded(rec, "lifi.order.deposit")
	if len(deposits) != 2 {
		t.Fatalf("deposit stage spans = %d, want one per retry (spans %v)", len(deposits), tracetest.Names(rec))
	}
	wantAttempt := []string{"1", "2"}
	for index, deposit := range deposits {
		if got, want := deposit.Parent().SpanID(), process.SpanContext().SpanID(); got != want {
			t.Fatalf("deposit span %d parent = %s, want the process span %s", index, got, want)
		}
		if got := tracetest.Attr(deposit, "tx.attempt"); got != wantAttempt[index] {
			t.Fatalf("deposit span %d tx.attempt = %q, want %s", index, got, wantAttempt[index])
		}
	}
	if len(fixture.txm.reqs) != 1 {
		t.Fatalf("fill submissions = %d, want 1", len(fixture.txm.reqs))
	}
}

// A fill deferred by another fill's capacity re-enters planning under the same processing span, and
// the retry attempt is named on its own reserve stage span (spec §9.4).
func TestOrderCapacityRetrySpansReserveStage(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := immediateTestSetup(t)
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID:           "route-1",
		CapacityID:        "capacity-1",
		Adapter:           fixture.adapter,
		AmountIn:          big.NewInt(1_000_000),
		ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut:      big.NewInt(990_001),
		ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	inputs := make(chan types.FillInput, 8)
	submitted := make(chan chan<- txmanager.Result, 2)
	txm := &fakeLifiTxSender{
		hold:   true,
		onSend: func(_ int, result chan<- txmanager.Result) { submitted <- result },
	}
	solver := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm,
		reservationAwareFillStrategy{plan: plan, blockAtReserved: big.NewInt(1_000_000), inputs: inputs},
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	solver.cfg.Strategy = StrategyConfig{Name: tracingStrategyName}
	solver.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			return profitableFillSnapshots(fixture.tokenIn, fixture.tokenOut, fixture.adapter, 1_000_000)
		},
	}
	first := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	secondValue := *first
	secondValue.OrderID = "order-2"
	secondValue.dedupeKey = "order-2-key"
	secondValue.Order.Nonce = new(big.Int).Add(first.Order.Nonce, big.NewInt(1))
	orders := make(chan *submittedOrder, 2)
	orders <- first
	orders <- &secondValue
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- solver.runOrderWorker(
			t.Context(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders, nil, nil,
		)
	}()

	firstResult := receiveFillSubmission(t, submitted)
	// Hold the first completion until the second order has been deferred: releasing the reservation
	// any earlier lets the second order plan unblocked, and no reserve stage is ever run. The three
	// decisions are the first order's plan, the second order's blocked plan, and its unreserved probe.
	for range 3 {
		receiveFillInput(t, inputs)
	}
	firstResult <- txm.fillResult()
	secondResult := receiveFillSubmission(t, submitted)
	secondResult <- txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after both fills completed")
	}

	reserves := tracetest.AllEnded(rec, "lifi.order.reserve")
	if len(reserves) != 1 {
		t.Fatalf("reserve stage spans = %d, want one retry (spans %v)", len(reserves), tracetest.Names(rec))
	}
	if got := tracetest.Attr(reserves[0], "tx.attempt"); got != "1" {
		t.Fatalf("reserve span tx.attempt = %q, want 1", got)
	}
	if got := len(tracetest.AllEnded(rec, "lifi.order.process")); got != 2 {
		t.Fatalf("process spans = %d, want one per order", got)
	}
	var deferredProcess sdktrace.ReadOnlySpan
	for _, process := range tracetest.AllEnded(rec, "lifi.order.process") {
		if process.SpanContext().SpanID() == reserves[0].Parent().SpanID() {
			deferredProcess = process
		}
	}
	if deferredProcess == nil {
		t.Fatal("reserve stage span is not a child of a processing span")
	}
	if got := tracetest.Attr(deferredProcess, "order.id"); got != secondValue.OrderID {
		t.Fatalf("retried process span order.id = %q, want %q", got, secondValue.OrderID)
	}
}

// A replayed feed message for an order whose fill is still pending must not close the processing
// span the live copy is still writing to: the order key is shared, and the inbox stops deduping the
// moment it delivers.
func TestOrderReplayKeepsProcessSpanOpenUntilFill(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	submitted := make(chan chan<- txmanager.Result, 1)
	fixture.txm.hold = true
	fixture.txm.onSend = func(_ int, result chan<- txmanager.Result) { submitted <- result }
	// The replay declines after its own status read; the first pass reads twice (planning, submission).
	replayed := make(chan struct{}, 1)
	var reads atomic.Int32
	reader := fixture.solver.reader.(fakeLifiReader)
	reader.statusFn = func() (uint8, error) {
		if reads.Add(1) == 3 {
			replayed <- struct{}{}
		}
		return lifiOrderStatusDeposited, nil
	}
	fixture.solver.reader = reader
	order, err := fixture.solver.admitOrderMessage(
		t.Context(), orderMessage{Event: orderSubmitEvent, Data: fixture.raw},
		func(context.Context, *submittedOrder) error { return nil },
	)
	if order == nil || err != nil {
		t.Fatalf("admitOrderMessage() = %+v, %v", order, err)
	}
	orders := make(chan *submittedOrder, 2)
	orders <- order
	orders <- order // the same order delivered twice, as a feed replay or recovery sweep does
	close(orders)
	done := make(chan error, 1)
	go func() { done <- fixture.solver.runOrderWorker(t.Context(), fixture.routes, orders, nil, nil) }()

	result := receiveFillSubmission(t, submitted)
	select {
	case <-replayed:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not process the replayed order")
	}
	// The replay has been through the worker and the fill is still pending: the span the live copy
	// writes to must still be open.
	if got := len(tracetest.AllEnded(rec, "lifi.order.process")); got != 0 {
		t.Fatalf("process spans ended while the fill was pending = %d, want none (spans %v)",
			got, tracetest.Names(rec))
	}
	result <- fixture.txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after the fill completed")
	}

	if got := len(tracetest.AllEnded(rec, "lifi.order.process")); got != 1 {
		t.Fatalf("process spans = %d, want one shared by the replay (spans %v)", got, tracetest.Names(rec))
	}
	process := tracetest.Ended(t, rec, "lifi.order.process")
	complete := tracetest.Ended(t, rec, "lifi.order.complete")
	if got, want := complete.Parent().SpanID(), process.SpanContext().SpanID(); got != want {
		t.Fatalf("complete span parent = %s, want the processing span %s", got, want)
	}
	wantHash := fixture.txm.fillResult().Hash.Hex()
	if got := tracetest.Attr(process, "tx.hash"); got != wantHash {
		t.Fatalf("process span tx.hash = %q, want %s", got, wantHash)
	}
	if len(fixture.txm.reqs) != 1 {
		t.Fatalf("fill submissions = %d, want 1", len(fixture.txm.reqs))
	}
}

// A shutdown drops whatever sits in the deposit-retry queue, so the worker's finishAll is the only
// thing left to close those orders' processing spans — once each, and as a cancellation rather than
// a failure.
func TestOrderWorkerShutdownEndsQueuedDepositRetrySpan(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	// A frozen clock keeps the scheduled retry from ever becoming ready, so the order stays queued.
	now := time.Now()
	fixture.solver.wallNow = func() time.Time { return now }
	read := make(chan struct{}, 1)
	reader := fixture.solver.reader.(fakeLifiReader)
	reader.statusFn = func() (uint8, error) {
		select {
		case read <- struct{}{}:
		default:
		}
		return lifiOrderStatusNone, nil // the deposit never becomes visible
	}
	fixture.solver.reader = reader
	order, err := fixture.solver.admitOrderMessage(
		t.Context(), orderMessage{Event: orderSubmitEvent, Data: fixture.raw},
		func(context.Context, *submittedOrder) error { return nil },
	)
	if order == nil || err != nil {
		t.Fatalf("admitOrderMessage() = %+v, %v", order, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	orders := make(chan *submittedOrder, 1)
	orders <- order
	done := make(chan error, 1)
	go func() { done <- fixture.solver.runOrderWorker(ctx, fixture.routes, orders, nil, nil) }()

	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not read the order's deposit status")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runOrderWorker = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}

	processes := tracetest.AllEnded(rec, "lifi.order.process")
	if len(processes) != 1 {
		t.Fatalf("process spans = %d, want exactly one ended at shutdown (spans %v)",
			len(processes), tracetest.Names(rec))
	}
	if got := tracetest.Attr(processes[0], "order.id"); got != tracingOrderID {
		t.Fatalf("process span order.id = %q, want %q", got, tracingOrderID)
	}
	if processes[0].Status().Code == codes.Error {
		t.Fatalf("a cancelled order must not be an error span: %v", processes[0].Status())
	}
	if !tracetest.HasEvent(processes[0], "cancelled") {
		t.Fatalf("process span has no cancelled event: %v", processes[0].Events())
	}
	if len(fixture.txm.reqs) != 0 {
		t.Fatalf("fill submissions = %d, want none for an order whose deposit never landed",
			len(fixture.txm.reqs))
	}
}

// replayFillStrategy answers for one deferred order: blocked by pending capacity, then a plain
// decline when the feed replays it while it waits, then a fillable plan on the capacity retry.
type replayFillStrategy struct {
	plan     *types.FillPlan
	deferred string
	calls    map[string]int
	declined chan struct{}
}

func (replayFillStrategy) DecideQuotes(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s replayFillStrategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if input.OrderID != s.deferred {
		return s.plan, nil
	}
	s.calls[input.OrderID]++
	if s.calls[input.OrderID] == 2 {
		s.declined <- struct{}{}
	}
	if s.calls[input.OrderID] < 3 {
		return nil, nil
	}
	return s.plan, nil
}

func (s replayFillStrategy) DecideFillWithoutReservations(
	_ context.Context, input types.FillInput,
) (*types.FillPlan, error) {
	if s.calls[input.OrderID] == 1 {
		return s.plan, nil
	}
	return nil, nil
}

// The same rule on the capacity-retry path: a replay that declines while the order still sits in the
// retry queue must not close its processing span, or the retry would open a second one.
func TestOrderReplayKeepsProcessSpanOpenWhileQueuedForCapacity(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := immediateTestSetup(t)
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID:           "route-1",
		CapacityID:        "capacity-1",
		Adapter:           fixture.adapter,
		AmountIn:          big.NewInt(1_000_000),
		ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut:      big.NewInt(990_001),
		ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	submitted := make(chan chan<- txmanager.Result, 2)
	txm := &fakeLifiTxSender{
		hold:   true,
		onSend: func(_ int, result chan<- txmanager.Result) { submitted <- result },
	}
	strategy := replayFillStrategy{
		plan: plan, deferred: "order-2", calls: make(map[string]int), declined: make(chan struct{}, 1),
	}
	solver := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	solver.cfg.Strategy = StrategyConfig{Name: tracingStrategyName}
	solver.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			return profitableFillSnapshots(fixture.tokenIn, fixture.tokenOut, fixture.adapter, 1_000_000)
		},
	}
	first := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	deferredValue := *first
	deferredValue.OrderID = "order-2"
	deferredValue.dedupeKey = "order-2-key"
	deferredValue.Order.Nonce = new(big.Int).Add(first.Order.Nonce, big.NewInt(1))
	deferred := &deferredValue
	orders := make(chan *submittedOrder, 3)
	orders <- first
	orders <- deferred
	orders <- deferred // replayed while it waits on the capacity of the first fill
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- solver.runOrderWorker(
			t.Context(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders, nil, nil,
		)
	}()

	firstResult := receiveFillSubmission(t, submitted)
	select {
	case <-strategy.declined:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not process the replayed order")
	}
	firstResult <- txm.fillResult()
	receiveFillSubmission(t, submitted) <- txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after both fills completed")
	}

	var deferredSpans []sdktrace.ReadOnlySpan
	for _, process := range tracetest.AllEnded(rec, "lifi.order.process") {
		if tracetest.Attr(process, "order.id") == deferred.OrderID {
			deferredSpans = append(deferredSpans, process)
		}
	}
	if len(deferredSpans) != 1 {
		t.Fatalf("process spans for the deferred order = %d, want 1 (spans %v)",
			len(deferredSpans), tracetest.Names(rec))
	}
	if got := tracetest.Attr(deferredSpans[0], "tx.hash"); got != txm.fillResult().Hash.Hex() {
		t.Fatalf("deferred process span tx.hash = %q, want %s", got, txm.fillResult().Hash.Hex())
	}
	if got := len(tracetest.AllEnded(rec, "lifi.order.reserve")); got != 1 {
		t.Fatalf("reserve stage spans = %d, want one retry", got)
	}
}

// unsupportedContextStrategy stands in for a strategy that does not handle this order's output
// format — a permanent decision this solver logs at V(1) and deliberately keeps out of Sentry.
type unsupportedContextStrategy struct{}

func (unsupportedContextStrategy) DecideQuotes(
	context.Context, types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (unsupportedContextStrategy) DecideFill(
	context.Context, types.FillInput,
) (*types.FillPlan, error) {
	return nil, types.MarkPermanentFillDecisionError(types.ErrUnsupportedOutputContext)
}

// An output format the strategy does not handle is an expected skip, so neither the planning stage
// nor the processing span may report it as an error.
func TestOrderPlanTraceDeclinesUnsupportedOutputContext(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	fixture.solver.strategy = unsupportedContextStrategy{}

	fixture.run(t)

	plan := tracetest.Ended(t, rec, "lifi.order.plan")
	if plan.Status().Code == codes.Error {
		t.Fatalf("an unsupported output context must not be an error span: %v", plan.Status())
	}
	if !tracetest.HasEvent(plan, "declined") {
		t.Fatalf("plan span has no declined event: %v", plan.Events())
	}
	if tracetest.HasEvent(plan, "exception") {
		t.Fatalf("an unsupported output context recorded an exception: %v", plan.Events())
	}
	process := tracetest.Ended(t, rec, "lifi.order.process")
	if process.Status().Code == codes.Error {
		t.Fatalf("process span status = %v, want no error", process.Status())
	}
	if len(fixture.txm.reqs) != 0 {
		t.Fatalf("fill submissions = %d, want none", len(fixture.txm.reqs))
	}
}

// A plan whose capacity reservations are unusable is a failure we log at Error, so submission has to
// report it rather than returning a bare nil that ends the order's span clean. validateFillPlan
// normalizes every route before submission, so this is only reachable by calling submitFill directly.
func TestSubmitFillReportsRejectedPlan(t *testing.T) {
	fixture := newTracingOrderFixture(t)
	order := testSubmittedOrder(t, fixture.solver.cfg, common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"))

	fill, err := fixture.solver.submitFill(
		t.Context(), order, &types.FillPlan{}, &fillCalldata{OrderID: common.HexToHash("0x1")},
		big.NewInt(1), time.Unix(1_700_000_000, 0), time.Unix(1_700_000_000, 0),
	)

	if fill != nil {
		t.Fatalf("submitFill() fill = %+v, want none", fill)
	}
	if !errors.Is(err, errFillPlanRejected) {
		t.Fatalf("submitFill() error = %v, want %v", err, errFillPlanRejected)
	}
	if len(fixture.txm.reqs) != 0 {
		t.Fatalf("fill submissions = %d, want none", len(fixture.txm.reqs))
	}
}

// Each quote cycle roots its own trace: the strategy decision and the order-server reconcile are
// stages of it, and the order server sees the traceparent (spec §9.4).
func TestQuoteRefreshTraceReachesOrderServer(t *testing.T) {
	rec := tracetest.Install(t)
	var mu sync.Mutex
	var traceparents []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		traceparents = append(traceparents, r.Header.Get("traceparent"))
		mu.Unlock()
		var dto lifiorder.SubmitQuotesDto
		if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		ranges := 0
		for _, quote := range dto.Quotes {
			ranges += len(quote.Ranges)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "quotesAdded": ranges,
		}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer server.Close()
	solver := newTracingQuoteSolver(server.URL)

	solver.refreshQuotes(t.Context(), nil, newQuoteState(time.Second))

	tracetest.RequireSpans(t, rec, "lifi.quotes.refresh", "lifi.quotes.decide", "lifi.quotes.reconcile")
	refresh := tracetest.Ended(t, rec, "lifi.quotes.refresh")
	if refresh.Parent().IsValid() {
		t.Fatalf("quote refresh span has parent %v, want a root", refresh.Parent())
	}
	if got := tracetest.Attr(refresh, "solver"); got != Name {
		t.Fatalf("quote refresh span solver = %q, want %q", got, Name)
	}
	decide := tracetest.Ended(t, rec, "lifi.quotes.decide")
	if got, want := decide.Parent().SpanID(), refresh.SpanContext().SpanID(); got != want {
		t.Fatalf("decide span parent = %s, want the refresh span %s", got, want)
	}
	if got := tracetest.Attr(decide, "strategy.name"); got != tracingStrategyName {
		t.Fatalf("decide span strategy.name = %q, want %q", got, tracingStrategyName)
	}
	reconcile := tracetest.Ended(t, rec, "lifi.quotes.reconcile")
	if got, want := reconcile.Parent().SpanID(), refresh.SpanContext().SpanID(); got != want {
		t.Fatalf("reconcile span parent = %s, want the refresh span %s", got, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(traceparents) != 1 {
		t.Fatalf("order server requests = %d, want 1", len(traceparents))
	}
	if !strings.Contains(traceparents[0], refresh.SpanContext().TraceID().String()) {
		t.Fatalf("order server traceparent = %q, want the refresh trace %s",
			traceparents[0], refresh.SpanContext().TraceID())
	}
}

// Retiring the published curve is its own traced operation, so a suspension that has to retry is
// visible next to the refresh that preceded it.
func TestQuoteSuspendTraceWrapsReconcile(t *testing.T) {
	rec := tracetest.Install(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status": "success", "quotesAdded": 0}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer server.Close()
	solver := newTracingQuoteSolver(server.URL)
	state := newQuoteState(30 * time.Second)
	state.active = indexQuotePairs([]types.Quote{testStandingQuote(testQuoteRoute(), 1_000)})

	solver.suspendQuotes(t.Context(), state)

	suspend := tracetest.Ended(t, rec, "lifi.quotes.suspend")
	reconcile := tracetest.Ended(t, rec, "lifi.quotes.reconcile")
	if got, want := reconcile.Parent().SpanID(), suspend.SpanContext().SpanID(); got != want {
		t.Fatalf("reconcile span parent = %s, want the suspend span %s", got, want)
	}
	if len(state.active) != 0 {
		t.Fatalf("active quote pairs = %d, want none", len(state.active))
	}
}

func newTracingQuoteSolver(baseURL string) *Solver {
	cfg := testLifiConfig()
	cfg.Strategy = StrategyConfig{Name: tracingStrategyName}
	return &Solver{
		cfg:          cfg,
		reader:       fakeLifiReader{},
		strategy:     tracingQuoteStrategy{quotes: []types.Quote{testStandingQuote(testQuoteRoute(), 1_000)}},
		orders:       newOrderClient(baseURL, "test-key", 5*time.Second, 11155111),
		log:          logr.Discard(),
		now:          func(context.Context) (time.Time, error) { return time.Unix(1_700_000_000, 0), nil },
		maxFeePerGas: func(context.Context) (*big.Int, error) { return big.NewInt(1), nil },
		wallNow:      func() time.Time { return time.Unix(1_700_000_000, 0) },
		txLaneState:  alwaysReadyTransactionLane(),
	}
}

type tracingQuoteStrategy struct {
	quotes []types.Quote
}

func (s tracingQuoteStrategy) DecideQuotes(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	return types.QuoteOutput{Quotes: s.quotes}, nil
}

func (tracingQuoteStrategy) DecideFill(context.Context, types.FillInput) (*types.FillPlan, error) {
	return nil, nil
}

// Every dial carries the trace context into the handshake, so a connection can be found in the
// trace backend (spec §6.4).
func TestOrderFeedDialCarriesTraceparent(t *testing.T) {
	rec := tracetest.Install(t)
	handshake := make(chan http.Header, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case handshake <- r.Header.Clone():
		default:
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "")

	_, err := feed.watchOnce(t.Context(), orderFeedConnectionHooks{}, func(context.Context, orderMessage) {})
	if err == nil {
		t.Fatal("watchOnce returned without a disconnect error")
	}

	connect := tracetest.Ended(t, rec, "lifi.feed.connect")
	if got := tracetest.Attr(connect, "solver"); got != Name {
		t.Fatalf("connect span solver = %q, want %q", got, Name)
	}
	headers := <-handshake
	if got := headers.Get("traceparent"); !strings.Contains(got, connect.SpanContext().TraceID().String()) {
		t.Fatalf("handshake traceparent = %q, want the connect trace %s",
			got, connect.SpanContext().TraceID())
	}
}

// Trace loggers are derived from the base logger at each span-starting site, never from an
// already-derived one: re-deriving appends a second trace_id/span_id pair to every line.
func TestOrderLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	log, capture := tracetest.CaptureLogs(t, 1)
	fixture.solver.log = log

	fixture.run(t)

	lines := capture()
	if len(lines) == 0 {
		t.Fatal("no log output captured")
	}
	tracetest.RequireTraceIDsOnce(t, lines)
	var sawOrderLine bool
	for _, line := range lines {
		if strings.Contains(line, `"orderId"`) && strings.Contains(line, `"trace_id"`) {
			sawOrderLine = true
		}
	}
	if !sawOrderLine {
		t.Fatalf("no order-path line carried trace_id: %v", lines)
	}
}

// A fill the manager rejected before broadcasting has no transaction: the outcome is recorded and
// tx.hash is left off rather than stamped as the zero hash.
func TestFillCompletionOmitsTxHashWhenNotBroadcast(t *testing.T) {
	rec := tracetest.Install(t)
	solver := &Solver{log: logr.Discard()}
	fill := &pendingFill{
		order:          &submittedOrder{OrderID: tracingOrderID, QuoteID: tracingQuoteID},
		orderID:        common.HexToHash("0x1"),
		reservationKey: "order-1",
	}
	pending := &pendingFillState{byOrder: map[string]*pendingFill{"order-1": fill}}

	ctx, end := tracer.Start(t.Context(), "lifi.order.process")
	err := solver.completeFill(ctx, pending, fillCompletion{fill: fill, result: txmanager.Result{
		Outcome: txmanager.OutcomeSubmissionError,
		Err:     errors.New("insufficient funds for gas * price + value"),
	}})
	end(err)

	if err == nil {
		t.Fatal("submission error was not reported as the completion failure")
	}
	for _, name := range []string{"lifi.order.process", "lifi.order.complete"} {
		span := tracetest.Ended(t, rec, name)
		tracetest.RequireNoAttr(t, span, "tx.hash") // a transaction that never went out has none
		tracetest.RequireAttr(t, span, "tx.outcome", string(txmanager.OutcomeSubmissionError))
	}
}

// The advertised-discount skip line runs inside the quote refresh stage, so it now carries that
// stage's trace ids — once, because the context holds the base logger and Log stamps at retrieval.
func TestDiscountSkipLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	var lines []string
	s := &Solver{
		log: funcr.NewJSON(func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1}),
		discounts: &fakeDiscountClient{listed: &discounts.List{Discounts: []discounts.ListItem{
			{DiscountID: testDiscountID, Discount: "not-a-number"},
		}}},
	}

	ctx, end := tracer.Start(observability.WithLogger(t.Context(), s.log), "lifi.quotes.refresh")
	_, degraded := s.quoteDiscountInventories(ctx, nil, time.Unix(1_800_000_000, 0))
	end(nil)

	if !degraded {
		t.Fatal("malformed advertised discount was not reported as an issue")
	}
	var skipped string
	for _, line := range lines {
		if strings.Contains(line, `"private discounts: ignored"`) {
			skipped = line
		}
	}
	if skipped == "" {
		t.Fatalf("no skip line was logged: %v", lines)
	}
	if n := strings.Count(skipped, `"trace_id"`); n != 1 {
		t.Fatalf("trace_id appears %d times in %s, want once", n, skipped)
	}
	if n := strings.Count(skipped, `"span_id"`); n != 1 {
		t.Fatalf("span_id appears %d times in %s, want once", n, skipped)
	}
}

// Tracing is off by default: no provider is installed here, so every span is a no-op and the order
// must be filled exactly as it is without tracing.
func TestOrderFillsWithTracingDisabled(t *testing.T) {
	fixture := newTracingOrderFixture(t)

	fixture.run(t)

	if len(fixture.txm.reqs) != 1 {
		t.Fatalf("fill submissions = %d, want 1", len(fixture.txm.reqs))
	}
	if fixture.solver.capacity.Len() != 0 {
		t.Fatal("pending reservation was not released")
	}
}

// A retryable failure re-queues the order through the inbox for the next recovery sweep. The order
// is still referenced while it waits there, so its processing span stays open and the redelivery
// continues the same trace instead of opening a second one.
func TestOrderProcessSpanSurvivesInboxRetry(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingOrderFixture(t)
	filled := fixture.routes[0]
	var statusReads atomic.Int32
	fixture.solver.reader = fakeLifiReader{
		orderID: common.HexToHash(tracingOnChainOrderID),
		statusFn: func() (uint8, error) {
			if statusReads.Add(1) == 1 {
				return 0, errors.New("temporary status failure")
			}
			return lifiOrderStatusDeposited, nil
		},
		fill: profitableFillSnapshots(filled.TokenIn, filled.TokenOut, filled.Adapter, 1_000_000),
	}
	inbox := newOrderInbox(4)
	inbox.beginRecovery()
	defer inbox.endRecovery()
	if err := fixture.admit(t, inbox); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	marked := make(chan struct{}, 1)
	onRetryable := func(order *submittedOrder, attemptLimit int) bool {
		requeued := inbox.markRecoveryRetry(order, attemptLimit)
		marked <- struct{}{}
		return requeued
	}
	ctx, cancel := context.WithTimeout(fixture.context(t), 10*time.Second)
	defer cancel()
	orders := make(chan *submittedOrder)
	inboxDone := make(chan error, 1)
	workerDone := make(chan error, 1)
	go func() { inboxDone <- inbox.run(ctx, orders) }()
	go func() { workerDone <- fixture.solver.runOrderWorker(ctx, fixture.routes, orders, onRetryable, nil) }()

	select {
	case <-marked:
	case <-ctx.Done():
		t.Fatalf("the retryable failure was never re-queued: %v", ctx.Err())
	}
	retries := inbox.takeRecoveryRetries()
	if len(retries) != 1 {
		t.Fatalf("recovery retries = %d, want the failed order", len(retries))
	}
	// The inbox drops the delivered order's key only just after handing it to the worker, and the
	// worker can reach the retry first; re-enqueueing before that is coalesced away as a replay.
	waitForInboxDelivery(t, inbox)
	if err := inbox.enqueueWait(ctx, retries[0]); err != nil {
		t.Fatalf("re-enqueue recovery retry: %v", err)
	}
	inbox.closeInput()
	if err := <-workerDone; err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}
	if err := <-inboxDone; err != nil {
		t.Fatalf("order inbox: %v", err)
	}

	processes := tracetest.AllEnded(rec, "lifi.order.process")
	if len(processes) != 1 {
		t.Fatalf("process spans = %d, want one across the retry (spans %v)", len(processes), tracetest.Names(rec))
	}
	process := processes[0]
	if process.Status().Code == codes.Error {
		t.Fatalf("a retried order that then filled must not be an error span: %v", process.Status())
	}
	for _, stage := range []string{"lifi.order.plan", "lifi.order.submit"} {
		span := tracetest.Ended(t, rec, stage)
		if got, want := span.Parent().SpanID(), process.SpanContext().SpanID(); got != want {
			t.Fatalf("%s span parent = %s, want the process span %s", stage, got, want)
		}
	}
}

// waitForInboxDelivery blocks until the inbox has finished handing every queued order to the worker.
func waitForInboxDelivery(t *testing.T, inbox *orderInbox) {
	t.Helper()
	for range 2000 {
		inbox.mu.Lock()
		delivered := len(inbox.queued) == 0
		inbox.mu.Unlock()
		if delivered {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("inbox did not finish delivering the order")
}
