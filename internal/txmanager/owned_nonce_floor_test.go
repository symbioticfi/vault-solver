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

// Only the sending RPC's latest account-nonce view lags; canonical receipt/header reads stay fresh.
type laggingOwnedNonceBackend struct {
	*mockBackend

	nonce uint64
}

func (b *laggingOwnedNonceBackend) NonceAt(ctx context.Context, account common.Address, block *big.Int) (uint64, error) {
	if block == nil {
		return b.nonce, nil
	}
	return b.mockBackend.NonceAt(ctx, account, block)
}

func TestConfirmedOwnedReceiptPreventsLaggingRPCFromReusingNonce(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     uint64
		minedNonce uint64
		wantNonce  uint64
	}{
		{"confirmed successful", types.ReceiptStatusSuccessful, 7, 8},
		{"confirmed reverted", types.ReceiptStatusFailed, 7, 8},
		{"fresh RPC above floor", types.ReceiptStatusSuccessful, 12, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &laggingOwnedNonceBackend{mockBackend: newMockBackend(), nonce: 7}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, Confirmations: 2}, logr.Discard())
			pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000})
			if err != nil {
				t.Fatal(err)
			}
			b.receipts[pending.originalHash].Status = tc.status
			b.head += 2
			result, done := m.receiptResult(t.Context(), pending)
			if !done || (result.Outcome != OutcomeConfirmed && result.Outcome != OutcomeReverted) {
				t.Fatalf("owned receipt = %+v, %v; want terminal canonical receipt", result, done)
			}
			b.nonce = tc.minedNonce
			fresh, err := m.broadcast(t.Context(), Request{To: common.Address{2}, GasLimit: 21_000})
			if err != nil || fresh == nil || fresh.nonce != tc.wantNonce {
				t.Fatalf("fresh nonce after owned receipt = %+v, %v; want %d", fresh, err, tc.wantNonce)
			}
		})
	}
}

func TestUnconfirmedOwnedReceiptDoesNotAdvanceNonceFloor(t *testing.T) {
	for _, status := range []uint64{types.ReceiptStatusSuccessful, types.ReceiptStatusFailed} {
		t.Run(map[uint64]string{types.ReceiptStatusSuccessful: "successful", types.ReceiptStatusFailed: "reverted"}[status], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &laggingOwnedNonceBackend{mockBackend: newMockBackend(), nonce: 7}
				m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard())
				pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000})
				if err != nil {
					t.Fatal(err)
				}
				b.receipts[pending.originalHash].Status = status
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
				defer cancel()
				result, done := m.receiptResult(ctx, pending)
				if !done || result.Err == nil || result.Receipt == nil {
					t.Fatalf("unconfirmed owned receipt = %+v, %v; want failed confirmation wait", result, done)
				}
				fresh, err := m.broadcast(t.Context(), Request{To: common.Address{2}, GasLimit: 21_000})
				if err != nil || fresh == nil || fresh.nonce != 7 {
					t.Fatalf("fresh nonce after unconfirmed receipt = %+v, %v; want mined nonce 7", fresh, err)
				}
			})
		})
	}
}

func TestOrphanedOwnedReceiptDoesNotAdvanceNonceFloor(t *testing.T) {
	b := &laggingOwnedNonceBackend{mockBackend: newMockBackend(), nonce: 7}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, Confirmations: 2}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000})
	if err != nil {
		t.Fatal(err)
	}
	b.head += 2
	b.reorgedHeader = true
	if result, done := m.receiptResult(t.Context(), pending); done {
		t.Fatalf("orphaned receipt completed = %+v", result)
	}
	fresh, err := m.broadcast(t.Context(), Request{To: common.Address{2}, GasLimit: 21_000})
	if err != nil || fresh == nil || fresh.nonce != 7 {
		t.Fatalf("fresh nonce after orphaned receipt = %+v, %v; want mined nonce 7", fresh, err)
	}
}
