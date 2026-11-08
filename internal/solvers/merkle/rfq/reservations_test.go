package rfq

import (
	"math/big"
	"testing"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func testSpendLedger(t *testing.T) *spendLedger {
	t.Helper()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ledger, err := loadSpends(store)
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}

func testSpend() fillSpend {
	return fillSpend{CapacityID: liquidlane.NewCapacityID(1, vlt, tOut), IncludedAt: 8, Deadline: 100}
}

func TestSpendPersistenceFailureKeepsSignerOwnership(t *testing.T) {
	ledger := testSpendLedger(t)
	if err := ledger.store.Close(); err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{pending: "signed", result: submission.Result{Confirmed: true, Execution: &provider.Execution{BlockNumber: big.NewInt(8)}}}
	s := &Solver{store: ledger.store, sender: sender, spends: ledger, intent: fillIntent{ID: "signed", OrderID: "award", Spend: testSpend()}}
	if err := s.send(t.Context()); err == nil || sender.acks != 0 || sender.PendingID() == "" {
		t.Fatal("failed durable accounting released the signer fence", err)
	}
}

func TestPendingJournalPrecedesExpiryAndFailedAcknowledgement(t *testing.T) {
	ledger := testSpendLedger(t)
	spend := testSpend()
	if err := ledger.include("award", spend); err != nil {
		t.Fatal(err)
	}
	if err := ledger.expire(99); err != nil || !ledger.finalized("award") {
		t.Fatal("order expired before signed deadline", err)
	}
	sender := &fakeSender{pending: "signed", result: submission.Result{Pending: true}}
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	f.reader.header.Time = 1000 // The signed deadline has passed while acknowledgement is pending.
	backend := &fakeBackend{}
	s := &Solver{sender: sender, store: ledger.store, spends: ledger, workflow: f.workflow, backend: backend, intent: fillIntent{ID: "signed", OrderID: "award", Spend: spend}}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	sender.result = submission.Result{Confirmed: true, Execution: &provider.Execution{BlockNumber: big.NewInt(8)}}
	sender.ackErr = errors.New("canonical recheck unavailable")
	if err := s.tick(t.Context()); err == nil || sender.PendingID() == "" || !ledger.finalized("award") {
		t.Fatal("failed acknowledgement lost protected work", err)
	}
	if !f.reader.headerStarted.IsZero() || backend.reads != 0 || !ledger.finalized("award") {
		t.Fatal("pending acknowledgement permitted deadline cleanup or new lookup")
	}
	sender.ackErr = nil
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(ledger.state.Watermarks) != 1 || len(ledger.state.Completed) != 1 {
		t.Fatal("acknowledgement retry duplicated accounting")
	}
	if err := s.tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	restored, err := loadSpends(ledger.store)
	if err != nil {
		t.Fatal(err)
	}
	if restored.finalized("award") || restored.state.Watermarks[spend.CapacityID] != 8 || backend.reads != 1 {
		t.Fatal("deadline cleanup lost durable watermark protection")
	}
}
