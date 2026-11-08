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

type delayedWitnessBackend struct {
	*witnessBackend

	delay time.Duration
}

func (b *delayedWitnessBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(b.delay):
		return b.witnessBackend.HeaderByNumber(ctx, number)
	}
}

func (b *delayedWitnessBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(b.delay):
		return b.witnessBackend.TransactionReceipt(ctx, hash)
	}
}

func TestCanonicalPreflightAllowsThreeReadBudgets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
		b := &delayedWitnessBackend{witnessBackend: newWitnessBackend(tx, 0), delay: 700 * time.Millisecond}
		m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
		result, done := m.confirmPendingReceipt(t.Context(), &pendingTransaction{
			req: Request{Label: "slow-inclusion"}, nonce: tx.Nonce(),
		}, txAttempt{hash: tx.Hash()}, b.receipts[tx.Hash()])
		if !done || result.Outcome != OutcomeConfirmed || m.confirmedNonceFloor != tx.Nonce()+1 {
			t.Fatalf("three healthy 700ms reads did not confirm inclusion: %+v, done=%t", result, done)
		}
	})
}

func TestLateReceiptFitsPollBudgetWithThreeReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
		b := &delayedWitnessBackend{witnessBackend: newWitnessBackend(tx, 2), delay: 400 * time.Millisecond}
		m := newLateReceiptManager(t, b, Config{Confirmations: 2}, nil)
		observed := make(chan Result, 1)
		m.retainLateReceipts(t.Context(), &pendingTransaction{
			req:   Request{Label: "slow-late", ObserveReceipt: func(_ context.Context, result Result) { observed <- result }},
			nonce: tx.Nonce(), attempts: []txAttempt{{hash: tx.Hash()}},
		})
		m.pollLateReceipts(t.Context())
		select {
		case result := <-observed:
			if result.Outcome != OutcomeConfirmed || result.Hash != tx.Hash() {
				t.Fatalf("late inclusion = %+v", result)
			}
		default:
			t.Fatal("healthy 400ms reads exhausted the passive poll budget")
		}
		if b.receiptReads != 1 || b.numberedReads != 2 {
			t.Fatalf("late proof reads: receipts=%d numbered=%d", b.receiptReads, b.numberedReads)
		}
	})
}

func TestConfirmationAcceptsOmittedLogMetadata(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
	b := newWitnessBackend(tx, 2)
	b.receipts[tx.Hash()].Logs = []*types.Log{{Data: []byte{1}}}
	m := newLateReceiptManager(t, b, Config{}, nil)
	result, done := m.confirmPendingReceipt(t.Context(), &pendingTransaction{
		nonce: tx.Nonce(),
	}, txAttempt{hash: tx.Hash()}, b.receipts[tx.Hash()])
	if !done || result.Outcome != OutcomeConfirmed {
		t.Fatalf("optional log metadata prevented active confirmation: %+v, done=%t", result, done)
	}
	late, err := m.readLateReceipt(t.Context(), lateReceiptObservation{hash: tx.Hash()})
	if err != nil || late.Outcome != OutcomeConfirmed {
		t.Fatalf("optional log metadata prevented passive confirmation: %+v, %v", late, err)
	}
}
