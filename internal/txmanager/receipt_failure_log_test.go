package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
)

func newLogCapture(verbosity int) (*[]string, logr.Logger) {
	logs := &[]string{}
	return logs, funcr.NewJSON(func(entry string) { *logs = append(*logs, entry) }, funcr.Options{Verbosity: verbosity})
}

// countLogs counts entries carrying msg, split by whether they were written at error level (funcr
// emits "level" for Info calls only).
func countLogs(logs []string, msg string) (errorLevel, other int) {
	for _, entry := range logs {
		if !strings.Contains(entry, `"msg":"`+msg+`"`) {
			continue
		}
		if strings.Contains(entry, `"level":`) {
			other++
		} else {
			errorLevel++
		}
	}
	return errorLevel, other
}

func newStreakManager(t *testing.T, b Backend, logger logr.Logger) *Manager {
	t.Helper()
	return New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			MaxFeeGwei:          100,
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Second,
			PendingTimeout:      time.Second,
		},
		logger,
	)
}

func sendAndWait(t *testing.T, m *Manager, label string) {
	t.Helper()
	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: label,
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	if got := <-result; got.Err != nil {
		t.Fatalf("result: %v", got.Err)
	}
}

// A stuck RPC used to produce one error per poll per pending transaction (1,400 events in an hour
// from three pods). A failure streak is now one error at its start, debug while it lasts, and one
// info line with the count and duration when reads recover.
func TestReceiptReadFailuresLogOncePerStreak(t *testing.T) {
	logs, logger := newLogCapture(1)
	b := &receiptErrorBackend{mockBackend: newMockBackend(), receiptFailures: 3}
	m := newStreakManager(t, b, logger)
	startManagerForTest(t, m)
	sendAndWait(t, m, "receipt streak")

	if errs, debug := countLogs(*logs, "pending transaction receipt unavailable"); errs != 1 || debug != 2 {
		t.Fatalf("receipt unavailable lines: error=%d debug=%d, want 1 and 2; logs:\n%s", errs, debug, strings.Join(*logs, "\n"))
	}
	if _, info := countLogs(*logs, "pending transaction receipt reads recovered"); info != 1 {
		t.Fatalf("recovery lines = %d, want 1; logs:\n%s", info, strings.Join(*logs, "\n"))
	}
	for _, entry := range *logs {
		if strings.Contains(entry, "reads recovered") && !strings.Contains(entry, `"consecutiveFailures":3`) {
			t.Fatalf("recovery line should carry the streak length: %s", entry)
		}
	}
}

// An outage that never recovers must not go quiet after its first error: the streak is re-raised at
// error level every readFailureReminderInterval with how long it has lasted.
func TestReceiptReadFailuresRemindWhileUnrecovered(t *testing.T) {
	previous := readFailureReminderInterval
	readFailureReminderInterval = 0
	t.Cleanup(func() { readFailureReminderInterval = previous })

	logs, logger := newLogCapture(0)
	b := &receiptErrorBackend{mockBackend: newMockBackend(), receiptFailures: 4}
	m := newStreakManager(t, b, logger)
	startManagerForTest(t, m)
	sendAndWait(t, m, "receipt reminder")

	errs, _ := countLogs(*logs, "pending transaction receipt unavailable")
	if errs != 4 {
		t.Fatalf("error-level lines = %d, want 4; logs:\n%s", errs, strings.Join(*logs, "\n"))
	}
	reminders := 0
	for _, entry := range *logs {
		if strings.Contains(entry, `"since"`) {
			reminders++
		}
	}
	if reminders != 3 {
		t.Fatalf("reminder lines with streak duration = %d, want 3; logs:\n%s", reminders, strings.Join(*logs, "\n"))
	}
}

// splitReceiptBackend answers NotFound for every hash except one, which errors a fixed number of times.
type splitReceiptBackend struct {
	*mockBackend

	failing  common.Hash
	failures atomic.Int32
	limit    int32
}

func (b *splitReceiptBackend) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	if h == b.failing && b.failures.Add(1) <= b.limit {
		return nil, errors.New("receipt rpc timeout")
	}
	return b.mockBackend.TransactionReceipt(ctx, h)
}

// After a fee replacement two hashes are polled per sweep. One answering while the other fails must
// count as one continuing streak, not a recovery plus a fresh error every sweep.
func TestReceiptReadStreakSpansAttempts(t *testing.T) {
	logs, logger := newLogCapture(1)
	mk := func(nonce uint64, tip int64) *types.Transaction {
		return types.NewTx(&types.DynamicFeeTx{
			ChainID: big.NewInt(11155111), Nonce: nonce, GasTipCap: big.NewInt(tip), GasFeeCap: big.NewInt(tip * 2),
			Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
		})
	}
	first, replacement := mk(7, 1), mk(7, 2)
	b := &splitReceiptBackend{mockBackend: newMockBackend(), failing: replacement.Hash(), limit: 3}
	m := newStreakManager(t, b, logger)
	req := Request{Label: "split", Solver: "rfq-filler"}
	pending := &pendingTransaction{
		req: req, log: m.requestLog(req), nonce: 7, originalHash: first.Hash(),
		attempts: []txAttempt{{hash: first.Hash(), tx: first}, {hash: replacement.Hash(), tx: replacement}},
	}

	for range 3 {
		if _, done := m.receiptResult(t.Context(), pending); done {
			t.Fatal("lifecycle completed without a receipt")
		}
	}
	if errs, debug := countLogs(*logs, "pending transaction receipt unavailable"); errs != 1 || debug != 2 {
		t.Fatalf("receipt unavailable lines: error=%d debug=%d, want 1 and 2; logs:\n%s", errs, debug, strings.Join(*logs, "\n"))
	}
	if _, info := countLogs(*logs, "pending transaction receipt reads recovered"); info != 0 {
		t.Fatalf("recovered while one hash still failed; logs:\n%s", strings.Join(*logs, "\n"))
	}
	for _, entry := range *logs {
		if !strings.Contains(entry, `"solver":"rfq-filler"`) {
			t.Fatalf("txmanager log line should name the owning solver: %s", entry)
		}
	}

	if _, done := m.receiptResult(t.Context(), pending); done {
		t.Fatal("lifecycle completed without a receipt")
	}
	if _, info := countLogs(*logs, "pending transaction receipt reads recovered"); info != 1 {
		t.Fatalf("recovery lines = %d, want 1; logs:\n%s", info, strings.Join(*logs, "\n"))
	}
}

// scriptedReceiptBackend answers receipt reads from a script (nil delegates to the mock), then
// delegates for the rest.
type scriptedReceiptBackend struct {
	*mockBackend

	script []error
	calls  atomic.Int32
}

func (b *scriptedReceiptBackend) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	if i := int(b.calls.Add(1)) - 1; i < len(b.script) && b.script[i] != nil {
		return nil, b.script[i]
	}
	return b.mockBackend.TransactionReceipt(ctx, h)
}

// A failed read between two nulls is not two consecutive misses: NotFound, timeout, NotFound must
// not be reported as a reorg.
func TestConfirmationMissCountResetsOnReadError(t *testing.T) {
	b := &scriptedReceiptBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard())
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	b.receipts[tx.Hash()] = successfulReceipt(tx, b.head-2)
	// initial pending read, then the confirmation reads
	b.script = []error{nil, ethereum.NotFound, errors.New("receipt rpc timeout"), ethereum.NotFound, nil}
	pending := &pendingTransaction{
		req: Request{Label: "scripted"}, nonce: 7,
		attempts: []txAttempt{{hash: tx.Hash(), tx: tx}},
	}
	m.trackUnminedTransaction(pending)

	result, done := m.receiptResult(t.Context(), pending)
	if !done || result.Outcome != OutcomeConfirmed {
		t.Fatalf("result = %+v done=%v, want confirmed", result, done)
	}
}
