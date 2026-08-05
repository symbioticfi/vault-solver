package lifi

import (
	"context"
	"encoding/json"
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
	select {
	case <-enqueued:
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked without a consumer")
	}

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
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inbox did not stop after cancellation")
	}
}

func TestParseOrderMessageIgnoresDutchAuctions(t *testing.T) {
	tests := []byte{dutchAuctionContextType, exclusiveDutchAuctionContextType}
	for _, contextType := range tests {
		t.Run(hexutil.Encode([]byte{contextType}), func(t *testing.T) {
			cfg := testLifiConfig()
			var body map[string]any
			if err := json.Unmarshal(testOrderJSON(
				t,
				cfg,
				common.HexToAddress("0x6666666666666666666666666666666666666666"),
				common.HexToAddress("0x7777777777777777777777777777777777777777"),
			), &body); err != nil {
				t.Fatalf("unmarshal order: %v", err)
			}
			output := sliceField(t, mapField(t, body, "order"), "outputs")[0].(map[string]any)
			output["context"] = hexutil.Encode([]byte{contextType})
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal order: %v", err)
			}

			var logs []string
			solver := &Solver{
				cfg:     cfg,
				chainID: 11155111,
				log:     funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
			}
			if order := solver.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw}); order != nil {
				t.Fatalf("parseOrderMessage() = %+v, want ignored order", order)
			}
			logged := strings.Join(logs, "\n")
			if !strings.Contains(logged, "ignored unsupported Dutch auction") ||
				!strings.Contains(logged, hexutil.Encode([]byte{contextType})) {
				t.Fatalf("unsupported auction log = %s", logged)
			}
		})
	}
}

func TestOrderInboxCoalescesQueuedReplay(t *testing.T) {
	inbox := newOrderInbox(2)
	first := &submittedOrder{OrderID: "api-1", OnChainOrderID: "chain-1"}
	if err := inbox.enqueue(first); err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(&submittedOrder{OrderID: "api-2", OnChainOrderID: "chain-1"}); err != nil {
		t.Fatal(err)
	}
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

	if err := inbox.enqueue(&submittedOrder{OnChainOrderID: " 0xAbCd "}); err != nil {
		t.Fatal(err)
	}
	if order := <-orders; order.OnChainOrderID != " 0xAbCd " {
		t.Fatalf("order = %+v", order)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("inbox.run() error = %v", err)
	}

	if err := inbox.enqueue(&submittedOrder{OnChainOrderID: "0xabcd"}); err != nil {
		t.Fatal(err)
	}
	if len(inbox.orders) != 0 {
		t.Fatalf("REST replay was re-enqueued after live copy drained: %+v", inbox.orders)
	}
	if !inbox.tryEndRecovery(inbox.recoveryGen) {
		t.Fatal("recovery did not end after the replay was coalesced")
	}
	if err := inbox.enqueue(&submittedOrder{OnChainOrderID: "0xabcd"}); err != nil {
		t.Fatal(err)
	}
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
	if len(inbox.recoverySeen) != orderRecoverySeenCapacity {
		t.Fatalf("recovery seen keys = %d, want %d", len(inbox.recoverySeen), orderRecoverySeenCapacity)
	}
	if inbox.recoverySeen["0"] {
		t.Fatal("oldest recovery key was not evicted")
	}
	if !inbox.recoverySeen[strconv.Itoa(orderRecoverySeenCapacity)] {
		t.Fatal("newest recovery key is missing")
	}
}

func TestOrderInboxPreservesRecoveryEvictionOrderAfterCompaction(t *testing.T) {
	inbox := newOrderInbox(orderRecoverySeenCapacity + 2)
	inbox.beginRecovery()

	for index := 0; index <= orderRecoverySeenCapacity; index++ {
		if err := inbox.enqueue(&submittedOrder{OrderID: strconv.Itoa(index)}); err != nil {
			t.Fatalf("enqueue %d: %v", index, err)
		}
	}
	if inbox.recoverySeenNext == 0 {
		t.Fatal("recovery seen ring did not wrap")
	}

	if retries := inbox.takeRecoveryRetries(); len(retries) != 0 {
		t.Fatalf("recovery retries = %d, want 0", len(retries))
	}
	newest := strconv.Itoa(orderRecoverySeenCapacity + 1)
	if err := inbox.enqueue(&submittedOrder{OrderID: newest}); err != nil {
		t.Fatalf("enqueue newest: %v", err)
	}
	if inbox.recoverySeen["1"] {
		t.Fatal("oldest recovery key was not evicted after compaction")
	}
	if !inbox.recoverySeen[strconv.Itoa(orderRecoverySeenCapacity)] || !inbox.recoverySeen[newest] {
		t.Fatal("compaction evicted a newer recovery key")
	}
}

func TestOrderInboxRecoveryBarrierBackpressuresUntilWorker(t *testing.T) {
	inbox := newOrderInbox(1)
	if err := inbox.enqueueWait(t.Context(), &submittedOrder{OrderID: "first"}); err != nil {
		t.Fatal(err)
	}
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
	if err := <-barrierDone; err != nil {
		t.Fatalf("waitUntilProcessed: %v", err)
	}
	cancel()
	if err := <-runDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("inbox.run() error = %v", err)
	}
}

func TestOrderInboxRejectsOverflow(t *testing.T) {
	inbox := newOrderInbox(1)
	if err := inbox.enqueue(&submittedOrder{OrderID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(&submittedOrder{OrderID: "second"}); !errors.Is(err, errOrderInboxFull) {
		t.Fatalf("enqueue error = %v, want %v", err, errOrderInboxFull)
	}
}

func TestOrderInboxCloseDrainsQueuedOrders(t *testing.T) {
	inbox := newOrderInbox(2)
	first := &submittedOrder{OrderID: "first"}
	second := &submittedOrder{OrderID: "second"}
	if err := inbox.enqueue(first); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if err := inbox.enqueue(second); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}
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
	if err := <-done; err != nil {
		t.Fatalf("order inbox drain: %v", err)
	}
}

func TestOrderInboxRecoveryOverflowRequiresAnotherSweep(t *testing.T) {
	inbox := newOrderInbox(1)
	inbox.beginRecovery()
	if err := inbox.enqueue(&submittedOrder{OrderID: "first"}); err != nil {
		t.Fatal(err)
	}
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

	if err := inbox.enqueue(&submittedOrder{OrderID: "before-barrier"}); err != nil {
		t.Fatal(err)
	}
	processedGen, err := inbox.waitUntilProcessed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := inbox.enqueue(&submittedOrder{OrderID: "after-barrier"}); err != nil {
		t.Fatal(err)
	}
	if inbox.tryEndRecovery(processedGen) {
		t.Fatal("recovery ended after an order was enqueued behind the processed barrier")
	}
	processedGen, err = inbox.waitUntilProcessed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
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
	retries := newReservationRetryQueue(2)
	first := &submittedOrder{OrderID: "first"}
	second := &submittedOrder{OrderID: "second"}
	if err := retries.enqueue(first, 0); err != nil {
		t.Fatal(err)
	}
	if err := retries.enqueue(first, 0); err != nil || retries.len() != 1 {
		t.Fatalf("duplicate enqueue: len=%d err=%v", retries.len(), err)
	}
	if err := retries.enqueue(second, 1); err != nil {
		t.Fatal(err)
	}
	if err := retries.enqueue(&submittedOrder{OrderID: "dropped-newest"}, 1); !errors.Is(err, errOrderRetryFull) {
		t.Fatalf("overflow error = %v, want %v", err, errOrderRetryFull)
	}
	if order := retries.popReady(0); order != nil {
		t.Fatalf("retry before reservation change = %+v", order)
	}
	if order := retries.popReady(1); order != first {
		t.Fatalf("first ready retry = %+v, want first", order)
	}
	if order := retries.popReady(1); order != nil {
		t.Fatalf("second retry ran in its enqueue generation: %+v", order)
	}
	if order := retries.popReady(2); order != second {
		t.Fatalf("second ready retry = %+v, want second", order)
	}
}

func TestOrderWorkerRecoveryBarrierFollowsCapacityReservation(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{hold: true}
	solver := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
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

	select {
	case <-barrier.processed:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not acknowledge recovery barrier")
	}
	select {
	case <-inputDrained:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not acknowledge the drained input")
	}
	if reservations := solver.capacity.Snapshot(); len(reservations) == 0 {
		t.Fatal("recovery barrier passed before accepted fill reserved capacity")
	}
	if len(txm.results) != 1 {
		t.Fatalf("pending transactions = %d, want 1", len(txm.results))
	}
	txm.results[0] <- txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after pending fill completed")
	}
}

func TestOrderWorkerMarksTransientFailureForRecovery(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	solver := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		&fakeLifiTxSender{},
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
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

	if err := solver.runOrderWorker(t.Context(), nil, orders, func(got *submittedOrder, attemptLimit int) {
		marked <- markedRecovery{order: got, attemptLimit: attemptLimit}
	}, nil); err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

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
	solver := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		&fakeLifiTxSender{},
		webhookstrategy.New(client),
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	solver.orders = newOrderClient(orderServer.URL, "test-key", time.Second, 11155111)
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
	if err := <-inboxDone; err != nil {
		t.Fatalf("inbox.run: %v", err)
	}
	if err := <-workerDone; err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}
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

func TestOrderRecoveryRetainsLiveRetryAcrossReconnectWhenSnapshotOmitsOrder(t *testing.T) {
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
	orders := make(chan *submittedOrder)
	go func() { _ = inbox.run(ctx, orders) }()
	var attempts atomic.Int32
	firstAttemptDone := make(chan struct{})
	go func() {
		for order := range orders {
			if order.processed != nil {
				close(order.processed)
				continue
			}
			attempt := attempts.Add(1)
			if attempt <= maximumStrategyRecoveryAttempts {
				inbox.markRecoveryRetry(order, maximumStrategyRecoveryAttempts)
			}
			if attempt == 1 {
				close(firstAttemptDone)
			}
		}
	}()
	if err := inbox.enqueue(&submittedOrder{OrderID: "live-only"}); err != nil {
		t.Fatalf("enqueue live order: %v", err)
	}
	select {
	case <-firstAttemptDone:
	case <-ctx.Done():
		t.Fatalf("live worker attempt did not complete: %v", ctx.Err())
	}

	// The connection drops before its recovery sweep can consume the retained
	// retry. The next connection starts from an empty, lagging REST snapshot.
	inbox.beginRecovery()

	if !solver.recoverOrdersUntilSuccess(ctx, inbox) {
		t.Fatalf("reconnect recovery did not converge: %v", ctx.Err())
	}
	if got := attempts.Load(); got != maximumStrategyRecoveryAttempts {
		t.Fatalf("worker attempts = %d, want retained retry and attempt budget %d", got, maximumStrategyRecoveryAttempts)
	}
	if got := requests.Load(); got == 0 {
		t.Fatal("empty reconnect snapshot was not queried")
	}
}
