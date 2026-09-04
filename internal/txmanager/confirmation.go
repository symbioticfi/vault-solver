package txmanager

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

func (m *Manager) waitForConfirmations(
	ctx context.Context,
	hash common.Hash,
	receipt *types.Receipt,
	required uint64,
) (*types.Receipt, error) {
	if receipt == nil || receipt.BlockNumber == nil {
		return receipt, errors.New("receipt block number is required")
	}
	if required == 0 {
		return receipt, nil
	}
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		refreshed, confirmed, err := m.checkConfirmation(ctx, hash, receipt, required)
		receipt = refreshed
		if err != nil || confirmed {
			return receipt, err
		}
		select {
		case <-ctx.Done():
			return receipt, context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

func (m *Manager) checkConfirmation(
	ctx context.Context,
	hash common.Hash,
	receipt *types.Receipt,
	required uint64,
) (*types.Receipt, bool, error) {
	head, err := m.header(ctx, nil)
	if err != nil {
		m.log.Error(err, "confirmation head unavailable", "hash", hash.Hex())
		return receipt, false, nil
	}
	refreshed, err := m.confirmationReceipt(ctx, hash)
	if err != nil {
		if errors.Is(err, errReceiptReorged) {
			return receipt, false, err
		}
		m.log.Error(err, "receipt confirmation check unavailable", "hash", hash.Hex())
		return receipt, false, nil
	}
	if !hasConfirmationDepth(head, refreshed, required) {
		return refreshed, false, nil
	}
	if err := m.canonicalAtStableHead(ctx, head, refreshed); err != nil {
		if errors.Is(err, errReceiptReorged) {
			return refreshed, false, err
		}
		m.log.Error(err, "receipt canonicality check unavailable", "hash", hash.Hex())
		return refreshed, false, nil
	}
	return refreshed, true, nil
}

func hasConfirmationDepth(head *types.Header, receipt *types.Receipt, required uint64) bool {
	if head == nil || head.Number == nil || receipt == nil || receipt.BlockNumber == nil ||
		!head.Number.IsUint64() || !receipt.BlockNumber.IsUint64() {
		return false
	}
	latest, included := head.Number.Uint64(), receipt.BlockNumber.Uint64()
	return latest >= included && latest-included >= required
}

func (m *Manager) confirmationReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	receipt, err := m.backend.TransactionReceipt(readCtx, hash)
	if errors.Is(err, ethereum.NotFound) {
		return nil, errors.Errorf("%w: receipt %s disappeared", errReceiptReorged, hash.Hex())
	}
	if err != nil {
		return nil, errors.Errorf("transaction receipt %s: %w", hash.Hex(), err)
	}
	if err := validateReceipt(hash, receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (m *Manager) confirmCanonicalReceipt(ctx context.Context, receipt *types.Receipt) error {
	head, err := m.header(ctx, nil)
	if err != nil {
		return errors.Errorf("confirmation head: %w", err)
	}
	return m.canonicalAtStableHead(ctx, head, receipt)
}

func (m *Manager) canonicalAtStableHead(
	ctx context.Context,
	head *types.Header,
	receipt *types.Receipt,
) error {
	if head == nil || head.Number == nil || receipt == nil || receipt.BlockNumber == nil {
		return errors.New("canonical receipt check requires head and receipt block numbers")
	}
	included, err := m.header(ctx, receipt.BlockNumber)
	if err != nil {
		return errors.Errorf("receipt block %s: %w", receipt.BlockNumber, err)
	}
	if included.Hash() != receipt.BlockHash {
		return errors.Errorf("%w: receipt block %s is no longer canonical", errReceiptReorged, receipt.BlockHash.Hex())
	}
	latest, err := m.header(ctx, nil)
	if err != nil {
		return errors.Errorf("stable confirmation head: %w", err)
	}
	if latest.Hash() != head.Hash() {
		return errors.New("confirmation head changed during canonical receipt check")
	}
	return nil
}

func (m *Manager) header(ctx context.Context, number *big.Int) (*types.Header, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	header, err := m.backend.HeaderByNumber(readCtx, number)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Number == nil {
		return nil, errors.New("header number is required")
	}
	return header, nil
}

func validateReceipt(hash common.Hash, receipt *types.Receipt) error {
	if receipt == nil || receipt.BlockNumber == nil {
		return errors.Errorf("transaction receipt %s has no block number", hash.Hex())
	}
	if receipt.TxHash != hash {
		return errors.Errorf("transaction receipt %s returned mismatched hash %s", hash.Hex(), receipt.TxHash.Hex())
	}
	if receipt.BlockHash == (common.Hash{}) {
		return errors.Errorf("transaction receipt %s has no block hash", hash.Hex())
	}
	return nil
}
