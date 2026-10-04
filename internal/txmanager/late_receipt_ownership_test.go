package txmanager

import (
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestLateReceiptLeavesNonceAndFeeOwnershipUnchanged(t *testing.T) {
	b := &laggingOwnedNonceBackend{mockBackend: newMockBackend(), nonce: 7}
	metrics := newTestMetrics(t)
	m := newLateReceiptManager(t, b, Config{Confirmations: 2}, metrics)
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000, Label: "passive"})
	if err != nil {
		t.Fatal(err)
	}
	m.rememberReusable(pending.nonce, pending.fees)
	m.retainLateReceipts(t.Context(), pending)
	b.head += 2
	beforeFloor, beforeReady, beforeFees := m.confirmedNonceFloor, m.LaneReady(), m.reusableSnapshot()
	entry, ok := m.nextLateReceipt()
	if !ok {
		t.Fatal("released request was not retained for passive observation")
	}
	if err := m.observeLateReceipt(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	if m.confirmedNonceFloor != beforeFloor || m.LaneReady() != beforeReady || !reflect.DeepEqual(beforeFees, m.reusableSnapshot()) {
		t.Fatal("passive telemetry changed nonce selection, lane readiness or the reusable fee hint")
	}
	if nonce, _, err := m.selectNonce(t.Context()); err != nil || nonce != 7 {
		t.Fatalf("passive receipt overrode the sending RPC nonce: nonce=%d err=%v", nonce, err)
	}
	assertMetric(t, metrics.lateReceipts.WithLabelValues("passive", "confirmed"), 1)
	assertMetric(t, metrics.requests.WithLabelValues("passive", "confirmed"), 0)
}

func TestLateReceiptIgnoresPeerHashAtOwnedNonce(t *testing.T) {
	b := newMockBackend()
	metrics := newTestMetrics(t)
	m := newLateReceiptManager(t, b, Config{}, metrics)
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000, Label: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	delete(b.receipts, pending.originalHash)
	peer := types.NewTx(&types.DynamicFeeTx{Nonce: pending.nonce, To: ptr(common.Address{2}), Gas: 21_000,
		GasFeeCap: big.NewInt(40_000_000_000), GasTipCap: big.NewInt(2_000_000_000)})
	putLateReceipt(b, peer, types.ReceiptStatusSuccessful, b.head)
	m.retainLateReceipts(t.Context(), pending)
	entry, ok := m.nextLateReceipt()
	if !ok {
		t.Fatal("owned hash was not retained")
	}
	if err := m.observeLateReceipt(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	assertMetric(t, metrics.lateReceipts.WithLabelValues("owned", "confirmed"), 0)
	assertMetric(t, metrics.gasUsed.WithLabelValues("owned", "confirmed"), 0)
	assertMetric(t, metrics.feePaidWei.WithLabelValues("owned", "confirmed"), 0)
}

func TestLateReceiptRejectsUnknownStatus(t *testing.T) {
	b := newMockBackend()
	metrics := newTestMetrics(t)
	m := newLateReceiptManager(t, b, Config{}, metrics)
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000, Label: "invalid"})
	if err != nil {
		t.Fatal(err)
	}
	putLateReceipt(b, pending.attempts[0].tx, 2, b.head)
	m.retainLateReceipts(t.Context(), pending)
	entry, ok := m.nextLateReceipt()
	if !ok {
		t.Fatal("owned hash was not retained")
	}
	result, err := m.readLateReceipt(t.Context(), entry)
	if err == nil || !strings.Contains(err.Error(), "invalid status 2") || result.Receipt != nil {
		t.Fatalf("malformed receipt entered passive telemetry: result=%+v err=%v", result, err)
	}
	assertMetric(t, metrics.lateReceipts.WithLabelValues("invalid", "confirmed"), 0)
}
