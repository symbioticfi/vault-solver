package txmanager

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

func TestReceiptReorgKeepsLifecyclePending(t *testing.T) {
	tests := map[string]func(*mockBackend) Backend{
		"receipt disappears": func(b *mockBackend) Backend {
			return &disappearingReceiptBackend{mockBackend: b}
		},
		"receipt reorgs during head read": func(b *mockBackend) Backend {
			b.reorgOnHeadRead = true
			return b
		},
		"receipt block is no longer canonical": func(b *mockBackend) Backend {
			b.reorgedHeader = true
			return b
		},
	}
	for name, backend := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			m := New(
				backend(b), mustSigner(t), big.NewInt(11155111),
				Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
			)
			tx := types.NewTx(&types.DynamicFeeTx{
				ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
				Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
			})
			b.receipts[tx.Hash()] = successfulReceipt(tx, b.head-2)
			pending := &pendingTransaction{
				req: Request{Label: "reorged"}, nonce: 7,
				attempts: []txAttempt{{hash: tx.Hash(), tx: tx}},
			}
			m.trackUnminedTransaction(pending)

			if result, done := m.receiptResult(t.Context(), pending); done {
				t.Fatalf("reorged receipt completed lifecycle: %+v", result)
			}
			m.unminedMu.Lock()
			tracked := m.unmined == pending
			m.unminedMu.Unlock()
			if !tracked {
				t.Fatal("reorged lifecycle lost active ownership")
			}
		})
	}
}

func TestConfirmationsRequireStableHead(t *testing.T) {
	b := newMockBackend()
	b.head = 102
	b.latestHeads = []uint64{102, 100, 102, 102}
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	receipt := successfulReceipt(tx, 100)
	b.receipts[tx.Hash()] = receipt
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)

	got, err := m.waitForConfirmations(t.Context(), tx.Hash(), receipt, 2)
	if err != nil || got != receipt {
		t.Fatalf("waitForConfirmations = (%+v, %v), want stable confirmed receipt", got, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.latestHeads) != 0 {
		t.Fatalf("confirmation returned before stable head snapshot; unread heads = %v", b.latestHeads)
	}
	if b.headerHashReads != 4 {
		t.Fatalf("ancestry reads = %d, want 4 before both final head checks", b.headerHashReads)
	}
}

func TestConfirmationsRejectReceiptFromDifferentFork(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	receipt := successfulReceipt(tx, 100)
	receipt.BlockHash = forkedReceiptHeader(100, "fallback").Hash()
	backend := &mixedForkBackend{mockBackend: newMockBackend()}
	backend.receipts[tx.Hash()] = receipt
	m := New(
		backend, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)

	got, err := m.waitForConfirmations(t.Context(), tx.Hash(), receipt, 2)
	if got != receipt || !errors.Is(err, errReceiptReorged) {
		t.Fatalf("waitForConfirmations = (%+v, %v), want reorg error", got, err)
	}
}

func TestTransientReceiptErrorKeepsTrackingPendingTransaction(t *testing.T) {
	b := &receiptErrorBackend{mockBackend: newMockBackend(), failures: 1}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			MaxFeeGwei:          100,
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Second,
			PendingTimeout:      time.Second,
		},
		logr.Discard(),
	)
	startTestManager(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "receipt retry",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	if got := <-result; got.Err != nil {
		t.Fatalf("receipt retry result: %v", got.Err)
	}
}

func TestReceiptLookupTimeoutDoesNotStarveOlderAttempt(t *testing.T) {
	b := newMockBackend()
	older := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	newest := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(3),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	b.receipts[older.Hash()] = successfulReceipt(older, b.head)
	backend := &blockedReceiptHashBackend{mockBackend: b, hash: newest.Hash()}
	m := New(
		backend, mustSigner(t), big.NewInt(11155111),
		Config{ReplacementInterval: 2 * time.Millisecond}, logr.Discard(),
	)
	pending := &pendingTransaction{
		req: Request{Label: "fair receipt lookup"}, nonce: 7,
		attempts: []txAttempt{{hash: older.Hash()}, {hash: newest.Hash()}},
		// Exercise the slow newest hash first; the next poll must resume at the older attempt.
		receiptCursor: 1,
	}
	if result, done := m.receiptResult(t.Context(), pending); done {
		t.Fatalf("slow newest lookup completed lifecycle: %+v", result)
	}
	result, done := m.receiptResult(t.Context(), pending)
	if !done || result.Err != nil || result.Hash != older.Hash() || result.Receipt != b.receipts[older.Hash()] {
		t.Fatalf("older mined attempt result = (%+v, %v)", result, done)
	}
}

func TestMalformedReceiptDoesNotCompleteLifecycle(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	tests := map[string]func(*types.Receipt){
		"mismatched transaction hash": func(receipt *types.Receipt) {
			receipt.TxHash = common.HexToHash("0x1234")
		},
		"missing block hash": func(receipt *types.Receipt) {
			receipt.BlockHash = common.Hash{}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			receipt := successfulReceipt(tx, b.head)
			mutate(receipt)
			b.receipts[tx.Hash()] = receipt
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
			pending := &pendingTransaction{
				req: Request{Label: "malformed receipt"}, nonce: 7,
				attempts: []txAttempt{{hash: tx.Hash()}},
			}
			m.trackUnminedTransaction(pending)

			if result, done := m.receiptResult(t.Context(), pending); done {
				t.Fatalf("malformed receipt completed lifecycle: %+v", result)
			}
			m.unminedMu.Lock()
			tracked := m.unmined == pending
			m.unminedMu.Unlock()
			if !tracked {
				t.Fatal("malformed receipt released the pending nonce")
			}
		})
	}
}
