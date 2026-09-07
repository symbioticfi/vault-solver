package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"

	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/webhook"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func TestOrderInboxDoesNotBlockAndPreservesOrder(t *testing.T) {
	const count = 5_000
	inbox := newOrderInbox(count)

	enqueued := make(chan struct{})
	go func() {
		for i := range count {
			if err := inbox.enqueue(&submittedOrder{OrderID: strconv.Itoa(i)}); err != nil {
				t.Errorf("enqueue %d: %v", i, err)
				return
			}
		}
		close(enqueued)
	}()
	testcheck.ReceiveWithin(t, enqueued, time.Second, "enqueue blocked without a consumer")

	orders := make(chan *submittedOrder)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- inbox.run(ctx, orders) }()
	for i := range count {
		order := <-orders
		if order.OrderID != strconv.Itoa(i) {
			t.Fatalf("order %d = %s", i, order.OrderID)
		}
	}
	cancel()
	testcheck.ReceiveWithin(t, done, time.Second, "inbox did not stop after cancellation")
}

func TestParseOrderMessageLogsIgnoredOrders(t *testing.T) {
	foreignSettler := common.HexToAddress("0x008C3800F3Ad9b3B662d002E90Cc00000000eE17")
	type testCase struct {
		name         string
		mutate       func(*testing.T, map[string]any)
		want, absent []string
	}
	tests := []testCase{
		{
			name: "foreign chain",
			mutate: func(t *testing.T, body map[string]any) {
				t.Helper()
				order := mapField(t, body, "order")
				order["originChainId"], order["inputOracle"] = "1", foreignSettler.Hex()
				output := sliceField(t, order, "outputs")[0].(map[string]any)
				output["chainId"], output["oracle"], output["settler"] = "1", hexID(foreignSettler), hexID(foreignSettler)
			},
			want:   []string{"order feed: ignored order for another chain", "order is for a different chain"},
			absent: []string{`"error"`},
		},
		{
			name: "target mismatch",
			mutate: func(t *testing.T, body map[string]any) {
				t.Helper()
				mapField(t, body, "order")["inputOracle"] = foreignSettler.Hex()
			},
			want:   []string{"order feed: ignored order", "does not match outputSettler", `"error"`},
			absent: []string{"another chain"},
		},
	}
	for _, contextType := range []byte{dutchAuctionContextType, exclusiveDutchAuctionContextType} {
		tests = append(tests, testCase{
			name: hexutil.Encode([]byte{contextType}),
			mutate: func(t *testing.T, body map[string]any) {
				t.Helper()
				output := sliceField(t, mapField(t, body, "order"), "outputs")[0].(map[string]any)
				output["context"] = hexutil.Encode([]byte{contextType})
			},
			want: []string{"ignored unsupported Dutch auction", hexutil.Encode([]byte{contextType})},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testLifiConfig()
			raw := mutatedTestOrderJSON(t, cfg, func(body map[string]any) { tt.mutate(t, body) })
			var logs []string
			solver := &Solver{cfg: cfg, chainID: 11155111,
				log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
			}
			if order := solver.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw}); order != nil {
				t.Fatalf("parseOrderMessage() = %+v, want ignored order", order)
			}
			logged := strings.Join(logs, "\n")
			for _, want := range tt.want {
				if !strings.Contains(logged, want) {
					t.Errorf("log missing %q: %s", want, logged)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(logged, absent) {
					t.Errorf("log contains %q: %s", absent, logged)
				}
			}
		})
	}
}

func TestOrderInboxCoalescesQueuedReplay(t *testing.T) {
	inbox := newOrderInbox(2)
	first := &submittedOrder{OrderID: "api-1", OnChainOrderID: "chain-1"}
	testcheck.NoError(t, inbox.enqueue(first))
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: "api-2", OnChainOrderID: "chain-1"}))
	if len(inbox.orders) != 1 {
		t.Fatalf("queued orders = %d, want 1", len(inbox.orders))
	}
}

func TestOrderInboxRecoveryCoalescesDrainedReplay(t *testing.T) {
	inbox := newOrderInbox(2)
	inbox.beginRecovery()
	ctx, cancel := context.WithCancel(t.Context())
	orders := make(chan *submittedOrder)
	done := make(chan error, 1)
	go func() { done <- inbox.run(ctx, orders) }()

	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OnChainOrderID: " 0xAbCd "}))
	if order := <-orders; order.OnChainOrderID != " 0xAbCd " {
		t.Fatalf("order = %+v", order)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("inbox.run() error = %v", err)
	}

	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OnChainOrderID: "0xabcd"}))
	if len(inbox.orders) != 0 {
		t.Fatalf("REST replay was re-enqueued after live copy drained: %+v", inbox.orders)
	}
	inbox.endRecovery()
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OnChainOrderID: "0xabcd"}))
	if len(inbox.orders) != 1 {
		t.Fatalf("order was not admitted after recovery ended: %+v", inbox.orders)
	}
}

func TestOrderInboxBoundsRecoveryDedupe(t *testing.T) {
	inbox := newOrderInbox(orderRecoverySeenCapacity + 1)
	inbox.beginRecovery()
	for index := 0; index <= orderRecoverySeenCapacity; index++ {
		if err := inbox.enqueue(&submittedOrder{OrderID: strconv.Itoa(index)}); err != nil {
			t.Fatalf("enqueue %d: %v", index, err)
		}
	}
	if inbox.seen.Len() != orderRecoverySeenCapacity {
		t.Fatalf("recovery seen keys = %d, want %d", inbox.seen.Len(), orderRecoverySeenCapacity)
	}
	if inbox.entries["0"] != nil && inbox.entries["0"].seen != nil {
		t.Fatal("oldest recovery key was not evicted")
	}
	if inbox.entries[strconv.Itoa(orderRecoverySeenCapacity)] == nil || inbox.entries[strconv.Itoa(orderRecoverySeenCapacity)].seen == nil {
		t.Fatal("newest recovery key is missing")
	}
	inbox.endRecovery()
}

func TestOrderInboxPreservesRecoveryEvictionOrderAfterCompaction(t *testing.T) {
	inbox := newOrderInbox(orderRecoverySeenCapacity + 2)
	inbox.beginRecovery()
	defer inbox.endRecovery()

	for index := 0; index <= orderRecoverySeenCapacity; index++ {
		if err := inbox.enqueue(&submittedOrder{OrderID: strconv.Itoa(index)}); err != nil {
			t.Fatalf("enqueue %d: %v", index, err)
		}
	}
	if inbox.seen.Front().Value != "1" {
		t.Fatal("oldest recovery key was not evicted")
	}

	if retries := inbox.takeRecoveryRetries(); len(retries) != 0 {
		t.Fatalf("recovery retries = %d, want 0", len(retries))
	}
	newest := strconv.Itoa(orderRecoverySeenCapacity + 1)
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: newest}), "enqueue newest: %v")
	if inbox.entries["1"] != nil && inbox.entries["1"].seen != nil {
		t.Fatal("oldest recovery key was not evicted after compaction")
	}
	if (inbox.entries[strconv.Itoa(orderRecoverySeenCapacity)] == nil || inbox.entries[strconv.Itoa(orderRecoverySeenCapacity)].seen == nil) || (inbox.entries[newest] == nil || inbox.entries[newest].seen == nil) {
		t.Fatal("compaction evicted a newer recovery key")
	}
}

func TestOrderInboxRecoveryBarrierBackpressuresUntilWorker(t *testing.T) {
	inbox := newOrderInbox(1)
	testcheck.NoError(t, inbox.enqueueWait(t.Context(), &submittedOrder{OrderID: "first"}))
	barrierDone := make(chan error, 1)
	go func() {
		_, err := inbox.waitUntilProcessed(t.Context())
		barrierDone <- err
	}()
	select {
	case err := <-barrierDone:
		t.Fatalf("barrier passed full inbox before worker started: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	ctx, cancel := context.WithCancel(t.Context())
	orders := make(chan *submittedOrder)
	runDone := make(chan error, 1)
	go func() { runDone <- inbox.run(ctx, orders) }()
	if order := <-orders; order.OrderID != "first" {
		t.Fatalf("first order = %+v", order)
	}
	barrier := <-orders
	if barrier.processed == nil {
		t.Fatalf("second work item is not a barrier: %+v", barrier)
	}
	close(barrier.processed)
	testcheck.NoError(t, <-barrierDone, "waitUntilProcessed: %v")
	cancel()
	if err := <-runDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("inbox.run() error = %v", err)
	}
}

func TestOrderInboxRejectsOverflow(t *testing.T) {
	inbox := newOrderInbox(1)
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: "first"}))
	if err := inbox.enqueue(&submittedOrder{OrderID: "second"}); !errors.Is(err, errOrderInboxFull) {
		t.Fatalf("enqueue error = %v, want %v", err, errOrderInboxFull)
	}
}

func TestOrderInboxCloseDrainsQueuedOrders(t *testing.T) {
	inbox := newOrderInbox(2)
	first := &submittedOrder{OrderID: "first"}
	second := &submittedOrder{OrderID: "second"}
	testcheck.NoError(t, inbox.enqueue(first), "enqueue first: %v")
	testcheck.NoError(t, inbox.enqueue(second), "enqueue second: %v")
	inbox.closeInput()
	if err := inbox.enqueue(&submittedOrder{OrderID: "late"}); !errors.Is(err, errOrderInboxClosed) {
		t.Fatalf("enqueue after close error = %v, want %v", err, errOrderInboxClosed)
	}

	out := make(chan *submittedOrder)
	done := make(chan error, 1)
	go func() { done <- inbox.run(t.Context(), out) }()
	if got := <-out; got != first {
		t.Fatalf("first drained order = %v, want first", got)
	}
	if got := <-out; got != second {
		t.Fatalf("second drained order = %v, want second", got)
	}
	if _, ok := <-out; ok {
		t.Fatal("order output remained open after drain")
	}
	testcheck.NoError(t, <-done, "order inbox drain: %v")
}

func TestOrderInboxRecoveryOverflowRequiresAnotherSweep(t *testing.T) {
	inbox := newOrderInbox(1)
	inbox.beginRecovery()
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: "first"}))
	if err := inbox.enqueue(&submittedOrder{OrderID: "dropped"}); !errors.Is(err, errOrderInboxFull) {
		t.Fatalf("enqueue overflow error = %v", err)
	}
	if inbox.tryEndRecovery(inbox.recoveryGen) {
		t.Fatal("recovery ended despite an inbox overflow")
	}
	if !inbox.tryEndRecovery(inbox.recoveryGen) {
		t.Fatal("recovery did not end after an overflow-free sweep")
	}
}

func TestOrderInboxRecoveryGenerationRejectsPostBarrierEnqueue(t *testing.T) {
	inbox := newOrderInbox(4)
	inbox.beginRecovery()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	orders := make(chan *submittedOrder)
	done := make(chan error, 1)
	go func() { done <- inbox.run(ctx, orders) }()
	go func() {
		for order := range orders {
			if order.processed != nil {
				close(order.processed)
			}
		}
	}()

	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: "before-barrier"}))
	processedGen, err := inbox.waitUntilProcessed(t.Context())
	testcheck.NoError(t, err)
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: "after-barrier"}))
	if inbox.tryEndRecovery(processedGen) {
		t.Fatal("recovery ended after an order was enqueued behind the processed barrier")
	}
	processedGen, err = inbox.waitUntilProcessed(t.Context())
	testcheck.NoError(t, err)
	if !inbox.tryEndRecovery(processedGen) {
		t.Fatal("recovery did not end after the later order passed a new barrier")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("inbox.run() error = %v", err)
	}
}

func TestOrderInboxBoundsLimitedRecoveryRetriesAcrossOrderInstances(t *testing.T) {
	inbox := newOrderInbox(1)
	inbox.beginRecovery()
	defer inbox.endRecovery()

	for attempt := 1; attempt <= maximumStrategyRecoveryAttempts; attempt++ {
		order := &submittedOrder{OrderID: "poisoned"}
		inbox.markRecoveryRetry(order, maximumStrategyRecoveryAttempts)
		retries := inbox.takeRecoveryRetries()
		if attempt < maximumStrategyRecoveryAttempts {
			if len(retries) != 1 || retries[0] != order {
				t.Fatalf("attempt %d retries = %+v, want current order", attempt, retries)
			}
			continue
		}
		if len(retries) != 0 {
			t.Fatalf("attempt %d retries = %+v, want exhausted budget", attempt, retries)
		}
	}

	inbox.markRecoveryRetry(&submittedOrder{OrderID: "poisoned"}, maximumStrategyRecoveryAttempts)
	if retries := inbox.takeRecoveryRetries(); len(retries) != 0 {
		t.Fatalf("replacement order retries = %+v, want budget retained by order key", retries)
	}

	for attempt := 1; attempt <= maximumStrategyRecoveryAttempts+1; attempt++ {
		order := &submittedOrder{OrderID: "chain-read-failure"}
		inbox.markRecoveryRetry(order, 0)
		retries := inbox.takeRecoveryRetries()
		if len(retries) != 1 || retries[0] != order {
			t.Fatalf("unlimited attempt %d retries = %+v, want current order", attempt, retries)
		}
	}
}

func TestReservationRetryQueueIsBoundedFIFO(t *testing.T) {
	retries := newOrderRetries(2, 1)
	first := &submittedOrder{OrderID: "first"}
	second := &submittedOrder{OrderID: "second"}
	testcheck.NoError(t, retries.enqueueCapacity(first, 0))
	if err := retries.enqueueCapacity(first, 0); err != nil || len(retries.capacity) != 1 {
		t.Fatalf("duplicate enqueue: len=%d err=%v", len(retries.capacity), err)
	}
	testcheck.NoError(t, retries.enqueueCapacity(second, 1))
	if err := retries.enqueueCapacity(&submittedOrder{OrderID: "dropped-newest"}, 1); !errors.Is(err, errOrderRetryFull) {
		t.Fatalf("overflow error = %v, want %v", err, errOrderRetryFull)
	}
	if order := retries.popCapacity(0); order != nil {
		t.Fatalf("retry before reservation change = %+v", order)
	}
	if order := retries.popCapacity(1); order != first {
		t.Fatalf("first ready retry = %+v, want first", order)
	}
	if order := retries.popCapacity(1); order != nil {
		t.Fatalf("second retry ran in its enqueue generation: %+v", order)
	}
	if order := retries.popCapacity(2); order != second {
		t.Fatalf("second ready retry = %+v, want second", order)
	}
}

func TestOrderWorkerRecoveryBarrierFollowsCapacityReservation(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	testcheck.NoError(t, err, "New strategy: %v")
	txm := &fakeLifiTxSender{hold: true}
	solver := fixture.solver(txm, strategy, lifiOrderStatusDeposited)
	barrier := &submittedOrder{processed: make(chan struct{})}
	orders := make(chan *submittedOrder, 2)
	orders <- testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	orders <- barrier
	close(orders)
	inputDrained := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- solver.runOrderWorker(
			t.Context(),
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			nil,
			inputDrained,
		)
	}()

	testcheck.ReceiveWithin(t, barrier.processed, 3*time.Second, "worker did not acknowledge recovery barrier")
	testcheck.ReceiveWithin(t, inputDrained, 3*time.Second, "worker did not acknowledge the drained input")
	if reservations := solver.capacity.Snapshot(); len(reservations) == 0 {
		t.Fatal("recovery barrier passed before accepted fill reserved capacity")
	}
	if len(txm.results) != 1 {
		t.Fatalf("pending transactions = %d, want 1", len(txm.results))
	}
	txm.results[0] <- txm.fillResult()
	{
		err := testcheck.ReceiveWithin(t, done, 3*time.Second, "worker did not stop after pending fill completed")
		testcheck.NoError(t, err, "runOrderWorker: %v")
	}
}

func TestOrderWorkerMarksTransientFailureForRecovery(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	testcheck.NoError(t, err, "New strategy: %v")
	solver := fixture.solver(&fakeLifiTxSender{}, strategy, lifiOrderStatusDeposited)
	solver.reader = fakeLifiReader{statusErr: errors.New("temporary status failure")}
	order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	orders := make(chan *submittedOrder, 1)
	orders <- order
	close(orders)
	type markedRecovery struct {
		order        *submittedOrder
		attemptLimit int
	}
	marked := make(chan markedRecovery, 1)

	testcheck.NoError(t, solver.runOrderWorker(t.Context(), nil, orders, func(got *submittedOrder, attemptLimit int) {
		marked <- markedRecovery{order: got, attemptLimit: attemptLimit}
	}, nil), "runOrderWorker: %v")
	select {
	case got := <-marked:
		if got.order != order {
			t.Fatalf("marked order = %p, want %p", got.order, order)
		}
		if got.attemptLimit != 0 {
			t.Fatalf("recovery attempt limit = %d, want unlimited", got.attemptLimit)
		}
	default:
		t.Fatal("transient worker failure was not returned to recovery")
	}
}

func TestOrderWorkerRetriesDepositPropagation(t *testing.T) {
	for _, test := range []struct {
		name                string
		statuses            []uint8
		copies              int
		wantDepositDeferred float64
	}{
		{
			name: "before planning",
			statuses: []uint8{
				lifiOrderStatusNone,
				lifiOrderStatusNone,
				lifiOrderStatusDeposited,
				lifiOrderStatusDeposited,
			},
			wantDepositDeferred: 2,
		},
		{
			name: "before submission",
			statuses: []uint8{
				lifiOrderStatusDeposited,
				lifiOrderStatusNone,
				lifiOrderStatusDeposited,
				lifiOrderStatusDeposited,
			},
			wantDepositDeferred: 1,
		},
		{
			name: "duplicate while waiting", copies: 2,
			statuses:            []uint8{lifiOrderStatusNone, lifiOrderStatusDeposited, lifiOrderStatusDeposited},
			wantDepositDeferred: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := immediateTestSetup(t)
			strategy, err := defaultstrategy.New(defaultstrategy.Config{})
			testcheck.NoError(t, err, "New strategy: %v")
			submitted := make(chan struct{}, 1)
			txm := &fakeLifiTxSender{onSend: func(int, chan<- txmanager.Result) { submitted <- struct{}{} }}
			solver := fixture.solver(txm, strategy, test.statuses[0])
			reg := prometheus.NewRegistry()
			metrics, err := newLIFIMetrics(reg, nil, "")
			testcheck.NoError(t, err, "newLIFIMetrics: %v")
			solver.metrics = metrics
			solver.wallNow = time.Now
			var statusReads atomic.Int32
			reader := solver.reader.(fakeLifiReader)
			reader.statusFn = func() (uint8, error) {
				index := int(statusReads.Add(1) - 1)
				return test.statuses[index], nil
			}
			solver.reader = reader
			orders := make(chan *submittedOrder, max(1, test.copies))
			order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
			for range cap(orders) {
				orders <- order
			}
			done := make(chan error, 1)
			go func() {
				done <- solver.runOrderWorker(
					t.Context(),
					testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
					orders,
					nil,
					nil,
				)
			}()

			testcheck.ReceiveWithin(t, submitted, 3*time.Second, "worker did not fill after deposit became visible")
			close(orders)
			testcheck.NoError(t, <-done, "runOrderWorker: %v")
			if got, want := statusReads.Load(), int32(len(test.statuses)); got != want {
				t.Fatalf("status reads = %d, want %d", got, want)
			}
			if len(txm.reqs) != 1 {
				t.Fatalf("fill submissions = %d, want 1", len(txm.reqs))
			}
			wantByOutcome := map[string]float64{
				string(orderProcessingDepositDeferred): test.wantDepositDeferred,
				string(orderProcessingSubmitted):       1,
			}
			for _, outcome := range orderProcessingOutcomes {
				metricstest.RequireWorkflowEventCount(
					t, reg, Name, "order_processing", string(outcome), wantByOutcome[string(outcome)],
				)
			}
		})
	}
}

func TestOrderWorkerMetersDepositRetryExpiryFromTimer(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	testcheck.NoError(t, err, "New strategy: %v")
	solver := fixture.solver(&fakeLifiTxSender{}, strategy, lifiOrderStatusNone)
	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, nil, "")
	testcheck.NoError(t, err)
	solver.metrics = metrics
	expired := make(chan struct{}, 1)
	solver.log = funcr.NewJSON(func(entry string) {
		if strings.Contains(entry, "deposit did not become visible within retry bounds") {
			select {
			case expired <- struct{}{}:
			default:
			}
		}
	}, funcr.Options{})
	base := time.Now()
	var wallCalls atomic.Int32
	solver.wallNow = func() time.Time {
		if wallCalls.Add(1) == 1 {
			return base
		}
		return base.Add(maximumOrderDepositRetryWindow + time.Second)
	}
	order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	order.Order.Expires = uint32(base.Add(time.Hour).Unix())
	order.Order.FillDeadline = uint32(base.Add(time.Hour).Unix())
	orders := make(chan *submittedOrder, 1)
	orders <- order
	done := make(chan error, 1)
	go func() { done <- solver.runOrderWorker(t.Context(), nil, orders, nil, nil) }()
	testcheck.ReceiveWithin(t, expired, time.Second, "deposit retry did not expire from the timer path")
	close(orders)
	testcheck.NoError(t, <-done, "runOrderWorker: %v")
	metricstest.RequireWorkflowEventCount(
		t, reg, Name, "order_processing", string(orderProcessingDepositDeferred), 1,
	)
	metricstest.RequireWorkflowEventCount(
		t, reg, Name, "order_processing", string(orderProcessingNotActionable), 1,
	)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "queue_drop", string(orderQueueDepositRetry), 1)
}

func TestOrderWorkerProcessesLaterOrdersWhileWaitingForDeposit(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	testcheck.NoError(t, err, "New strategy: %v")
	submitted := make(chan struct{}, 1)
	txm := &fakeLifiTxSender{onSend: func(int, chan<- txmanager.Result) { submitted <- struct{}{} }}
	solver := fixture.solver(txm, strategy, lifiOrderStatusNone)
	solver.wallNow = time.Now
	first := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	first.OrderID = "waiting"
	first.OnChainOrderID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	first.dedupeKey = "waiting-key"
	secondValue := *first
	secondValue.OrderID = "ready"
	secondValue.OnChainOrderID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	secondValue.dedupeKey = "ready-key"
	secondValue.Order.Nonce = new(big.Int).Add(first.Order.Nonce, big.NewInt(1))
	second := &secondValue
	firstOrderID := common.BigToHash(first.Order.Nonce)
	secondOrderID := common.BigToHash(second.Order.Nonce)
	var firstReads atomic.Int32
	var secondReads atomic.Int32
	var unexpectedStatus atomic.Bool
	reader := solver.reader.(fakeLifiReader)
	reader.orderIDFn = func(order inputsettler.StandardOrder) common.Hash {
		return common.BigToHash(order.Nonce)
	}
	reader.statusForOrderFn = func(orderID common.Hash) (uint8, error) {
		switch orderID {
		case firstOrderID:
			firstReads.Add(1)
			return lifiOrderStatusNone, nil
		case secondOrderID:
			secondReads.Add(1)
			return lifiOrderStatusDeposited, nil
		default:
			unexpectedStatus.Store(true)
			return 255, nil
		}
	}
	solver.reader = reader
	orders := make(chan *submittedOrder, 2)
	orders <- first
	orders <- second
	done := make(chan error, 1)
	go func() {
		done <- solver.runOrderWorker(
			t.Context(),
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			nil,
			nil,
		)
	}()

	filled := false
	select {
	case <-submitted:
		filled = true
	case <-time.After(3 * time.Second):
	}
	close(orders)
	testcheck.NoError(t, <-done, "runOrderWorker: %v")
	if !filled {
		t.Fatalf("later deposited order was blocked by the delayed retry: first reads=%d second reads=%d txs=%d",
			firstReads.Load(), secondReads.Load(), len(txm.reqs))
	}
	if got := firstReads.Load(); got != 1 {
		t.Fatalf("waiting order status reads before intake close = %d, want 1", got)
	}
	if got := secondReads.Load(); got != 2 {
		t.Fatalf("ready order status reads = %d, want planning and submission reads", got)
	}
	if len(txm.reqs) != 1 {
		t.Fatalf("fill submissions = %d, want 1", len(txm.reqs))
	}
	if unexpectedStatus.Load() {
		t.Fatal("worker read status for an unexpected order id")
	}
}

func TestOrderWorkerDepositRetryDoesNotHoldRecoveryBarrier(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	testcheck.NoError(t, err, "New strategy: %v")
	solver := fixture.solver(&fakeLifiTxSender{}, strategy, lifiOrderStatusNone)
	solver.wallNow = time.Now
	barrier := &submittedOrder{processed: make(chan struct{})}
	orders := make(chan *submittedOrder, 2)
	orders <- testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	orders <- barrier
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- solver.runOrderWorker(
			ctx,
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			nil,
			nil,
		)
	}()

	testcheck.ReceiveWithin(t, barrier.processed, time.Second, "deposit propagation retry held the recovery barrier")
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runOrderWorker error = %v, want context cancellation", err)
	}
}

func TestOrderWorkerDropsDepositRetriesWhenIntakeStops(t *testing.T) {
	for _, test := range []struct {
		name   string
		cancel bool
	}{
		{name: "closed intake"},
		{name: "canceled context", cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := immediateTestSetup(t)
			strategy, err := defaultstrategy.New(defaultstrategy.Config{})
			testcheck.NoError(t, err, "New strategy: %v")
			txm := &fakeLifiTxSender{}
			solver := fixture.solver(txm, strategy, lifiOrderStatusNone)
			solver.wallNow = time.Now
			statusRead := make(chan struct{}, 1)
			reader := solver.reader.(fakeLifiReader)
			reader.statusFn = func() (uint8, error) {
				statusRead <- struct{}{}
				return lifiOrderStatusNone, nil
			}
			solver.reader = reader
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			orders := make(chan *submittedOrder, 1)
			orders <- testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
			if !test.cancel {
				close(orders)
			}
			done := make(chan error, 1)
			go func() {
				done <- solver.runOrderWorker(
					ctx,
					testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
					orders,
					nil,
					nil,
				)
			}()
			testcheck.ReceiveWithin(t, statusRead, time.Second, "worker did not read the initial order status")
			if test.cancel {
				cancel()
			}
			{
				err := testcheck.ReceiveWithin(t, done, time.Second, "worker retained a deposit retry after intake stopped")
				if test.cancel && !errors.Is(err, context.Canceled) {
					t.Fatalf("runOrderWorker error = %v, want context cancellation", err)
				}
				if !test.cancel && err != nil {
					t.Fatalf("runOrderWorker: %v", err)
				}
			}
			if len(txm.reqs) != 0 {
				t.Fatalf("fill submissions = %d, want 0", len(txm.reqs))
			}
		})
	}
}

func TestOrderRecoveryBoundsPersistentWebhookDecodeFailure(t *testing.T) {
	var webhookAttempts atomic.Int32
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/decide-fill" {
			http.NotFound(w, r)
			return
		}
		webhookAttempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{`))
	}))
	defer webhookServer.Close()
	client, err := webhook.NewClient(webhook.Config{URL: webhookServer.URL, Timeout: time.Second})
	testcheck.NoError(t, err, "NewClient: %v")

	fixture := immediateTestSetup(t)
	recoveredOrder := testListedOrderJSON(
		t,
		fixture.cfg,
		fixture.tokenIn,
		fixture.tokenOut,
		orderStatusSigned,
	)
	orderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var orders []json.RawMessage
		if r.URL.Query().Get("status") == orderStatusSigned {
			orders = []json.RawMessage{recoveredOrder}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(testListedOrdersPageJSON(t, orders, len(orders), 0))
	}))
	defer orderServer.Close()
	solver := fixture.solver(&fakeLifiTxSender{}, webhookstrategy.New(client), lifiOrderStatusDeposited)
	solver.orders = newOrderClient(orderServer.URL, "test-key", time.Second, 11155111)
	operationReg := prometheus.NewRegistry()
	operationMetrics, err := newLIFIMetrics(operationReg, nil, "")
	testcheck.NoError(t, err)
	solver.metrics = operationMetrics
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	inbox := newOrderInbox(2)
	inbox.beginRecovery()
	orders := make(chan *submittedOrder)
	inboxDone := make(chan error, 1)
	workerDone := make(chan error, 1)
	go func() { inboxDone <- inbox.run(ctx, orders) }()
	go func() {
		workerDone <- solver.runOrderWorker(
			ctx,
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			inbox.markRecoveryRetry,
			nil,
		)
	}()

	if !solver.recoverOrdersUntilSuccess(ctx, inbox) {
		t.Fatalf("persistent webhook decode failure prevented recovery from completing: %v", ctx.Err())
	}
	if got := webhookAttempts.Load(); got != maximumStrategyRecoveryAttempts {
		t.Fatalf("webhook fill attempts = %d, want bounded total %d", got, maximumStrategyRecoveryAttempts)
	}
	inbox.closeInput()
	testcheck.NoError(t, <-inboxDone, "inbox.run: %v")
	testcheck.NoError(t, <-workerDone, "runOrderWorker: %v")
	metricstest.RequireExternalOperationCount(
		t, operationReg, Name, orderRecoveryOperation, "success", 1,
	)
}

func TestAwaitFillTreatsClosedResultChannelAsFailure(t *testing.T) {
	results := make(chan txmanager.Result)
	close(results)
	fill := &pendingFill{result: results}
	completions := make(chan fillCompletion, 1)

	awaitFill(fill, completions)
	completion := <-completions
	if completion.result.Err == nil {
		t.Fatal("closed transaction result channel was treated as a successful fill")
	}
}

func TestOrderRecoveryRetriesAndSweepsUntilStable(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requests.Add(1)
		if request == 1 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		var orders []json.RawMessage
		if r.URL.Query().Get("status") == orderStatusSigned {
			order := testListedOrderJSON(t, cfg, tokenIn, tokenOut, orderStatusSigned)
			orders = []json.RawMessage{order}
			if request >= 4 {
				orders = append(orders, json.RawMessage(strings.Replace(
					string(order),
					`"nonce":"7"`,
					`"nonce":"8"`,
					1,
				)))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(testListedOrdersPageJSON(t, orders, len(orders), 0))
	}))
	defer server.Close()

	solver := &Solver{
		cfg:     cfg,
		chainID: 11155111,
		orders:  newOrderClient(server.URL, "test-key", time.Second, 11155111),
		log:     logr.Discard(),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	inbox := newOrderInbox(4)
	inbox.beginRecovery()
	defer inbox.endRecovery()
	orders := make(chan *submittedOrder)
	go func() { _ = inbox.run(ctx, orders) }()
	go func() {
		for order := range orders {
			if order.processed != nil {
				close(order.processed)
			}
		}
	}()
	recovered := make(chan struct{})
	go func() {
		if solver.recoverOrdersUntilSuccess(ctx, inbox) {
			close(recovered)
		}
	}()

	select {
	case <-recovered:
	case <-ctx.Done():
		t.Fatalf("recovery did not retry successfully: %v", ctx.Err())
	}
	if got := requests.Load(); got != 7 {
		t.Fatalf("GET /orders requests = %d, want failure plus three converging sweeps", got)
	}
}

func TestOrderRecoveryRetriesLiveWorkerFailureBeforeReady(t *testing.T) {
	cfg := testLifiConfig()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(testListedOrdersPageJSON(t, nil, 0, 0))
	}))
	defer server.Close()

	solver := &Solver{
		cfg: cfg, chainID: 11155111,
		orders: newOrderClient(server.URL, "test-key", time.Second, 11155111),
		log:    logr.Discard(),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	inbox := newOrderInbox(4)
	inbox.beginRecovery()
	defer inbox.endRecovery()
	orders := make(chan *submittedOrder)
	go func() { _ = inbox.run(ctx, orders) }()
	var attempts atomic.Int32
	go func() {
		for order := range orders {
			if order.processed != nil {
				close(order.processed)
				continue
			}
			if attempts.Add(1) == 1 {
				inbox.markRecoveryRetry(order, 0)
			}
		}
	}()
	testcheck.NoError(t, inbox.enqueue(&submittedOrder{OrderID: "live-only"}), "enqueue live order: %v")

	if !solver.recoverOrdersUntilSuccess(ctx, inbox) {
		t.Fatalf("recovery did not converge: %v", ctx.Err())
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("worker attempts = %d, want retained retry for the live-only order", got)
	}
	if got := requests.Load(); got < 4 {
		t.Fatalf("GET /orders requests = %d, want at least two empty sweeps", got)
	}
}

func TestCompleteFillTreatsIncludedTransactionAsSuccess(t *testing.T) {
	var logs []string
	solver := &Solver{
		log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}
	fill := &pendingFill{
		order:          &submittedOrder{OrderID: "order-1", QuoteID: "quote-1"},
		orderID:        common.HexToHash("0x1"),
		reservationKey: "order-1",
	}
	pending := map[string]bool{"order-1": true}

	solver.completeFill(pending, fillCompletion{fill: fill, result: txmanager.Result{
		Outcome: txmanager.OutcomeIncludedUnconfirmed,
		Err:     errors.New("confirmation wait failed"),
	}})

	logged := strings.Join(logs, "\n")
	if len(pending) != 0 || strings.Contains(logged, `"msg":"order fill failed"`) ||
		!strings.Contains(logged, "order fill included but confirmation wait failed") {
		t.Fatalf("included completion: pending=%d logs=%s", len(pending), logged)
	}
}
