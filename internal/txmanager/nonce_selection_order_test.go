package txmanager

import (
	"context"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type siblingEstimateBackend struct {
	*mockBackend

	minedBeforeEstimate bool
	nonceReads          int
	estimateNonceReads  int
}

func (b *siblingEstimateBackend) NonceAt(ctx context.Context, account common.Address, block *big.Int) (uint64, error) {
	b.nonceReads++
	return b.mockBackend.NonceAt(ctx, account, block)
}

func (b *siblingEstimateBackend) EstimateGasNextBlock(
	_ context.Context, _ ethereum.CallMsg, _ *types.Header, _ time.Duration,
) (uint64, error) {
	b.estimateNonceReads = b.nonceReads
	// A sibling consumes N=7 either immediately before or immediately after the simulated call.
	b.latestNonce = 8
	if b.minedBeforeEstimate {
		return 0, &estimateRPCError{3, "execution reverted", "0x1f6d5aef"}
	}
	return 50_000, nil
}

func (b *siblingEstimateBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if tx.Nonce() < b.latestNonce {
		b.sendErrs = append(b.sendErrs, errors.New("nonce too low"))
	}
	return b.mockBackend.SendTransaction(ctx, tx)
}

func TestBroadcastSelectsNonceBeforeEstimateReconcilesSiblingFill(t *testing.T) {
	b := &siblingEstimateBackend{mockBackend: newMockBackend(), minedBeforeEstimate: true}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	// The stale backend can still report open, but the execution simulation sees the consumed order.
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, Obsolete: func(context.Context) (bool, error) { return false, nil }})
	var revert *ExecutionRevertError
	if pending != nil || !errors.As(err, &revert) || len(b.attemptedTransactions()) != 0 {
		t.Fatalf("sibling-mined estimate = %+v, %v; want unsigned execution revert", pending, err)
	}
	if b.estimateNonceReads != 1 {
		t.Fatalf("nonce reads before simulation = %d; want one", b.estimateNonceReads)
	}
}

func TestBroadcastKeepsSelectedNonceWhenSiblingMinesAfterEstimate(t *testing.T) {
	b := &siblingEstimateBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, Obsolete: func(context.Context) (bool, error) { return false, nil }})
	if err != nil || pending == nil || pending.nonce != 7 || !isNonceConsumedError(pending.broadcastErr) {
		t.Fatalf("sibling-mined broadcast = %+v, %v; want nonce 7 consumed race", pending, err)
	}
	if b.nonceReads != 1 || b.estimateNonceReads != 1 || len(b.attemptedTransactions()) != 1 {
		t.Fatalf("broadcast read/sign sequence: nonce reads=%d before estimate=%d attempts=%d", b.nonceReads, b.estimateNonceReads, len(b.attemptedTransactions()))
	}
}
