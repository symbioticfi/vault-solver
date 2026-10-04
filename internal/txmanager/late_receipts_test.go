package txmanager

import (
	"context"
	"math/big"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

func TestOrdinaryReceiptInvokesTelemetryBeforeResultDelivery(t *testing.T) {
	backend := newMockBackend()
	manager := startTestManager(t, backend, Config{}, newTestMetrics(t))
	observed := make(chan Result, 2)
	result := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "ordinary",
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
	})
	select {
	case receipt := <-observed:
		if receipt.Hash != result.Hash || receipt.Outcome != OutcomeConfirmed || receipt.Receipt == nil {
			t.Fatalf("receipt observation = %+v, result = %+v", receipt, result)
		}
		if !slices.Equal(receipt.Attempts, result.Attempts) {
			t.Fatalf("ordinary telemetry lost signed attempt ownership: observer=%v caller=%v", receipt.Attempts, result.Attempts)
		}
	default:
		t.Fatal("ordinary result was delivered without receipt telemetry")
	}
	select {
	case duplicate := <-observed:
		t.Fatalf("duplicate receipt telemetry: %+v", duplicate)
	default:
	}
}

func TestLateReceiptAccountsOldWinnerWhileFreshLaneWorks(t *testing.T) {
	backend := &pendingMetricsBackend{mockBackend: newMockBackend()}
	metrics := newTestMetrics(t)
	manager := startTestManager(t, backend, Config{
		PendingTimeout: 15 * time.Millisecond, ReplacementInterval: time.Hour,
		LateReceiptTimeout: time.Second,
	}, metrics)
	observed := make(chan Result, 2)
	old := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0xaaa"), Data: []byte{1}, GasLimit: 21_000, Label: "old",
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
	})
	if old.Outcome != OutcomeAbandoned || old.Receipt != nil {
		t.Fatalf("old request = %+v, want execution unknown", old)
	}
	fresh, accepted := manager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xbbb"), Data: []byte{2}, GasLimit: 30_000, Label: "fresh",
	})
	if !accepted {
		t.Fatal("released lane refused fresh work")
	}
	waitForLateReceiptSends(t, backend.mockBackend, 2)
	transactions := backend.attemptedTransactions()
	if transactions[0].Nonce() != transactions[1].Nonce() {
		t.Fatal("fresh request skipped the abandoned nonce")
	}
	putLateReceipt(backend.mockBackend, transactions[0], types.ReceiptStatusSuccessful, 100)
	select {
	case receipt := <-observed:
		if receipt.Hash != old.Hash || receipt.Outcome != OutcomeConfirmed || receipt.Receipt == nil {
			t.Fatalf("late observation = %+v", receipt)
		}
		if !slices.Equal(receipt.Attempts, []common.Hash{receipt.Hash}) {
			t.Fatalf("late receipt did not identify its owned hash: %+v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("abandoned transaction executed without late receipt telemetry")
	}
	if got := <-fresh; got.Outcome != OutcomeNonceConsumed {
		t.Fatalf("fresh losing request = %+v", got)
	}
	assertMetric(t, metrics.requests.WithLabelValues("old", string(OutcomeAbandoned)), 1)
	assertMetric(t, metrics.requests.WithLabelValues("old", string(OutcomeConfirmed)), 0)
	assertMetric(t, metrics.inflight.WithLabelValues("old"), 0)
	assertMetric(t, metrics.gasUsed.WithLabelValues("old", string(OutcomeConfirmed)), 21_000)
	assertMetric(t, metrics.feePaidWei.WithLabelValues("old", string(OutcomeConfirmed)), 42_000_000_000_000)
	if got := len(backend.attemptedTransactions()); got != 2 {
		t.Fatalf("receipt observer sent another transaction: sends=%d", got)
	}
	select {
	case duplicate := <-observed:
		t.Fatalf("duplicate late telemetry: %+v", duplicate)
	default:
	}
}

func TestLateRevertedReceiptAccountsActualCosts(t *testing.T) {
	backend := &pendingMetricsBackend{mockBackend: newMockBackend()}
	metrics := newTestMetrics(t)
	manager := startTestManager(t, backend, Config{
		PendingTimeout: 10 * time.Millisecond, ReplacementInterval: time.Hour,
	}, metrics)
	observed := make(chan Result, 1)
	result := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0xaaa"), GasLimit: 30_000, Label: "revert",
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
	})
	if result.Outcome != OutcomeAbandoned {
		t.Fatalf("result=%+v", result)
	}
	putLateReceipt(backend.mockBackend, backend.lastSent(), types.ReceiptStatusFailed, 100)
	select {
	case receipt := <-observed:
		if receipt.Outcome != OutcomeReverted || receipt.Err == nil {
			t.Fatalf("late revert=%+v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("late revert was never observed")
	}
	assertMetric(t, metrics.lateReceipts.WithLabelValues("revert", "reverted"), 1)
	assertMetric(t, metrics.gasUsed.WithLabelValues("revert", "reverted"), 30_000)
	assertMetric(t, metrics.feePaidWei.WithLabelValues("revert", "reverted"), 60_000_000_000_000)
	assertMetric(t, metrics.requests.WithLabelValues("revert", "abandoned"), 1)
	assertMetric(t, metrics.inflight.WithLabelValues("revert"), 0)
}

func TestLateReceiptWaitsForCanonicalConfirmationDepth(t *testing.T) {
	backend := &pendingMetricsBackend{mockBackend: newMockBackend()}
	manager := startTestManager(t, backend, Config{
		Confirmations: 2, PendingTimeout: 10 * time.Millisecond, ReplacementInterval: time.Hour,
	}, nil)
	observed := make(chan Result, 2)
	result := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0xaaa"), GasLimit: 21_000,
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
	})
	if result.Outcome != OutcomeAbandoned {
		t.Fatalf("result=%+v", result)
	}
	tx := backend.lastSent()
	orphan := successfulReceipt(tx, 100)
	orphan.BlockHash = forkedReceiptHeader(100, "orphan").Hash()
	backend.mu.Lock()
	backend.receipts[tx.Hash()] = orphan
	backend.mu.Unlock()
	assertNoLateReceipt(t, observed)
	backend.mu.Lock()
	backend.head = 102
	backend.mu.Unlock()
	assertNoLateReceipt(t, observed)
	putLateReceipt(backend.mockBackend, tx, types.ReceiptStatusSuccessful, 101)
	assertNoLateReceipt(t, observed)
	backend.mu.Lock()
	backend.head = 103
	backend.mu.Unlock()
	select {
	case receipt := <-observed:
		if receipt.Outcome != OutcomeConfirmed || receipt.Receipt.BlockNumber.Uint64() != 101 {
			t.Fatalf("late canonical receipt=%+v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical receipt at configured depth was never observed")
	}
}

func TestLateReceiptRetentionExpiresAndCapacityDoesNotBlockSending(t *testing.T) {
	for _, reason := range []string{"expired", "capacity"} {
		t.Run(reason, func(t *testing.T) {
			backend := &pendingMetricsBackend{mockBackend: newMockBackend()}
			metrics := newTestMetrics(t)
			cfg := Config{PendingTimeout: 5 * time.Millisecond, ReplacementInterval: time.Hour,
				LateReceiptTimeout: 30 * time.Millisecond, LateReceiptMaxHashes: 1}
			if reason == "capacity" {
				cfg.LateReceiptTimeout = time.Second
			}
			manager := startTestManager(t, backend, cfg, metrics)
			observed := make(chan Result, 2)
			first := manager.Send(t.Context(), Request{
				To: common.HexToAddress("0xaaa"), GasLimit: 21_000, Label: "first",
				ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
			})
			if first.Outcome != OutcomeAbandoned {
				t.Fatalf("first=%+v", first)
			}
			if reason == "capacity" {
				second := manager.Send(t.Context(), Request{To: common.HexToAddress("0xbbb"), GasLimit: 21_000, Label: "second"})
				if second.Outcome != OutcomeAbandoned {
					t.Fatalf("observer capacity blocked fresh request: %+v", second)
				}
			} else {
				time.Sleep(45 * time.Millisecond)
			}
			assertMetric(t, metrics.lateReceiptDropped.WithLabelValues("first", reason), 1)
			assertMetric(t, metrics.lateReceiptPending.WithLabelValues("first"), 0)
			putLateReceipt(backend.mockBackend, backend.attemptedTransactions()[0], types.ReceiptStatusSuccessful, 100)
			assertNoLateReceipt(t, observed)
		})
	}
}

type blockedLateReceiptBackend struct {
	*pendingMetricsBackend

	block   atomic.Bool
	entered chan struct{}
}

func (b *blockedLateReceiptBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	if b.block.Load() {
		select {
		case b.entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.pendingMetricsBackend.TransactionReceipt(ctx, hash)
}

func TestShutdownStopsPassiveReceiptObserverWithoutDrainingIt(t *testing.T) {
	backend := &blockedLateReceiptBackend{
		pendingMetricsBackend: &pendingMetricsBackend{mockBackend: newMockBackend()}, entered: make(chan struct{}, 1),
	}
	metrics := newTestMetrics(t)
	manager := newLateReceiptManager(t, backend, Config{
		PendingTimeout: 5 * time.Millisecond, ReplacementInterval: time.Hour, ShutdownTimeout: time.Hour,
	}, metrics)
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { manager.Start(ctx); close(stopped) }()
	defer cancel()
	result := manager.Send(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000, Label: "stopped"})
	if result.Outcome != OutcomeAbandoned {
		t.Fatalf("result=%+v", result)
	}
	backend.block.Store(true)
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("passive observer never read retained hash")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("passive observation extended shutdown drain")
	}
	assertMetric(t, metrics.lateReceiptDropped.WithLabelValues("stopped", "shutdown"), 1)
	assertMetric(t, metrics.lateReceiptPending.WithLabelValues("stopped"), 0)
}

type invalidReceiptStatusBackend struct{ *mockBackend }

func (b *invalidReceiptStatusBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := b.mockBackend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.mu.Lock()
	b.receipts[tx.Hash()].Status = 2
	b.mu.Unlock()
	return nil
}

func TestInvalidReceiptStatusSkipsTelemetryWithoutChangingLifecycle(t *testing.T) {
	backend := &invalidReceiptStatusBackend{mockBackend: newMockBackend()}
	metrics := newTestMetrics(t)
	manager := startTestManager(t, backend, Config{
		PendingTimeout: 10 * time.Millisecond, ReplacementInterval: time.Hour,
		LateReceiptTimeout: 20 * time.Millisecond,
	}, metrics)
	observed := make(chan Result, 1)
	result := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "invalid",
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
	})
	if result.Outcome != OutcomeConfirmed || result.Receipt == nil || result.Receipt.Status != 2 {
		t.Fatalf("telemetry changed the ordinary request result: %+v", result)
	}
	assertNoLateReceipt(t, observed)
	assertMetric(t, metrics.gasUsed.WithLabelValues("invalid", "confirmed"), 0)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("invalid", "confirmed"), 0)
}

func TestReceiptAccountingDeduplicatesSameHashAcrossNormalAndLatePaths(t *testing.T) {
	metrics := newTestMetrics(t)
	backend := &metricsReceiptBackend{mockBackend: newMockBackend(), effectiveGasPrice: big.NewInt(2_000_000_000)}
	manager := startTestManager(t, backend, Config{}, metrics)
	var callbacks atomic.Int64
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "once",
		ObserveReceipt: func(context.Context, Result) { callbacks.Add(1) }}
	result := manager.Send(t.Context(), req)
	if result.Outcome != OutcomeConfirmed {
		t.Fatalf("normal result=%+v", result)
	}
	metadata := manager.receiptMetadata(req, 7)
	var competing sync.WaitGroup
	for index := range 8 {
		competing.Go(func() { manager.recordReceipt(t.Context(), metadata, result, index%2 == 0) })
	}
	competing.Wait()
	if callbacks.Load() != 1 {
		t.Fatalf("same hash invoked callbacks %d times", callbacks.Load())
	}
	assertMetric(t, metrics.gasUsed.WithLabelValues("once", "confirmed"), 21_000)
	assertMetric(t, metrics.feePaidWei.WithLabelValues("once", "confirmed"), 42_000_000_000_000)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("once", "confirmed"), 0)
	assertMetric(t, metrics.requests.WithLabelValues("once", "confirmed"), 1)
}

type lateReceiptHeadSwitchBackend struct {
	*mockBackend

	reads int
}

func (b *lateReceiptHeadSwitchBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if number != nil {
		return b.mockBackend.HeaderByNumber(ctx, number)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reads++
	if b.reads >= 3 {
		b.head, b.reorgedHeader = 102, true
	}
	return b.headerLocked(b.head), nil
}

func TestLateReceiptCannotCombineOldAncestryWithNewForkDepth(t *testing.T) {
	backend := &lateReceiptHeadSwitchBackend{mockBackend: newMockBackend()}
	manager := newLateReceiptManager(t, backend, Config{Confirmations: 2}, nil)
	pending, err := manager.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000})
	if err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	backend.reads, backend.head, backend.reorgedHeader = 0, 100, false
	backend.mu.Unlock()
	result, err := manager.readLateReceipt(t.Context(), lateReceiptObservation{
		hash: pending.originalHash, metadata: manager.receiptMetadata(pending.req, pending.nonce),
	})
	if result.Receipt != nil {
		t.Fatalf("old block-100 ancestry combined with another fork's block-102 depth: %+v, err=%v", result, err)
	}
}

func TestConsumedNonceReceiptLagStillUpdatesTelemetry(t *testing.T) {
	backend := &pendingMetricsBackend{mockBackend: newMockBackend()}
	metrics := newTestMetrics(t)
	manager := startTestManager(t, backend, Config{PendingTimeout: time.Hour, ReplacementInterval: time.Hour}, metrics)
	observed := make(chan Result, 1)
	result, accepted := manager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xaaa"), GasLimit: 21_000, Label: "lagged",
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt },
	})
	if !accepted {
		t.Fatal("request refused")
	}
	waitForLateReceiptSends(t, backend.mockBackend, 1)
	backend.mu.Lock()
	backend.latestNonce = 8
	backend.mu.Unlock()
	if completed := <-result; completed.Outcome != OutcomeNonceConsumed {
		t.Fatalf("nonce-consumed result=%+v", completed)
	}
	putLateReceipt(backend.mockBackend, backend.lastSent(), types.ReceiptStatusSuccessful, 100)
	select {
	case receipt := <-observed:
		if receipt.Outcome != OutcomeConfirmed {
			t.Fatalf("lagged receipt=%+v", receipt)
		}
	case <-time.After(time.Second):
		t.Fatal("receipt lag after consumed nonce lost telemetry")
	}
	assertMetric(t, metrics.requests.WithLabelValues("lagged", "nonce_consumed"), 1)
	assertMetric(t, metrics.requests.WithLabelValues("lagged", "confirmed"), 0)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("lagged", "confirmed"), 1)
}

type selectivelyBlockedLateReceiptBackend struct {
	*pendingMetricsBackend

	blocked common.Hash // guarded by mockBackend.mu
}

func (b *selectivelyBlockedLateReceiptBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	blocked := hash == b.blocked
	b.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.pendingMetricsBackend.TransactionReceipt(ctx, hash)
}

func TestBlockedLateHashDoesNotStarveAnotherOwnedReceipt(t *testing.T) {
	backend := &selectivelyBlockedLateReceiptBackend{pendingMetricsBackend: &pendingMetricsBackend{mockBackend: newMockBackend()}}
	manager := startTestManager(t, backend, Config{
		PendingTimeout: 5 * time.Millisecond, ReplacementInterval: 40 * time.Millisecond,
		LateReceiptTimeout: time.Second,
	}, nil)
	first := manager.Send(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000})
	if first.Outcome != OutcomeAbandoned {
		t.Fatalf("first=%+v", first)
	}
	backend.mu.Lock()
	backend.blocked = first.Hash
	backend.mu.Unlock()
	observed := make(chan Result, 1)
	second := manager.Send(t.Context(), Request{To: common.HexToAddress("0xbbb"), GasLimit: 21_000,
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt }})
	if second.Outcome != OutcomeAbandoned {
		t.Fatalf("second=%+v", second)
	}
	putLateReceipt(backend.mockBackend, backend.lastSent(), types.ReceiptStatusSuccessful, 100)
	select {
	case receipt := <-observed:
		if receipt.Hash != second.Hash {
			t.Fatalf("wrong hash observed: %+v", receipt)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("blocked old hash monopolized passive observation")
	}
}

func TestInitialNonceConflictDoesNotEnterPassiveObservation(t *testing.T) {
	backend := newMockBackend()
	backend.sendErrs = []error{errors.New("replacement transaction underpriced")}
	metrics := newTestMetrics(t)
	manager := startTestManager(t, backend, Config{}, metrics)
	observed := make(chan Result, 1)
	result := manager.Send(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000, Label: "conflict",
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt }})
	if result.Outcome != OutcomeNonceConflict {
		t.Fatalf("initial conflict=%+v", result)
	}
	putLateReceipt(backend, backend.attemptedTransactions()[0], types.ReceiptStatusSuccessful, 100)
	assertNoLateReceipt(t, observed)
	assertMetric(t, metrics.lateReceiptPending.WithLabelValues("conflict"), 0)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("conflict", "confirmed"), 0)
}

func TestUnconfirmedReceiptDoesNotRetireOtherSameNonceHashes(t *testing.T) {
	backend := newMockBackend()
	manager := newLateReceiptManager(t, backend, Config{}, newTestMetrics(t))
	req := Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000, Label: "old"}
	old, err := manager.broadcast(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	manager.retainLateReceipts(t.Context(), old)
	backend.mu.Lock()
	delete(backend.receipts, old.originalHash)
	backend.mu.Unlock()
	sibling, err := manager.broadcast(t.Context(), Request{To: common.HexToAddress("0xbbb"), GasLimit: 21_000, Label: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if sibling.nonce != old.nonce {
		t.Fatalf("passive candidates did not share a nonce: old=%d sibling=%d", old.nonce, sibling.nonce)
	}
	manager.retainLateReceipts(t.Context(), sibling)
	newTx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, Gas: 21_000, Data: []byte{2}})
	result := Result{Hash: newTx.Hash(), Receipt: successfulReceipt(newTx, 100), Outcome: OutcomeIncludedUnconfirmed}
	manager.recordReceipt(t.Context(), manager.receiptMetadata(req, 7), result, false)
	assertMetric(t, manager.metrics.lateReceiptPending.WithLabelValues("old"), 2)
	// A reverted receipt can still have a failed depth wait while the manager context is live.
	result.Outcome, result.Receipt.Status = OutcomeReverted, types.ReceiptStatusFailed
	result.Hash = common.HexToHash("0xbeef")
	result.Receipt.TxHash = result.Hash
	result.Err = errors.Errorf("reverted on-chain; confirmation wait: %w", errors.New("receipt read unavailable"))
	manager.recordReceipt(t.Context(), manager.receiptMetadata(req, 7), result, false)
	assertMetric(t, manager.metrics.lateReceiptPending.WithLabelValues("old"), 2)
	// A passive reverted winner has already proven depth and canonicality, so its siblings retire.
	putLateReceipt(backend, old.attempts[0].tx, types.ReceiptStatusFailed, 100)
	entry, ok := manager.nextLateReceipt()
	if !ok {
		t.Fatal("unconfirmed revert prematurely discarded its owned candidate")
	}
	if err := manager.observeLateReceipt(t.Context(), entry); err == nil {
		t.Fatal("passive reverted result did not preserve the revert error")
	}
	assertMetric(t, manager.metrics.lateReceiptPending.WithLabelValues("old"), 0)
}

type delayedLateAncestryBackend struct{ *mockBackend }

func (b *delayedLateAncestryBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Millisecond):
		return b.mockBackend.HeaderByHash(ctx, hash)
	}
}

func TestLateReceiptOldAncestryMakesProgressAcrossBoundedPolls(t *testing.T) {
	backend := &delayedLateAncestryBackend{mockBackend: newMockBackend()}
	backend.head = 160
	manager := newLateReceiptManager(t, backend, Config{
		ReplacementInterval: 80 * time.Millisecond, LateReceiptTimeout: 2 * time.Second,
	}, nil)
	observed := make(chan Result, 1)
	pending, err := manager.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000,
		ObserveReceipt: func(_ context.Context, receipt Result) { observed <- receipt }})
	if err != nil {
		t.Fatal(err)
	}
	putLateReceipt(backend.mockBackend, pending.attempts[0].tx, types.ReceiptStatusSuccessful, 100)
	manager.retainLateReceipts(t.Context(), pending)
	started := time.Now()
	manager.pollLateReceipts(t.Context())
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("first observation exceeded its overall read budget: %s", elapsed)
	}
	select {
	case result := <-observed:
		t.Fatalf("first short poll unexpectedly completed 60 delayed parent reads: %+v", result)
	default:
	}
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		manager.pollLateReceipts(t.Context())
		select {
		case result := <-observed:
			if result.Hash != pending.originalHash || result.Outcome != OutcomeConfirmed {
				t.Fatalf("old canonical receipt=%+v", result)
			}
			return
		default:
		}
	}
	t.Fatal("bounded ancestry reads restarted forever without accounting the available old receipt")
}

func TestObserverExitPreventsRetentionBeforeAdmissionsStop(t *testing.T) {
	metrics := newTestMetrics(t)
	manager := newLateReceiptManager(t, newMockBackend(), Config{}, metrics)
	pending, err := manager.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 21_000, Label: "exit"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager.monitorLateReceipts(ctx)
	// Start can still be scheduled before it closes the admission-stop channel. A detached
	// lifecycle must not append behind the observer's final cleanup in that interval.
	manager.retainLateReceipts(t.Context(), pending)
	assertMetric(t, metrics.lateReceiptPending.WithLabelValues("exit"), 0)
	assertMetric(t, metrics.lateReceiptDropped.WithLabelValues("exit", "shutdown"), 1)
}

type mutableLateHeaderBackend struct {
	*mockBackend

	served *types.Header
	calls  int
}

func (b *mutableLateHeaderBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	header, err := b.mockBackend.HeaderByHash(ctx, hash)
	b.served = header
	b.calls++
	return header, err
}

func TestPassiveHeaderCacheCopiesHeadersAndHonorsCapacityAndExpiry(t *testing.T) {
	backend := &mutableLateHeaderBackend{mockBackend: newMockBackend()}
	backend.head = 102
	manager := newLateReceiptManager(t, backend, Config{LateReceiptMaxHashes: 1, LateReceiptTimeout: 15 * time.Millisecond}, nil)
	firstHash, secondHash := receiptTestHeader(100).Hash(), receiptTestHeader(101).Hash()
	first, err := manager.lateReceiptParentHeader(t.Context(), firstHash)
	if err != nil {
		t.Fatal(err)
	}
	backend.served.Number.SetUint64(9)
	if first.Number.Uint64() != 100 || first.Hash() != firstHash {
		t.Fatal("RPC-owned header mutation corrupted immutable cached proof")
	}
	if _, err := manager.lateReceiptParentHeader(t.Context(), firstHash); err != nil || backend.calls != 1 {
		t.Fatalf("verified cached hash was not reused: calls=%d err=%v", backend.calls, err)
	}
	if _, err := manager.lateReceiptParentHeader(t.Context(), secondHash); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.lateReceiptParentHeader(t.Context(), firstHash); err != nil || backend.calls != 3 {
		t.Fatalf("capacity did not evict old proof: calls=%d err=%v", backend.calls, err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := manager.lateReceiptParentHeader(t.Context(), firstHash); err != nil || backend.calls != 4 {
		t.Fatalf("expired header proof was reused: calls=%d err=%v", backend.calls, err)
	}
}

func TestReceiptAccountingLedgerHasCapacityAndExpiryBounds(t *testing.T) {
	manager := newLateReceiptManager(t, newMockBackend(), Config{LateReceiptMaxHashes: 1, LateReceiptTimeout: 10 * time.Millisecond}, nil)
	var callbacks atomic.Int64
	metadata := manager.receiptMetadata(Request{ObserveReceipt: func(context.Context, Result) { callbacks.Add(1) }}, 7)
	firstTx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, Gas: 21_000, Data: []byte{1}})
	secondTx := types.NewTx(&types.DynamicFeeTx{Nonce: 8, Gas: 21_000, Data: []byte{2}})
	first := Result{Hash: firstTx.Hash(), Receipt: successfulReceipt(firstTx, 100), Outcome: OutcomeConfirmed}
	second := Result{Hash: secondTx.Hash(), Receipt: successfulReceipt(secondTx, 101), Outcome: OutcomeConfirmed}
	manager.recordReceipt(t.Context(), metadata, first, false)
	manager.recordReceipt(t.Context(), metadata, first, false)
	if callbacks.Load() != 1 {
		t.Fatalf("retained hash was not deduplicated: %d", callbacks.Load())
	}
	manager.recordReceipt(t.Context(), metadata, second, false)
	manager.recordReceipt(t.Context(), metadata, first, false)
	if callbacks.Load() != 3 {
		t.Fatalf("accounting cache failed to respect one-hash capacity: %d", callbacks.Load())
	}
	time.Sleep(15 * time.Millisecond)
	manager.recordReceipt(t.Context(), metadata, first, false)
	if callbacks.Load() != 4 {
		t.Fatalf("accounted hash lifetime did not expire: %d", callbacks.Load())
	}
}

func assertNoLateReceipt(t *testing.T, observed <-chan Result) {
	t.Helper()
	select {
	case receipt := <-observed:
		t.Fatalf("unqualified late receipt counted: %+v", receipt)
	case <-time.After(20 * time.Millisecond):
	}
}

func putLateReceipt(backend *mockBackend, tx *types.Transaction, status, block uint64) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	receipt := successfulReceipt(tx, block)
	receipt.Status = status
	receipt.EffectiveGasPrice = big.NewInt(2_000_000_000)
	backend.receipts[tx.Hash()] = receipt
}

func waitForLateReceiptSends(t *testing.T, backend *mockBackend, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(backend.attemptedTransactions()) < count {
		if time.Now().After(deadline) {
			t.Fatalf("no fresh send: want %d", count)
		}
		time.Sleep(time.Millisecond)
	}
}

func newLateReceiptManager(t *testing.T, backend Backend, cfg Config, metrics *Metrics) *Manager {
	t.Helper()
	cfg.PollInterval = time.Millisecond
	return NewWithMetrics(backend, mustSigner(t), big.NewInt(11155111), cfg, metrics, logr.Discard())
}
