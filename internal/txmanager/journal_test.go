package txmanager

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

type journalCheckingBackend struct {
	*mockBackend

	beforeSend func(*types.Transaction) error
}

func (b *journalCheckingBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := b.beforeSend(tx); err != nil {
		return err
	}
	return b.mockBackend.SendTransaction(ctx, tx)
}

func initializedJournalManager(t *testing.T, backend Backend, path string, confirmations uint64) *Manager {
	t.Helper()
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatal(err)
	}
	m := New(backend, s, big.NewInt(11155111), Config{
		StateFile: path, Confirmations: confirmations, MaxFeeGwei: 100,
		PollInterval: time.Millisecond, ReplacementInterval: 20 * time.Millisecond,
		PendingTimeout: time.Minute, ShutdownTimeout: 25 * time.Millisecond,
	}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m
}

func readJournalState(t *testing.T, path string) journalState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state journalState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func waitJournalReady(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !m.LaneReady() {
		select {
		case <-deadline.C:
			t.Fatal("journal recovery did not finish")
		case <-tick.C:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
}

func TestJournalPersistsEveryAttemptBeforeBroadcast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := &journalCheckingBackend{mockBackend: newMockBackend()}
	checks := 0
	b.beforeSend = func(tx *types.Transaction) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var state journalState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		checks++
		if len(state.Attempts) != checks {
			return errors.New("broadcast did not retain complete attempt history")
		}
		last := new(types.Transaction)
		if err := last.UnmarshalBinary(state.Attempts[len(state.Attempts)-1].Raw); err != nil {
			return err
		}
		if last.Hash() != tx.Hash() {
			return errors.New("broadcast bytes differ from durable signed bytes")
		}
		return nil
	}
	m := initializedJournalManager(t, b, path, 0)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
		To: common.HexToAddress("0x1234"), Data: []byte{1, 2, 3}, Value: big.NewInt(4), Label: "write-ahead",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, cancellation := range []bool{false, true} {
		if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{cancellation: cancellation}); err != nil {
			t.Fatal(err)
		}
	}
	if checks != 3 || len(pending.attempts) != 3 {
		t.Fatalf("checks=%d attempts=%d", checks, len(pending.attempts))
	}
}

func TestJournalRecoveryRetainsWinnerAndCancelsUnresolvedWork(t *testing.T) {
	for _, winner := range []string{"original", "replacement", "cancellation", "reverted", "unbroadcast"} {
		t.Run(winner, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tx.json")
			b := newMockBackend()
			b.sendErrs = []error{errors.New("timeout after submission"), errors.New("timeout after replacement")}
			m := initializedJournalManager(t, b, path, 2)
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
				To: common.HexToAddress("0x1234"), Data: []byte{1, 2}, Label: "recover", Solver: "test",
				MaxFeePerGas: big.NewInt(70e9), CancelAt: time.Now().Add(time.Minute),
			})
			if err != nil {
				t.Fatal(err)
			}
			if winner == "replacement" || winner == "cancellation" {
				if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{cancellation: winner == "cancellation"}); err != nil {
					// These RPC timeouts are ambiguous; the signed replacement must be retained.
					if len(pending.attempts) != 2 {
						t.Fatal(err)
					}
				}
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			b.mu.Lock()
			if winner != "unbroadcast" {
				tx := pending.latestAttempt().tx
				b.receipts[tx.Hash()] = successfulReceipt(tx, 98)
				if winner == "reverted" {
					b.receipts[tx.Hash()].Status = types.ReceiptStatusFailed
				}
				b.latestNonce, b.pendingNonce = 8, 8
			} else {
				b.sendErrs = nil
			}
			b.mu.Unlock()
			restarted := initializedJournalManager(t, b, path, 0)
			if restarted.Available() || restarted.LaneReady() || restarted.Idle() {
				t.Fatal("recovered work allowed fresh commitments")
			}
			if restarted.recovered.req.Obsolete != nil || restarted.confirmations(restarted.recovered.req) != 2 {
				t.Fatal("recovery changed recorded finality or reconstructed callback")
			}
			results := make(chan Result, 1)
			restarted.recovered.result = results
			if winner == "unbroadcast" {
				// The mock mines the recovery cancellation at head 100; advance the head to
				// allow the persisted two-confirmation policy to complete.
				go func() {
					for {
						b.mu.Lock()
						if len(b.sent) > 0 {
							b.head = 102
							b.mu.Unlock()
							return
						}
						b.mu.Unlock()
						select {
						case <-t.Context().Done():
							return
						case <-time.After(time.Millisecond):
						}
					}
				}()
			}
			before := len(b.attemptedTransactions())
			startManagerForTest(t, restarted)
			waitJournalReady(t, restarted)
			result := <-results
			wantOutcome := OutcomeConfirmed
			switch winner {
			case "cancellation", "unbroadcast":
				wantOutcome = OutcomeCancelled
			case "reverted":
				wantOutcome = OutcomeReverted
			}
			if result.Outcome != wantOutcome || (winner != "unbroadcast" && result.Hash != pending.latestAttempt().hash) {
				t.Fatalf("winner %s produced result %+v", winner, result)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("finalized journal was not removed: %v", err)
			}
			attempts := b.attemptedTransactions()
			if winner != "unbroadcast" && len(attempts) != before {
				t.Fatal("recovery broadcast despite a final owned receipt")
			}
			for _, tx := range attempts[before:] {
				if tx.Nonce() != pending.nonce || tx.To() == nil || *tx.To() != restarted.signer.Address() || len(tx.Data()) != 0 || tx.Value().Sign() != 0 {
					t.Fatal("recovery replayed the business call or changed its nonce")
				}
			}
		})
	}
}

func TestJournalWriteFailureNeverBroadcastsOrReusesNonce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	m := initializedJournalManager(t, b, path, 0)
	// An unsafe state target makes save fail before any network exposure.
	if err := os.Symlink("missing", path); err != nil {
		t.Fatal(err)
	}
	startManagerForTest(t, m)
	result := m.Send(t.Context(), Request{To: common.HexToAddress("0x1234"), Label: "storage-failure"})
	if result.Err == nil || len(b.attemptedTransactions()) != 0 || m.Available() {
		t.Fatalf("unsafe write result=%+v sends=%d available=%v", result, len(b.attemptedTransactions()), m.Available())
	}
	if _, admitted := m.TrySend(t.Context(), Request{}); admitted {
		t.Fatal("storage failure allowed a second request")
	}
}

func TestJournalReplacementWriteFailureRetainsOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	m := initializedJournalManager(t, b, path, 0)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0x1234"), Label: "preserve"})
	if err != nil {
		t.Fatal(err)
	}
	original := readJournalState(t, path)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{cancellation: true}); err == nil {
		t.Fatal("unsafe replacement journal succeeded")
	}
	state := readJournalState(t, path)
	if len(state.Attempts) != 1 || string(state.Attempts[0].Raw) != string(original.Attempts[0].Raw) || len(b.attemptedTransactions()) != 1 || m.Available() {
		t.Fatal("replacement failure discarded ownership or broadcast")
	}
}

func TestJournalRetainsUnconfirmedAndTrackingStopped(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeTrackingStopped, OutcomeIncludedUnconfirmed, OutcomeCancelledUnconfirmed, OutcomeReverted} {
		t.Run(string(outcome), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tx.json")
			b := newMockBackend()
			m := initializedJournalManager(t, b, path, 2)
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0x1234"), Label: "unfinished"})
			if err != nil {
				t.Fatal(err)
			}
			result := m.finishJournal(t.Context(), pending, Result{Outcome: outcome, Receipt: successfulReceipt(pending.latestAttempt().tx, 100)})
			if result.Err == nil || m.Available() {
				t.Fatal("unfinished journal did not block admission")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("unfinished journal was lost", err)
			}
		})
	}
}

func TestJournalDefiniteInitialRejectionReleasesRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	b.sendErrs = []error{errors.New("insufficient funds")}
	m := initializedJournalManager(t, b, path, 0)
	startManagerForTest(t, m)
	result := m.Send(t.Context(), Request{To: common.HexToAddress("0x1234"), Label: "rejected"})
	if result.Outcome != OutcomeSubmissionError || result.Err == nil {
		t.Fatalf("rejection result=%+v", result)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("definitely rejected initial transaction retained its record", err)
	}
	if !m.Available() {
		t.Fatal("definite rejection paused admission")
	}
}

func TestJournalClearFailurePausesAdmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	m := initializedJournalManager(t, b, path, 0)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0x1234"), Label: "clear-failure"})
	if err != nil {
		t.Fatal(err)
	}
	result, done := m.confirmPendingReceipt(managerCtx(t.Context(), m), pending, pending.latestAttempt(), successfulReceipt(pending.latestAttempt().tx, 100))
	if !done || result.Err != nil {
		t.Fatalf("confirmation=%+v done=%v", result, done)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	result = m.finishJournal(managerCtx(t.Context(), m), pending, result)
	if result.Err == nil || m.Available() {
		t.Fatal("clear failure did not preserve the admission pause")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("clear failure removed the journal", err)
	}
}

// A slow receipt endpoint must never allow the roomy-block exact-rebroadcast path to replay
// recovered business calldata before its first sweep completes.
type recoverySlowReceiptBackend struct {
	*mockBackend
}

func (b *recoverySlowReceiptBackend) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestJournalRecoveryCannotRebroadcastWhileReceiptReadIsBlocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	b.sendErrs = []error{errors.New("ambiguous")}
	m := initializedJournalManager(t, b, path, 0)
	if _, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0x1234"), Label: "slow-recovery"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// A fast block cadence drives the stalled/rebroadcast branch before slow receipt I/O returns.
	slow := &recoverySlowReceiptBackend{mockBackend: b}
	restarted := initializedJournalManager(t, slow, path, 0)
	restarted.horizon.blockTime = time.Millisecond
	restarted.horizon.stallAfterBlocks = 1
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); restarted.Start(ctx) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for len(b.attemptedTransactions()) < 2 {
		select {
		case <-deadline.C:
			cancel()
			<-done
			t.Fatal("recovery cancellation was not broadcast")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	<-done
	// Start has bounded shutdown. The reader honors its cancellation and then exits promptly.
	for {
		restarted.unminedMu.Lock()
		active := restarted.unmined != nil
		restarted.unminedMu.Unlock()
		if !active {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("recovery lifecycle did not exit")
		case <-time.After(time.Millisecond):
		}
	}
	for _, tx := range b.attemptedTransactions()[1:] {
		if tx.To() == nil || *tx.To() != restarted.signer.Address() || len(tx.Data()) != 0 {
			t.Fatal("recovery rebroadcast original business calldata")
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("stopped recovery lost its journal", err)
	}
}

func TestJournalZeroConfirmationsStillRequiresCanonicalReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	m := initializedJournalManager(t, b, path, 0)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0x1234"), Label: "canonical"})
	if err != nil {
		t.Fatal(err)
	}
	bad := successfulReceipt(pending.latestAttempt().tx, 100)
	bad.BlockHash = common.HexToHash("0xbad")
	if _, done := m.confirmPendingReceipt(managerCtx(t.Context(), m), pending, pending.latestAttempt(), bad); done || pending.finalized {
		t.Fatal("untrusted receipt finalized durable ownership")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("untrusted receipt lost the journal", err)
	}
}

func TestJournalRecoveryRejectsDirectBusinessRebroadcast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	b := newMockBackend()
	m := initializedJournalManager(t, b, path, 0)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.HexToAddress("0x1234"), Label: "no-replay"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := initializedJournalManager(t, b, path, 0)
	if err := restarted.sendSigned(t.Context(), pending.latestAttempt().tx, true, false); err == nil {
		t.Fatal("direct recovery business rebroadcast succeeded")
	}
	if len(b.attemptedTransactions()) != 1 {
		t.Fatal("direct recovery reached the network")
	}
}

var _ Backend = (*recoverySlowReceiptBackend)(nil)
