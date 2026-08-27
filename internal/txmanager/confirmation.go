package txmanager

import (
	"context"
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
	confirmations uint64,
) (*types.Receipt, error) {
	if receipt == nil || receipt.BlockNumber == nil {
		return receipt, errors.New("receipt block number is required")
	}
	if confirmations == 0 {
		return receipt, nil
	}
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		headBefore, headErr := m.confirmationHead(ctx)
		if headErr != nil {
			m.log.Error(headErr, "confirmation head unavailable", "hash", hash.Hex())
		}
		refreshed, err := m.confirmationReceipt(ctx, hash)
		if err != nil {
			if errors.Is(err, errReceiptReorged) {
				return receipt, err
			}
			m.log.Error(err, "receipt confirmation check unavailable", "hash", hash.Hex())
		} else {
			receipt = refreshed
			if headErr == nil {
				head := headBefore.Number.Uint64()
				included := receipt.BlockNumber.Uint64()
				if head >= included && head-included >= confirmations {
					if err := m.confirmReceiptAncestry(ctx, headBefore, receipt); err != nil {
						if errors.Is(err, errReceiptReorged) {
							return receipt, err
						}
						m.log.Error(err, "receipt ancestry check unavailable", "hash", hash.Hex())
					} else {
						headAfter, afterErr := m.confirmationHead(ctx)
						if afterErr != nil {
							m.log.Error(afterErr, "confirmation head unavailable", "hash", hash.Hex())
						} else if headBefore.Hash() == headAfter.Hash() {
							return receipt, nil
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return receipt, context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

func (m *Manager) confirmationHead(ctx context.Context) (*types.Header, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	header, err := m.backend.HeaderByNumber(lookupCtx, nil)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Number == nil {
		return nil, errors.New("latest header number is required")
	}
	return header, nil
}

func (m *Manager) confirmationReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	receiptCtx, cancelReceipt := context.WithTimeout(ctx, m.receiptReadTimeout())
	receipt, err := m.backend.TransactionReceipt(receiptCtx, hash)
	cancelReceipt()
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
	headBefore, err := m.confirmationHead(ctx)
	if err != nil {
		return errors.Errorf("confirmation head before ancestry check: %w", err)
	}
	if err := m.confirmReceiptAncestry(ctx, headBefore, receipt); err != nil {
		return err
	}
	headAfter, err := m.confirmationHead(ctx)
	if err != nil {
		return errors.Errorf("confirmation head after ancestry check: %w", err)
	}
	if headBefore.Hash() != headAfter.Hash() {
		return errors.New("confirmation head changed during ancestry check")
	}
	return nil
}

func (m *Manager) confirmReceiptAncestry(
	ctx context.Context,
	head *types.Header,
	receipt *types.Receipt,
) error {
	if head == nil || head.Number == nil || receipt == nil || receipt.BlockNumber == nil {
		return errors.New("confirmation ancestry requires head and receipt block numbers")
	}
	if !head.Number.IsUint64() || !receipt.BlockNumber.IsUint64() {
		return errors.New("confirmation ancestry block number exceeds uint64")
	}
	included := receipt.BlockNumber.Uint64()
	current := head
	for current.Number.Uint64() > included {
		lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
		parent, err := m.backend.HeaderByHash(lookupCtx, current.ParentHash)
		cancel()
		if err != nil {
			return errors.Errorf("parent header %s: %w", current.ParentHash.Hex(), err)
		}
		if parent == nil || parent.Number == nil || !parent.Number.IsUint64() {
			return errors.Errorf("parent header %s is invalid", current.ParentHash.Hex())
		}
		if parent.Hash() != current.ParentHash || parent.Number.Uint64() != current.Number.Uint64()-1 {
			return errors.Errorf("parent header %s does not link to block %s", parent.Hash(), current.Hash())
		}
		current = parent
	}
	if current.Number.Uint64() != included || current.Hash() != receipt.BlockHash {
		return errors.Errorf(
			"%w: receipt block %s is no longer canonical", errReceiptReorged, receipt.BlockHash.Hex(),
		)
	}
	return nil
}

func validateReceipt(hash common.Hash, receipt *types.Receipt) error {
	if receipt == nil || receipt.BlockNumber == nil {
		return errors.Errorf("transaction receipt %s has no block number", hash.Hex())
	}
	if receipt.TxHash != hash {
		return errors.Errorf(
			"transaction receipt %s returned mismatched hash %s", hash.Hex(), receipt.TxHash.Hex(),
		)
	}
	if receipt.BlockHash == (common.Hash{}) {
		return errors.Errorf("transaction receipt %s has no block hash", hash.Hex())
	}
	return nil
}
