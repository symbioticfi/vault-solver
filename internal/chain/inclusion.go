package chain

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

// InclusionReorged checks a retained inclusion against the canonical header at its height.
// Only a different valid header proves that the inclusion was removed. Missing blocks and RPC
// errors are inconclusive: a provider may lag the head that originally reported the receipt.
func InclusionReorged(ctx context.Context, backend interface {
	HeaderByNumber(ctx context.Context, block *big.Int) (*types.Header, error)
}, block uint64, hash common.Hash) (bool, error) {
	if hash == (common.Hash{}) {
		return false, errors.New("inclusion block hash is required")
	}
	header, err := backend.HeaderByNumber(ctx, new(big.Int).SetUint64(block))
	if err != nil {
		return false, errors.Errorf("canonical inclusion block %d: %w", block, err)
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() || header.Number.Uint64() != block {
		return false, errors.Errorf("canonical inclusion block %d has an invalid header", block)
	}
	return header.Hash() != hash, nil
}

// ReconcileInclusion checks a previously successful receipt. If its block was replaced, the same
// transaction may have been included again elsewhere; prefer that canonical placement to a retry.
// A missing refreshed receipt only permits reopening after the original block is proven replaced.
func ReconcileInclusion(ctx context.Context, backend interface {
	HeaderByNumber(ctx context.Context, block *big.Int) (*types.Header, error)
	TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
}, original *types.Receipt) (bool, *types.Receipt, error) {
	if original == nil || original.BlockNumber == nil || !original.BlockNumber.IsUint64() ||
		original.TxHash == (common.Hash{}) || original.Status != types.ReceiptStatusSuccessful {
		return false, nil, errors.New("successful inclusion receipt is required")
	}
	reorged, err := InclusionReorged(ctx, backend, original.BlockNumber.Uint64(), original.BlockHash)
	if err != nil {
		return false, nil, err
	}
	if !reorged {
		return false, original, nil
	}
	refreshed, err := backend.TransactionReceipt(ctx, original.TxHash)
	if errors.Is(err, ethereum.NotFound) {
		return true, nil, nil
	}
	if err != nil {
		return false, nil, errors.Errorf("recheck orphaned transaction %s: %w", original.TxHash.Hex(), err)
	}
	if refreshed == nil || refreshed.BlockNumber == nil || !refreshed.BlockNumber.IsUint64() ||
		refreshed.TxHash != original.TxHash || refreshed.Status > types.ReceiptStatusSuccessful {
		return false, nil, errors.New("refreshed inclusion receipt is invalid")
	}
	reorged, err = InclusionReorged(ctx, backend, refreshed.BlockNumber.Uint64(), refreshed.BlockHash)
	if err != nil {
		return false, nil, err
	}
	if reorged {
		return false, nil, errors.New("refreshed inclusion receipt is not canonical")
	}
	if refreshed.Status == types.ReceiptStatusFailed {
		return true, nil, nil
	}
	return false, refreshed, nil
}
