package txmanager

import (
	"context"
	"io"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type pendingNonceErrorBackend struct {
	*mockBackend

	err   error
	block bool
}

func (b *pendingNonceErrorBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if b.block {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	if b.err != nil {
		return 0, b.err
	}
	return b.mockBackend.PendingNonceAt(ctx, account)
}

func TestFreshPendingNonceIncludesOtherSenders(t *testing.T) {
	b := newMockBackend()
	b.pendingNonce = 10 // Three foreign pending transactions need not be confirmed first.
	m := New(b, mustSigner(t), big.NewInt(1), Config{Confirmations: 2}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil || !m.LaneReady() {
		t.Fatalf("foreign pending work blocked initialization: %v", err)
	}
	for _, nonce := range []uint64{10, 14} {
		b.pendingNonce = nonce
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
		if err != nil || pending.nonce != nonce {
			t.Fatalf("fresh request nonce: pending=%+v err=%v, want %d", pending, err, nonce)
		}
	}
	if len(b.attemptedTransactions()) != 2 {
		t.Fatal("initialization sent a transaction for another sender's nonce")
	}
}

func TestInitialNonceRaceReleasesLaneForFreshOrder(t *testing.T) {
	for _, rpcError := range []string{"nonce too low", "nonce is too low", "nonce has already been used", "replacement transaction underpriced"} {
		t.Run(rpcError, func(t *testing.T) {
			b := newMockBackend()
			b.sendErrs = []error{errors.New(rpcError)}
			m := newTestManager(t, b)
			request := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "raced"}
			result := m.Send(t.Context(), request)
			if result.Outcome != OutcomeNonceConflict || !errors.Is(result.Err, ErrNonceConflict) || result.Hash == (common.Hash{}) || result.Receipt != nil || result.NotAdmitted || result.Outcome.Included() {
				t.Fatalf("nonce race result: %+v", result)
			}
			waitForAdmissionDemand(t, m, 0)
			if !m.LaneReady() || b.sendCalls != 1 {
				t.Fatal("nonce race held the lane or attempted a cancellation/replay")
			}
			b.mu.Lock()
			b.pendingNonce = 9
			b.mu.Unlock()
			fresh := m.Send(t.Context(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "fresh"})
			if fresh.Outcome != OutcomeConfirmed || fresh.Err != nil || b.lastSent().Nonce() != 9 {
				t.Fatalf("next order did not use a fresh pending nonce: %+v", fresh)
			}
		})
	}
}

func TestPendingNonceReadFailureIsBoundedAndRetryable(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "timeout"}[blocked], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &pendingNonceErrorBackend{mockBackend: newMockBackend(), err: io.ErrUnexpectedEOF, block: blocked}
				m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
				want := io.ErrUnexpectedEOF
				if blocked {
					want = context.DeadlineExceeded
				}
				started := time.Now()
				if err := m.Initialize(t.Context()); !errors.Is(err, want) || m.Available() {
					t.Fatalf("failed initialization: %v", err)
				}
				if elapsed := time.Since(started); elapsed > m.receiptReadTimeout() {
					t.Fatalf("nonce read exceeded budget: %s", elapsed)
				}
				b.err, b.block = nil, false
				if err := m.Initialize(t.Context()); err != nil || !m.Available() {
					t.Fatalf("nonce RPC recovery failed: %v", err)
				}
				b.err = io.ErrUnexpectedEOF
				if pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000}); pending != nil || !errors.Is(err, b.err) || len(b.attemptedTransactions()) != 0 {
					t.Fatalf("failed read signed bytes: pending=%+v err=%v", pending, err)
				}
				b.err = nil
				if pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000}); err != nil || pending.nonce != 7 {
					t.Fatalf("fresh send after RPC recovery: pending=%+v err=%v", pending, err)
				}
			})
		})
	}
}

func TestNonceUncertainNeverMeansIncluded(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeConfirmed, OutcomeIncludedUnconfirmed, OutcomeReverted, OutcomeCancelled, OutcomeSubmissionError, OutcomeNonceConflict, OutcomeNonceConsumed} {
		want := outcome == OutcomeNonceConflict || outcome == OutcomeNonceConsumed
		if outcome.NonceUncertain() != want || (want && outcome.Included()) {
			t.Fatalf("outcome %s: uncertain=%v included=%v", outcome, outcome.NonceUncertain(), outcome.Included())
		}
	}
}
