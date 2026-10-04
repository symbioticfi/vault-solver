package txmanager

import (
	"context"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

func TestReceiptReadTimeoutDoesNotCancelFill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := &receiptDeadlineBackend{mockBackend: newMockBackend()}
		manager := New(backend, mustSigner(t), big.NewInt(1), Config{
			MaxFeeGwei: 100, PollInterval: time.Second,
			ReplacementInterval: 30 * time.Second, PendingTimeout: 5 * time.Minute,
		}, logr.Discard())
		pending, err := manager.broadcast(t.Context(), Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill",
			Deadline: time.Now().Add(time.Minute),
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

type receiptDeadlineBackend struct {
	*mockBackend

	timedOut bool
}

func (b *receiptDeadlineBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	if !b.timedOut {
		b.timedOut = true
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.mockBackend.TransactionReceipt(ctx, hash)
}
