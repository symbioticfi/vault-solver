package txmanager

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Every hash gets its own timeout. One timeout must not truncate the sweep or
// increase logging beyond the existing first-error/repeat streak.
type diagnosticReceiptBackend struct {
	*mockBackend

	calls int
}

func (b *diagnosticReceiptBackend) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	b.calls++
	if b.calls%4 != 3 {
		return nil, ethereum.NotFound
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestReceiptSweepDiagnosticContextAndFrequency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs, logger := newLogCapture(1)
		b := &diagnosticReceiptBackend{mockBackend: newMockBackend()}
		m := newStreakManager(t, b, logger)
		m.cfg.ReplacementInterval = 40 * time.Millisecond
		pending := &pendingTransaction{req: Request{Label: "diagnostic"}, log: logger, attempts: []txAttempt{{hash: common.HexToHash("1")}, {hash: common.HexToHash("2")}, {hash: common.HexToHash("3")}, {hash: common.HexToHash("4")}}}
		for range 2 {
			if _, done := m.receiptResult(t.Context(), pending); done {
				t.Fatal("unexpected terminal receipt")
			}
		}
		errs, debug := countLogs(*logs, "pending transaction receipt unavailable")
		if errs != 1 || debug != 1 || b.calls != 8 {
			t.Fatalf("frequency: errors=%d debug=%d RPCs=%d", errs, debug, b.calls)
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte((*logs)[0]), &fields); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]any{
			"reason_code": "receipt_rpc_deadline", "cancelCause": context.DeadlineExceeded.Error(),
			"hashesChecked": float64(4), "hashesTotal": float64(4), "rpcChecks": float64(4),
			"rpcBudgetTotalMs": float64(80), "sweepElapsedMs": float64(20), "lastRPCDurationMs": float64(0),
		} {
			if fields[key] != want {
				t.Errorf("%s = %v, want %v", key, fields[key], want)
			}
		}
		if _, ok := fields["sweepBudgetMs"]; ok {
			t.Fatal("obsolete shared deadline advertised")
		}
	})
}

func TestReceiptSweepDiagnosticsIncludePriorityReads(t *testing.T) {
	logs, logger := newLogCapture(1)
	m := newStreakManager(t, newMockBackend(), logger)
	first, latest := txAttempt{hash: common.HexToHash("1")}, txAttempt{hash: common.HexToHash("2")}
	pending := &pendingTransaction{req: Request{Label: "diagnostic"}, log: logger, attempts: []txAttempt{first}}
	sweep := newReceiptSweep(pending, 1)
	pending.attempts = append(pending.attempts, latest)
	// A priority hash can also be revisited: it is one checked hash, but two RPCs.
	for _, read := range []receiptRead{
		{attempt: latest, err: context.DeadlineExceeded, budget: time.Second, duration: time.Second, cancelCause: context.DeadlineExceeded.Error()},
		{attempt: first, err: ethereum.NotFound, budget: time.Second, duration: time.Millisecond, cancelCause: "none"},
		{attempt: latest, err: ethereum.NotFound, budget: time.Second, duration: 2 * time.Millisecond, cancelCause: "none"},
	} {
		m.observeReceiptRead(pending, sweep, read)
	}
	m.finishReceiptSweep(pending, sweep)
	var fields map[string]any
	if err := json.Unmarshal([]byte((*logs)[0]), &fields); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"hashesChecked": float64(2), "hashesTotal": float64(2), "rpcChecks": float64(3), "rpcBudgetTotalMs": float64(3000), "lastRPCDurationMs": float64(2), "hash": latest.hash.Hex(), "cancelCause": context.DeadlineExceeded.Error()} {
		if fields[key] != want {
			t.Errorf("%s = %v, want %v", key, fields[key], want)
		}
	}
}
