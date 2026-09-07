package txmanager

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestTrySendClaimBeforeWorkerStarts(t *testing.T) {
	for _, cancelBeforeStart := range []bool{false, true} {
		name := "start worker"
		if cancelBeforeStart {
			name = "cancel caller"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				backend := newMockBackend()
				manager := newTestManager(t, backend, Config{PollInterval: time.Millisecond})
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				request := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000}
				done := make(chan bool, 1)
				go func() {
					result, accepted := manager.TrySend(ctx, request)
					if result.Err != nil {
						t.Errorf("first TrySend: %v", result.Err)
					}
					done <- accepted
				}()
				synctest.Wait()
				if _, accepted := manager.TrySend(t.Context(), request); accepted {
					t.Fatal("second request bypassed the waiting claim")
				}
				if backend.lastSent() != nil {
					t.Fatal("transaction sent before the worker started")
				}
				if cancelBeforeStart {
					cancel()
				} else {
					startManagerForTest(t, manager)
				}
				if accepted := <-done; accepted == cancelBeforeStart {
					t.Fatalf("first TrySend accepted = %v", accepted)
				}
				synctest.Wait()
				if !manager.Idle() {
					t.Fatal("completed admission left demand on the lane")
				}
				if cancelBeforeStart {
					startManagerForTest(t, manager)
					result, accepted := manager.TrySend(t.Context(), request)
					if !accepted || result.Err != nil || result.Receipt == nil {
						t.Fatalf("TrySend after cancelled claim = (%+v, %v)", result, accepted)
					}
				}
			})
		})
	}
}

func TestCancellationDuringFeeReadDoesNotImmediatelyBumpAgain(t *testing.T) {
	for _, reason := range []string{"pending_timeout", "request_deadline", "shutdown"} {
		t.Run(reason, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs []string
				logger := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
				backend := &replacementBackend{mockBackend: newMockBackend()}
				var blockNextFeeRead bool
				rpc := &headerBackend{Backend: backend, read: func(ctx context.Context, number *big.Int) (*types.Header, error) {
					if number == nil && blockNextFeeRead {
						blockNextFeeRead = false
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return backend.HeaderByNumber(ctx, number)
				}}
				manager := New(rpc, mustSigner(t), big.NewInt(1), Config{
					MaxFeeGwei: 100, PollInterval: time.Hour,
					ReplacementInterval: 40 * time.Millisecond, PendingTimeout: 50 * time.Millisecond,
				}, nil, logger)
				request := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill"}
				if reason != "pending_timeout" {
					manager.cfg.PendingTimeout = time.Hour
					if reason == "request_deadline" {
						request.CancelAt = time.Now().Add(50 * time.Millisecond)
					}
				}
				pending, err := manager.broadcast(t.Context(), request)
				testcheck.NoError(t, err)
				manager.trackUnminedTransaction(pending)
				if reason == "shutdown" {
					timer := time.AfterFunc(50*time.Millisecond, func() { requestCancellation(pending) })
					defer timer.Stop()
				}
				blockNextFeeRead = true
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				result := make(chan Result, 1)
				go func() { result <- manager.waitForPendingTransaction(ctx, pending) }()

				// The 40ms replacement's fee read times out at 60ms, after the 50ms
				// cancellation deadline or shutdown signal. The next replacement is at 80ms.
				time.Sleep(70 * time.Millisecond)
				synctest.Wait()
				backend.mu.Lock()
				sentBeforeNextTick := len(backend.sent)
				backend.mu.Unlock()
				if sentBeforeNextTick != 2 {
					t.Fatalf("sent %d transactions before the next replacement tick, want original plus one cancellation", sentBeforeNextTick)
				}
				time.Sleep(20 * time.Millisecond)
				synctest.Wait()
				cancel()
				<-result
				if len(backend.sent) != 3 {
					t.Fatalf("sent %d transactions after the next replacement tick, want original plus two cancellations", len(backend.sent))
				}
				for _, cancellation := range backend.sent[1:] {
					if cancellation.Nonce() != pending.nonce || cancellation.To() == nil ||
						*cancellation.To() != manager.signer.Address() || len(cancellation.Data()) != 0 ||
						cancellation.Gas() != cancellationGasLimit || cancellation.Value().Sign() != 0 {
						t.Fatalf("replacement was not a same-nonce cancellation: %v", cancellation)
					}
				}
				assertCancellationLog(t, logs, pending.originalHash, pending.cancelDeadline, reason)
			})
		})
	}
}

func TestReceiptReadTimeoutDoesNotCancelFill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := newMockBackend()
		var timedOut bool
		rpc := &receiptBackend{Backend: backend, read: func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
			if !timedOut {
				timedOut = true
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return backend.TransactionReceipt(ctx, hash)
		}}
		manager := New(rpc, mustSigner(t), big.NewInt(1), Config{
			MaxFeeGwei: 100, PollInterval: time.Second,
			ReplacementInterval: 30 * time.Second, PendingTimeout: 5 * time.Minute,
		}, nil, logr.Discard())
		pending, err := manager.broadcast(t.Context(), Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill",
			CancelAt: time.Now().Add(time.Minute),
		})
		testcheck.NoError(t, err)
		manager.trackUnminedTransaction(pending)
		result := manager.waitForPendingTransaction(t.Context(), pending)
		if result.Outcome != OutcomeConfirmed || result.Err != nil || result.Hash != pending.originalHash {
			t.Fatalf("receipt retry result = %+v, want original fill confirmed", result)
		}
		if len(backend.sent) != 1 {
			t.Fatalf("sent %d transactions after a receipt RPC timeout, want original fill only", len(backend.sent))
		}
	})
}

func assertCancellationLog(t *testing.T, logs []string, hash common.Hash, deadline time.Time, reason string) {
	t.Helper()
	count := 0
	for _, entry := range logs {
		var fields map[string]json.RawMessage
		testcheck.NoError(t, json.Unmarshal([]byte(entry), &fields))
		if string(fields["msg"]) != `"pending transaction cancellation requested"` {
			continue
		}
		count++
		for key, want := range map[string]string{
			"reason": reason, "hash": hash.Hex(), "deadline": deadline.UTC().Format(time.RFC3339Nano),
		} {
			var got string
			if err := json.Unmarshal(fields[key], &got); err != nil || got != want {
				t.Fatalf("cancellation log %s = %s, want %q", key, fields[key], want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("got %d cancellation-reason logs, want exactly one; logs: %v", count, logs)
	}
}
