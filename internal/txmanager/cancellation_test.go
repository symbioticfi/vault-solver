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
)

func TestCancellationDuringFeeReadDoesNotImmediatelyBumpAgain(t *testing.T) {
	for _, requestDeadline := range []bool{false, true} {
		name := "pending timeout"
		if requestDeadline {
			name = "request deadline"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs []string
				logger := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
				backend := &deadlineFeeReadBackend{replacementBackend: &replacementBackend{
					mockBackend: newMockBackend(),
				}}
				manager := New(backend, mustSigner(t), big.NewInt(1), Config{
					MaxFeeGwei: 100, PollInterval: time.Hour,
					ReplacementInterval: 40 * time.Millisecond, PendingTimeout: 50 * time.Millisecond,
				}, logger)
				request := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill"}
				if requestDeadline {
					manager.cfg.PendingTimeout = time.Hour
					request.CancelAt = time.Now().Add(50 * time.Millisecond)
				}
				pending, err := manager.broadcast(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				manager.trackUnminedTransaction(pending)
				backend.blockNextFeeRead = true
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				result := make(chan Result, 1)
				go func() { result <- manager.waitForPendingTransaction(ctx, pending) }()

				// The 40ms replacement's fee read times out at 60ms, after the 50ms
				// cancellation deadline. The next scheduled replacement is at 80ms.
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
				cancellation := backend.sent[1]
				if cancellation.Nonce() != pending.nonce || cancellation.To() == nil ||
					*cancellation.To() != manager.signer.Address() || len(cancellation.Data()) != 0 {
					t.Fatalf("replacement was not a same-nonce cancellation: %v", cancellation)
				}
				wantReason := "pending_timeout"
				if requestDeadline {
					wantReason = "request_deadline"
				}
				assertCancellationLog(t, logs, pending.originalHash, pending.cancelDeadline, wantReason)
			})
		})
	}
}

func TestReceiptReadTimeoutDoesNotCancelFill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := &receiptDeadlineBackend{mockBackend: newMockBackend()}
		manager := New(backend, mustSigner(t), big.NewInt(1), Config{
			MaxFeeGwei: 100, PollInterval: time.Second,
			ReplacementInterval: 30 * time.Second, PendingTimeout: 5 * time.Minute,
		}, logr.Discard())
		pending, err := manager.broadcast(t.Context(), Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill",
			CancelAt: time.Now().Add(time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
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
		if err := json.Unmarshal([]byte(entry), &fields); err != nil {
			t.Fatal(err)
		}
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

type deadlineFeeReadBackend struct {
	*replacementBackend

	blockNextFeeRead bool
}

type receiptDeadlineBackend struct {
	*mockBackend

	timedOut bool
}

func (b *deadlineFeeReadBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if number == nil && b.blockNextFeeRead {
		b.blockNextFeeRead = false
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.mockBackend.HeaderByNumber(ctx, number)
}

func (b *receiptDeadlineBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	if !b.timedOut {
		b.timedOut = true
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.mockBackend.TransactionReceipt(ctx, hash)
}
