package txmanager

import (
	"context"
	"io"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// A private relay can acknowledge every send without mining it or returning
// "nonce too low". Receipt publication is controlled separately by each test.
type silentAcceptanceBackend struct {
	*mockBackend

	nonceErr   error
	blockNonce bool
}

func (b *silentAcceptanceBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendCalls++
	b.attempted = append(b.attempted, tx)
	b.sent = append(b.sent, tx)
	return nil
}

func (b *silentAcceptanceBackend) NonceAt(ctx context.Context, account common.Address, block *big.Int) (uint64, error) {
	if b.blockNonce {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	if b.nonceErr != nil {
		return 0, b.nonceErr
	}
	return b.mockBackend.NonceAt(ctx, account, block)
}

func TestReplacementsStopWhenMinedNonceAdvances(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		cancellation         bool
		existingCancellation bool
		uncertain            bool
		capped               bool
	}{
		{name: "normal replacement"},
		{name: "initial cancellation", cancellation: true},
		{name: "cancellation replacement", cancellation: true, existingCancellation: true},
		{name: "uncertain exact rebroadcast", uncertain: true},
		{name: "capped normal rebroadcast", capped: true},
		{name: "capped cancellation rebroadcast", cancellation: true, existingCancellation: true, capped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
			if err != nil {
				t.Fatal(err)
			}
			if tc.existingCancellation {
				if _, err := m.tryReplace(t.Context(), pending, true); err != nil {
					t.Fatal(err)
				}
			}
			pending.attempts[0].exactRebroadcastPending = tc.uncertain
			if tc.capped {
				pending.fees.maxFee.Set(m.globalFeeLimit())
			}
			before := len(b.attemptedTransactions())
			fees := cloneFeeQuote(pending.fees)
			b.latestNonce, b.pendingNonce = 8, 8
			for range 3 {
				if _, err := m.tryReplace(t.Context(), pending, tc.cancellation); err != nil {
					t.Fatal(err)
				}
			}
			if got := len(b.attemptedTransactions()); got != before || len(pending.attempts) != before {
				t.Fatalf("consumed nonce was broadcast again: sends=%d attempts=%d, want %d", got, len(pending.attempts), before)
			}
			if pending.fees.maxFee.Cmp(fees.maxFee) != 0 || pending.fees.tip.Cmp(fees.tip) != 0 {
				t.Fatal("fees escalated after the nonce was consumed")
			}
			if m.Available() {
				t.Fatal("unexplained mined nonce advancement left admission/readiness available")
			}
			if _, err := m.broadcast(t.Context(), pending.req); !errors.Is(err, errNonceLanePaused) {
				t.Fatalf("new request after external consumption = %v, want paused lane", err)
			}
		})
	}
}

func TestPendingNonceAdvanceDoesNotSuppressReplacement(t *testing.T) {
	b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if err != nil {
		t.Fatal(err)
	}
	b.pendingNonce = 8 // Our unmined submission advances pending, not latest.
	if _, err := m.tryReplace(t.Context(), pending, false); err != nil {
		t.Fatal(err)
	}
	if len(b.attemptedTransactions()) != 2 || !m.Available() {
		t.Fatal("our pending nonce prevented a legitimate fee replacement")
	}
}

func TestConsumedNonceRequiresCanonicalOwnedReceipt(t *testing.T) {
	for _, orphaned := range []bool{false, true} {
		t.Run(map[bool]string{false: "canonical", true: "orphaned"}[orphaned], func(t *testing.T) {
			b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
			if err != nil {
				t.Fatal(err)
			}
			b.latestNonce, b.pendingNonce = 8, 8
			b.receipts[pending.originalHash] = successfulReceipt(pending.attempts[0].tx, 100)
			b.reorgedHeader = orphaned
			if _, err := m.tryReplace(t.Context(), pending, true); err != nil {
				t.Fatal(err)
			}
			if len(b.attemptedTransactions()) != 1 {
				t.Fatal("sent a cancellation after observing a consumed nonce")
			}
			result, done := m.receiptResult(t.Context(), pending)
			if orphaned {
				if done || m.Available() {
					t.Fatalf("orphaned owned receipt resolved nonce conflict: %+v", result)
				}
			} else if !done || !m.Available() || result.Outcome != OutcomeConfirmed || result.Hash != pending.originalHash {
				t.Fatalf("canonical owned receipt did not resolve the lifecycle: %+v", result)
			}
		})
	}
}

func TestReplacementNonceReadFailureDefersBroadcastAndRecovers(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "RPC error", true: "RPC deadline"}[blocked], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
				m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
				pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
				if err != nil {
					t.Fatal(err)
				}
				b.blockNonce, b.nonceErr = blocked, io.ErrUnexpectedEOF
				wantErr := io.ErrUnexpectedEOF
				if blocked {
					wantErr = context.DeadlineExceeded
				}
				started := time.Now()
				if _, err := m.tryReplace(t.Context(), pending, true); !errors.Is(err, wantErr) {
					t.Fatalf("nonce read error = %v, want %v", err, wantErr)
				}
				if elapsed := time.Since(started); elapsed > 2*time.Second {
					t.Fatalf("nonce check exceeded its read budget: %s", elapsed)
				}
				if len(b.attemptedTransactions()) != 1 || !m.Available() {
					t.Fatal("failed nonce read broadcast a cancellation or permanently conflicted the lane")
				}
				b.blockNonce, b.nonceErr = false, nil
				if _, err := m.tryReplace(t.Context(), pending, true); err != nil {
					t.Fatal(err)
				}
				if len(b.attemptedTransactions()) != 2 {
					t.Fatal("replacement did not resume after the nonce RPC recovered")
				}
			})
		})
	}
}

func TestConsumedNonceKeepsTrackingUntilOwnedReceiptArrives(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
		m := New(b, mustSigner(t), big.NewInt(1), Config{
			MaxFeeGwei: 100, PollInterval: time.Second, ReplacementInterval: 10 * time.Second,
			Confirmations: 2,
		}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
		if err != nil {
			t.Fatal(err)
		}
		b.latestNonce, b.pendingNonce = 8, 8
		m.trackUnminedTransaction(pending)
		ctx, cancel := context.WithCancel(t.Context())
		results := make(chan Result, 1)
		done := make(chan struct{})
		go func() {
			defer close(done)
			results <- m.waitForPendingTransaction(ctx, pending)
		}()
		defer func() { cancel(); <-done }()
		time.Sleep(31 * time.Second)
		synctest.Wait()
		if m.Available() || len(b.attemptedTransactions()) != 1 {
			t.Fatal("silent acceptance left consumed nonce replacements running")
		}
		select {
		case result := <-results:
			t.Fatalf("nonce advancement invented a terminal result: %+v", result)
		default:
		}
		b.mu.Lock()
		b.receipts[pending.originalHash] = successfulReceipt(pending.attempts[0].tx, 100)
		b.head = 102
		b.mu.Unlock()
		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case result := <-results:
			if result.Outcome != OutcomeConfirmed || result.Hash != pending.originalHash || result.Err != nil {
				t.Fatalf("delayed owned receipt = %+v", result)
			}
		default:
			t.Fatal("owned canonical receipt did not complete the lifecycle")
		}
		if !m.Available() {
			t.Fatal("owned canonical receipt did not resume admission/readiness")
		}
	})
}
