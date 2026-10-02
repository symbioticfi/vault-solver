package txmanager

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

type cappedRecoveryBackend struct {
	*hashNonceTestBackend

	cancellations []*types.Transaction
	cancelErr     error
	mineCancel    bool
}

func (b *cappedRecoveryBackend) SendCancellationTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancellations = append(b.cancellations, tx)
	if b.cancelErr == nil && b.mineCancel {
		b.latestNonce, b.pendingNonce, b.confirmedNonce = tx.Nonce()+1, tx.Nonce()+1, tx.Nonce()+1
		b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	}
	return b.cancelErr
}

func newCappedRecoveryBackend() *cappedRecoveryBackend {
	return &cappedRecoveryBackend{hashNonceTestBackend: &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}}
}

func TestReconcileUnknownStartupCancelsAfterTimeoutAndConfirms(t *testing.T) {
	b := newCappedRecoveryBackend()
	b.pendingNonce, b.mineCancel = 8, true
	m := sharedNonceManager(t, b, 2)
	m.cfg.PendingTimeout = 5 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := m.Initialize(ctx); err != nil || !m.Available() {
		t.Fatalf("startup nonce recovery failed: %v", err)
	}
	if len(b.cancellations) != 1 || len(b.attemptedTransactions()) != 0 {
		t.Fatal("startup recovery sent business bytes or repeated a confirmed cancellation")
	}
	tx := b.cancellations[0]
	if tx.Nonce() != 7 || tx.To() == nil || *tx.To() != m.signer.Address() || tx.Gas() != cancellationGasLimit || len(tx.Data()) != 0 || tx.Value().Sign() != 0 || tx.GasFeeCap().Cmp(m.globalFeeLimit()) != 0 || tx.GasTipCap().Cmp(m.globalFeeLimit()) != 0 {
		t.Fatal("unknown recovery did not use the fixed public capped payload")
	}
}

func TestReconcileUnknownSingletonRemainsRememberedAfterPoolDrop(t *testing.T) {
	b := newCappedRecoveryBackend()
	b.pendingNonce = 8
	m := sharedNonceManager(t, b, 0)
	if _, err := m.freshReconciledNonce(t.Context()); err == nil || len(b.cancellations) != 0 {
		t.Fatal("healthy unknown transaction was immediately cancelled")
	}
	observed := m.unknownNonce.observedAt
	b.pendingNonce = 7
	if _, err := m.freshReconciledNonce(t.Context()); err == nil || !m.unknownNonce.observedAt.Equal(observed) {
		t.Fatal("pool disappearance erased the observed unknown nonce")
	}
	m.unknownNonce.observedAt = time.Now().Add(-m.cfg.PendingTimeout)
	if _, err := m.freshReconciledNonce(t.Context()); err == nil || len(b.cancellations) != 1 || m.Available() {
		t.Fatal("dropped observation was neither cancelled at its nonce nor gated")
	}
	if _, err := m.freshReconciledNonce(t.Context()); err == nil || len(b.cancellations) != 2 || b.cancellations[0].Hash() != b.cancellations[1].Hash() {
		t.Fatal("capped unknown cancellation did not rebroadcast exact bytes")
	}
}

func TestReconcileUnknownRecoveryRejectsGapsAndUnconfirmedConsumption(t *testing.T) {
	for _, scenario := range []string{"multi nonce gap", "mined unconfirmed", "lower nonce reorg", "underpriced cap"} {
		t.Run(scenario, func(t *testing.T) {
			b := newCappedRecoveryBackend()
			b.pendingNonce = 8
			m := sharedNonceManager(t, b, 2)
			_, _ = m.freshReconciledNonce(t.Context())
			m.unknownNonce.observedAt = time.Now().Add(-m.cfg.PendingTimeout)
			switch scenario {
			case "multi nonce gap":
				b.pendingNonce = 9
			case "mined unconfirmed":
				b.latestNonce, b.pendingNonce = 8, 8
			case "lower nonce reorg":
				b.latestNonce, b.pendingNonce, b.confirmedNonce = 6, 6, 6
			case "underpriced cap":
				b.cancelErr = errors.New("replacement transaction underpriced")
			}
			if _, err := m.freshReconciledNonce(t.Context()); err == nil || m.Available() {
				t.Fatal("unsafe unknown lane resumed")
			}
			wantSends := 0
			if scenario == "underpriced cap" {
				wantSends = 1
				_, _ = m.freshReconciledNonce(t.Context())
				wantSends++
			}
			if len(b.cancellations) != wantSends {
				t.Fatalf("unknown cancellation sends = %d, want %d", len(b.cancellations), wantSends)
			}
			for _, tx := range b.cancellations {
				if tx.GasFeeCap().Cmp(m.globalFeeLimit()) != 0 || tx.GasTipCap().Cmp(m.globalFeeLimit()) != 0 {
					t.Fatal("cap rejection escalated fees")
				}
			}
		})
	}
}

func TestReconcileContestedDroppedCandidateWaitsFullTimeout(t *testing.T) {
	b := newCappedRecoveryBackend()
	b.sendErrs = []error{errors.New("replacement transaction underpriced")}
	m := sharedNonceManager(t, b, 0)
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123"), CancelAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	m.trackUnminedTransaction(pending)
	requestCancellation(pending)
	if _, err := m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true}); err != nil || len(b.cancellations) != 0 {
		t.Fatal("local shutdown cancelled another replica before the recovery timeout")
	}
	pending.firstSignedAt = time.Now().Add(-m.cfg.PendingTimeout)
	b.mineCancel = true
	if _, err := m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true}); err != nil || len(b.cancellations) != 1 || len(pending.attempts) != 2 {
		t.Fatalf("aged dropped candidate did not track capped cancellation: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := m.waitForPendingTransaction(ctx, pending)
	if result.Outcome != OutcomeCancelled || result.Receipt == nil || result.Hash != b.cancellations[0].Hash() {
		t.Fatalf("public cancellation receipt was not reconciled: %+v", result)
	}
}

func TestReconcileContestedCancellationRequiresCurrentCanonicalNonce(t *testing.T) {
	for _, scenario := range []string{"mined unconfirmed", "lower nonce reorg", "current state unavailable", "head changes"} {
		t.Run(scenario, func(t *testing.T) {
			b := newCappedRecoveryBackend()
			b.sendErrs = []error{errors.New("replacement transaction underpriced")}
			m := sharedNonceManager(t, b, 2)
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
			if err != nil {
				t.Fatal(err)
			}
			pending.firstSignedAt = time.Now().Add(-m.cfg.PendingTimeout)
			b.pendingNonce = 8 // The write endpoint still reports the rejected candidate's nonce.
			current := uint64(8)
			b.currentNonce = &current
			switch scenario {
			case "lower nonce reorg":
				current = 6
			case "current state unavailable":
				current, b.currentHashErr = 7, errors.New("canonical account state unavailable")
			case "head changes":
				current, b.latestHeads = 7, []uint64{100, 101}
			}
			_, _ = m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true})
			if len(b.cancellations) != 0 || len(pending.attempts) != 1 || m.Available() {
				t.Fatal("stale or unproved canonical nonce authorized a contested cancellation")
			}
			current, b.currentHashErr, b.latestHeads = 7, nil, nil
			if _, err := m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true}); err != nil || len(b.cancellations) != 1 {
				t.Fatalf("canonical nonce agreement did not restore capped recovery: %v", err)
			}
			current = 8
			_, _ = m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true})
			if len(b.cancellations) != 1 || len(pending.attempts) != 2 {
				t.Fatal("known unconfirmed inclusion authorized signed cancellation rebroadcast")
			}
		})
	}
}

func TestReconcileNormalReplacementCannotResetOriginalConflictAge(t *testing.T) {
	b := newCappedRecoveryBackend()
	m := sharedNonceManager(t, b, 0)
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
	if err != nil {
		t.Fatal(err)
	}
	delete(b.receipts, pending.originalHash) // The original remains pending while its fee is replaced.
	pending.firstSignedAt = time.Now().Add(-m.cfg.PendingTimeout)
	before := pending.firstSignedAt
	if _, err := m.tryReplace(t.Context(), pending, replaceIntent{}); err != nil {
		t.Fatal(err)
	}
	if !pending.firstSignedAt.Equal(before) || time.Since(pending.horizon.sentAt) >= m.cfg.PendingTimeout {
		t.Fatal("replacement altered the immutable original age")
	}
	delete(b.receipts, pending.attempts[len(pending.attempts)-1].hash)
	m.markNonceConflict(pending.nonce, pending.originalHash)
	if _, err := m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true}); err != nil || len(b.cancellations) != 1 {
		t.Fatal("fresh replacement reset an already-expired conflict recovery timeout")
	}
}
