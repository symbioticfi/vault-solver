package txmanager

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestSend_HappyPath(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "test"})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("expected successful receipt, got %+v", res.Receipt)
	}

	tx := b.lastSent()
	if tx == nil {
		t.Fatal("no transaction sent")
	}
	if tx.Nonce() != 7 {
		t.Fatalf("expected nonce 7 (seeded from pending), got %d", tx.Nonce())
	}
	if tx.Type() != types.DynamicFeeTxType {
		t.Fatalf("expected EIP-1559 tx, got type %d", tx.Type())
	}
	// gas = estimate + 5%
	if tx.Gas() != 52_500 {
		t.Fatalf("expected gas 52500 (50000 + 5%%), got %d", tx.Gas())
	}
}
