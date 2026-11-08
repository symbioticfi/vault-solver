package txmanager

import (
	"context"
	"io"
	"math/big"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type underpricedReplacementScenario struct {
	manager *Manager
	backend *mockBackend
	pending *pendingTransaction
	logs    *[]string
	cause   error
}

func underpricedReplacement(t *testing.T) underpricedReplacementScenario {
	t.Helper()
	b := newMockBackend()
	logs, log := newLogCapture(1)
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: 100 * time.Millisecond}, log)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.Address{1}, GasLimit: 21_000})
	if err != nil {
		t.Fatal(err)
	}
	delete(b.receipts, pending.originalHash)
	m.trackUnminedTransaction(pending)
	b.sendErrs = []error{nil, errors.New("replacement transaction underpriced")}
	_, replacementErr := m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{})
	return underpricedReplacementScenario{m, b, pending, logs, replacementErr}
}

func TestUnderpricedOwnedReplacementPreservesAcceptedFeeHint(t *testing.T) {
	scenario := underpricedReplacement(t)
	m, b, pending := scenario.manager, scenario.backend, scenario.pending
	logs, err := scenario.logs, scenario.cause
	if !isPendingNonceCollision(err) || len(pending.attempts) != 2 {
		t.Fatalf("replacement result = %v, attempts=%d; want tracked rejected candidate", err, len(pending.attempts))
	}
	original := b.attemptedTransactions()[0]
	if pending.fees.maxFee.Cmp(original.GasFeeCap()) != 0 || pending.fees.tip.Cmp(original.GasTipCap()) != 0 {
		t.Fatalf("rejected replacement ratcheted fees to %s/%s", pending.fees.maxFee, pending.fees.tip)
	}
	// Another replacement path cannot keep bidding after contention was already established.
	m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{})
	if len(b.attemptedTransactions()) != 2 {
		t.Fatal("continued replacing after an underpriced own replacement")
	}
	if failures, _ := countLogs(*logs, "replacement broadcast uncertain; tracking signed hash"); failures != 0 {
		t.Fatal("expected nonce contention produced an Error alert")
	}
}

func TestUnderpricedOwnedReplacementImmediatelyAbandonsForReconciliation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scenario := underpricedReplacement(t)
		m, b, pending := scenario.manager, scenario.backend, scenario.pending
		cause := scenario.cause
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		started := time.Now()
		result := m.waitForPendingTransaction(managerCtx(ctx, m), pending)
		if result.Outcome != OutcomeAbandoned || !errors.Is(result.Err, ErrAbandoned) || !errors.Is(result.Err, cause) {
			t.Fatalf("underpriced result = %+v; want reconciliation abandonment", result)
		}
		if time.Since(started) != 0 || len(b.attemptedTransactions()) != 2 || len(pending.attempts) != 2 {
			t.Fatal("contention kept the pending lifecycle or broadcast again")
		}
		hint := m.reusableSnapshot()
		if hint == nil || hint.fees.maxFee.Cmp(b.attemptedTransactions()[0].GasFeeCap()) != 0 {
			t.Fatal("abandonment retained rejected replacement fees")
		}
	})
}

func TestUnderpricedYieldResultRetainsEverySignedCandidate(t *testing.T) {
	scenario := underpricedReplacement(t)
	m, pending := scenario.manager, scenario.pending
	results := make(chan Result, 1)
	pending.result = results
	pending.lifecycle = m.metrics.beginLifecycle("fill")
	m.complete(managerCtx(t.Context(), m), pending)
	result := <-results
	if result.Outcome != OutcomeAbandoned || len(result.Attempts) != 2 ||
		result.Attempts[0] != pending.originalHash || result.Attempts[1] != pending.latestAttempt().hash {
		t.Fatalf("contention yield lost a signed variant needed for peer classification: %+v", result)
	}
}

func TestNonceCooldownAppliesToAnotherQueuedOrder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured time.Duration
		wait       time.Duration
	}{
		{"default slot", 0, 12 * time.Second},
		{"configured slot", 5 * time.Second, 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := newMockBackend()
				b.sendErrs = []error{errors.New("replacement transaction underpriced")}
				m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: 100 * time.Millisecond, Horizon: HorizonConfig{BlockTime: tc.configured}}, logr.Discard())
				startManagerForTest(t, m)
				first := m.Send(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000})
				if first.Outcome != OutcomeNonceConflict {
					t.Fatalf("first collision = %+v", first)
				}
				started := time.Now()
				result, accepted := m.SendAsync(t.Context(), Request{To: common.Address{2}, GasLimit: 21_000})
				if !accepted {
					t.Fatal("different order was not admitted for the bounded cooldown")
				}
				synctest.Wait()
				if len(b.attemptedTransactions()) != 1 {
					t.Fatal("another queued order bypassed the conflicted nonce cooldown")
				}
				time.Sleep(tc.wait - time.Second)
				if len(b.attemptedTransactions()) != 1 {
					t.Fatal("fresh transaction raised fees inside the cooldown")
				}
				completed := <-result
				if completed.Outcome != OutcomeConfirmed || time.Since(started) != tc.wait || len(b.attemptedTransactions()) != 2 {
					t.Fatalf("bounded cooldown result = %+v after %s", completed, time.Since(started))
				}
			})
		})
	}
}

func TestNonceCooldownEndsWhenMinedNonceAdvances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newMockBackend()
		b.sendErrs = []error{errors.New("replacement transaction underpriced")}
		m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: 100 * time.Millisecond}, logr.Discard())
		if _, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000}); err != nil {
			t.Fatal(err)
		}
		result := make(chan *pendingTransaction, 1)
		go func() {
			pending, _ := m.broadcast(t.Context(), Request{To: common.Address{2}, GasLimit: 21_000})
			result <- pending
		}()
		synctest.Wait()
		if len(b.attemptedTransactions()) != 1 {
			t.Fatal("fresh transaction did not wait for its nonce")
		}
		time.Sleep(time.Second)
		b.mu.Lock()
		b.latestNonce = 8
		b.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		pending := <-result
		if pending == nil || pending.nonce != 8 || m.reusableSnapshot() != nil {
			t.Fatalf("mined advancement did not release cooldown at fresh nonce: %+v", pending)
		}
	})
}

func TestNonceCooldownHonorsDeadlineAndContextCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "caller cancellation", true: "request deadline"}[deadline], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := newMockBackend()
				b.sendErrs = []error{errors.New("replacement transaction underpriced")}
				m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: 100 * time.Millisecond}, logr.Discard())
				if _, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000}); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				request := Request{To: common.Address{2}, GasLimit: 21_000}
				want := context.Canceled
				if deadline {
					request.Deadline, want = time.Now().Add(time.Second), context.DeadlineExceeded
				}
				result := make(chan error, 1)
				go func() {
					_, err := m.broadcast(ctx, request)
					result <- err
				}()
				synctest.Wait()
				if !deadline {
					cancel()
				}
				if err := <-result; !errors.Is(err, want) || len(b.attemptedTransactions()) != 1 {
					t.Fatalf("cooldown stop = %v, sends=%d; want %v and no signing", err, len(b.attemptedTransactions()), want)
				}
			})
		})
	}
}

func TestReplacementTransportFailureStillAlerts(t *testing.T) {
	b := newMockBackend()
	logs, log := newLogCapture(1)
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, log)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{To: common.Address{1}, GasLimit: 21_000})
	if err != nil {
		t.Fatal(err)
	}
	delete(b.receipts, pending.originalHash)
	b.sendErrs = []error{nil, io.ErrUnexpectedEOF}
	_, err = m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("transport error = %v", err)
	}
	if failures, _ := countLogs(*logs, "replacement broadcast uncertain; tracking signed hash"); failures != 1 {
		t.Fatal("replacement transport failure lost its Error alert")
	}
}

func TestUnderpricedExactRebroadcastPathsYieldWithoutErrorAlerts(t *testing.T) {
	for _, operation := range []string{"uncertain", "capped", "stalled"} {
		t.Run(operation, func(t *testing.T) {
			b := newMockBackend()
			logs, log := newLogCapture(1)
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, log)
			ctx := managerCtx(t.Context(), m)
			pending, err := m.broadcast(ctx, Request{To: common.Address{1}, GasLimit: 21_000})
			if err != nil {
				t.Fatal(err)
			}
			delete(b.receipts, pending.originalHash)
			b.sendErrs = []error{nil, errors.New("replacement transaction underpriced")}
			switch operation {
			case "uncertain":
				pending.attempts[0].exactRebroadcastPending = true
				m.rebroadcastUncertainAttempt(ctx, pending)
			case "capped":
				m.rebroadcastLatestAttempt(ctx, pending)
			case "stalled":
				m.rebroadcastStalledAttempt(ctx, pending)
			}
			m.tryReplace(ctx, pending, replaceIntent{})
			if len(b.attemptedTransactions()) != 2 {
				t.Fatal("rebroadcast contention did not stop later fee escalation")
			}
			for _, entry := range *logs {
				if !strings.Contains(entry, `"level":`) {
					t.Fatalf("expected rebroadcast contention produced Error alert: %s", entry)
				}
			}
		})
	}
}
