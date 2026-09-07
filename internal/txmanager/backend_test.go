package txmanager

import (
	"context"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

// RPC overrides are fixed before a scenario starts. Callback-owned state must obey
// the same synchronization rules as the backend it wraps.
type receiptBackend struct {
	Backend

	read func(context.Context, common.Hash) (*types.Receipt, error)
}

func (b *receiptBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	return b.read(ctx, hash)
}

type headerBackend struct {
	Backend

	read func(context.Context, *big.Int) (*types.Header, error)
}

func (b *headerBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return b.read(ctx, number)
}

func failReceiptReads(base Backend, count int64) *receiptBackend {
	var calls atomic.Int64
	return &receiptBackend{Backend: base, read: func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
		if calls.Add(1) <= count {
			return nil, errors.New("temporary receipt failure")
		}
		return base.TransactionReceipt(ctx, hash)
	}}
}

func disappearingReceipts(base Backend) *receiptBackend {
	var calls atomic.Int64
	return &receiptBackend{Backend: base, read: func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
		if calls.Add(1) > 1 {
			return nil, ethereum.NotFound
		}
		return base.TransactionReceipt(ctx, hash)
	}}
}

func failHeadReads(base Backend, failures *atomic.Int64) *headerBackend {
	return &headerBackend{Backend: base, read: func(ctx context.Context, number *big.Int) (*types.Header, error) {
		if failures.Add(-1) >= 0 {
			return nil, errors.New("temporary head failure")
		}
		return base.HeaderByNumber(ctx, number)
	}}
}
