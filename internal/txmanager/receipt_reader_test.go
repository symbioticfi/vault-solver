package txmanager

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

// receiptResult runs one sweep through the real reader for receipt validation,
// confirmation and log-streak cases. Lifecycle timer scheduling is covered by
// waitForPendingTransaction tests below.
func (m *Manager) receiptResult(ctx context.Context, pending *pendingTransaction) (Result, bool) {
	sweep := newReceiptSweep(pending)
	if sweep == nil {
		return Result{}, false
	}
	reader := m.startReceiptReader(ctx)
	defer reader.stop()
	for i, attempt := range sweep.attempts {
		select {
		case reader.requests <- attempt:
		case <-ctx.Done():
			return Result{}, false
		}
		pending.receiptCursor = (sweep.start + i + 1) % len(sweep.attempts)
		var read receiptRead
		select {
		case read = <-reader.results:
		case <-ctx.Done():
			return Result{}, false
		}
		if m.observeReceiptRead(pending, sweep, read) {
			return m.confirmPendingReceipt(ctx, pending, attempt, read.receipt)
		}
	}
	m.finishReceiptSweep(pending, sweep)
	return Result{}, false
}

// controlledReceiptBackend holds the original hash until released, while newly
// signed replacements use the mock's mined receipt. It never holds the mock lock
// across I/O, matching the production client's concurrent read/write contract.
type controlledReceiptBackend struct {
	*mockBackend

	blocked common.Hash
	release chan struct{}
	active  atomic.Int32
}

func (b *controlledReceiptBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	b.active.Add(1)
	defer b.active.Add(-1)
	if hash == b.blocked {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.release:
			return nil, ethereum.NotFound
		}
	}
	return b.mockBackend.TransactionReceipt(ctx, hash)
}

func TestPendingReceiptDoesNotBlockLifecycleEvents(t *testing.T) {
	for _, event := range []string{"request deadline", "shutdown cancellation", "replacement"} {
		t.Run(event, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				backend := &controlledReceiptBackend{mockBackend: newMockBackend(), release: make(chan struct{})}
				cfg := Config{MaxFeeGwei: 100, PollInterval: 700 * time.Millisecond, ReplacementInterval: 10 * time.Second, PendingTimeout: time.Minute}
				req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Data: []byte{1}, Label: "async receipts"}
				if event == "request deadline" {
					req.CancelAt = time.Now().Add(100 * time.Millisecond)
				}
				if event == "replacement" {
					cfg.ReplacementInterval = time.Second
				}
				m := New(backend, mustSigner(t), big.NewInt(1), cfg, logr.Discard())
				pending, err := m.broadcast(t.Context(), req)
				if err != nil {
					t.Fatal(err)
				}
				backend.blocked = pending.originalHash
				m.trackUnminedTransaction(pending)
				results := make(chan Result, 1)
				go func() { results <- m.waitForPendingTransaction(t.Context(), pending) }()
				synctest.Wait()
				if event == "shutdown cancellation" {
					close(pending.cancelRequested)
				}
				delay := 110 * time.Millisecond
				if event == "replacement" {
					delay = 1010 * time.Millisecond
				}
				time.Sleep(delay)
				synctest.Wait()
				sent := backend.attemptedTransactions()
				if len(sent) != 2 {
					t.Fatalf("sent %d transactions while receipt blocked, want 2", len(sent))
				}
				if backend.active.Load() != 1 {
					t.Fatal("expected a receipt read still in flight at lifecycle event")
				}
				isCancellation := sent[1].To() != nil && *sent[1].To() == m.signer.Address() && len(sent[1].Data()) == 0
				if isCancellation != (event != "replacement") || sent[1].Nonce() != sent[0].Nonce() {
					t.Fatalf("unexpected replacement: cancellation=%v nonce=%d", isCancellation, sent[1].Nonce())
				}
				close(backend.release)
				got := <-results
				want := OutcomeCancelled
				if event == "replacement" {
					want = OutcomeConfirmed
				}
				if got.Outcome != want || got.Hash != sent[1].Hash() {
					t.Fatalf("result %+v, want %s for new hash", got, want)
				}
				if backend.active.Load() != 0 {
					t.Fatal("receipt worker outlived lifecycle")
				}
			})
		})
	}
}

type slowReceiptBackend struct {
	*mockBackend

	delay   time.Duration
	budgets []time.Duration
}

func (b *slowReceiptBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	deadline, _ := ctx.Deadline()
	b.budgets = append(b.budgets, time.Until(deadline))
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(b.delay):
		return b.mockBackend.TransactionReceipt(ctx, hash)
	}
}

func TestReceiptSweepGivesEveryHashFullTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := &slowReceiptBackend{mockBackend: newMockBackend(), delay: 70 * time.Millisecond}
		m := New(backend, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Second, ReplacementInterval: 30 * time.Second}, logr.Discard())
		pending := &pendingTransaction{req: Request{Label: "52 hashes"}, log: logr.Discard(), nonce: 7, cancelDeadline: time.Now().Add(time.Hour)}
		for i := range 52 {
			tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, Gas: 21000, GasFeeCap: big.NewInt(int64(i + 1))})
			pending.attempts = append(pending.attempts, txAttempt{hash: tx.Hash()})
			if i == 51 {
				backend.receipts[tx.Hash()] = successfulReceipt(tx, backend.head)
			}
		}
		pending.originalHash = pending.attempts[0].hash
		result := m.waitForPendingTransaction(t.Context(), pending)
		if result.Outcome != OutcomeConfirmed || result.Hash != pending.attempts[51].hash {
			t.Fatalf("result: %+v", result)
		}
		if len(backend.budgets) != 52 {
			t.Fatalf("reads=%d, want 52", len(backend.budgets))
		}
		for i, budget := range backend.budgets {
			if budget != 2*time.Second {
				t.Fatalf("read %d budget=%s", i, budget)
			}
		}
	})
}

func TestReceiptReaderStopsWithLifecycleContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := &controlledReceiptBackend{mockBackend: newMockBackend(), release: make(chan struct{})}
		m := New(backend, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000})
		if err != nil {
			t.Fatal(err)
		}
		backend.blocked = pending.originalHash
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan Result, 1)
		go func() { result <- m.waitForPendingTransaction(ctx, pending) }()
		synctest.Wait()
		if backend.active.Load() != 1 {
			t.Fatal("receipt not started")
		}
		cancel()
		got := <-result
		if got.Outcome != OutcomeTrackingStopped || !errors.Is(got.Err, context.Canceled) {
			t.Fatalf("result: %+v", got)
		}
		if backend.active.Load() != 0 {
			t.Fatal("receipt reader was not joined")
		}
	})
}

func TestReceiptReaderResumesAfterReorg(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := newMockBackend()
		logs, logger := newLogCapture(0)
		m := New(backend, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, Confirmations: 2, PollInterval: time.Second}, logger)
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000})
		if err != nil {
			t.Fatal(err)
		}
		backend.receipts[pending.originalHash].BlockNumber = big.NewInt(int64(backend.head - 2))
		backend.receipts[pending.originalHash].BlockHash = receiptTestHeader(backend.head - 2).Hash()
		backend.reorgedHeader = true
		m.trackUnminedTransaction(pending)
		result := make(chan Result, 1)
		go func() { result <- m.waitForPendingTransaction(t.Context(), pending) }()
		synctest.Wait()
		if _, info := countLogs(*logs, "transaction inclusion reorged; resuming pending lifecycle"); info != 1 {
			t.Fatalf("reorg logs: %v", *logs)
		}
		select {
		case got := <-result:
			t.Fatalf("reorg completed lifecycle: %+v", got)
		default:
		}
		backend.mu.Lock()
		backend.reorgedHeader = false
		backend.mu.Unlock()
		got := <-result
		if got.Outcome != OutcomeConfirmed || got.Hash != pending.originalHash {
			t.Fatalf("result: %+v", got)
		}
	})
}

func TestCoincidentCancellationAndReplacementSendOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := mustSigner(t)
		backend := &replacementBackend{mockBackend: newMockBackend(), cancellationTo: s.Address()}
		m := New(backend, s, big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, ReplacementInterval: 100 * time.Millisecond, PendingTimeout: 100 * time.Millisecond}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Data: []byte{1}})
		if err != nil {
			t.Fatal(err)
		}
		m.trackUnminedTransaction(pending)
		got := m.waitForPendingTransaction(t.Context(), pending)
		if got.Outcome != OutcomeCancelled || len(backend.sent) != 2 {
			t.Fatalf("result=%+v sends=%d", got, len(backend.sent))
		}
	})
}

func TestReceiptReaderStopUnblocksUndeliveredResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
		reader := m.startReceiptReader(t.Context())
		reader.requests <- txAttempt{}
		synctest.Wait() // RPC is complete; the reader is blocked delivering its result.
		reader.stop()
	})
}
