package uniswapx

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// reconcileCompletedOrder runs only for a newly listed open order. An owned canonical fill takes
// precedence over a lagging API. Missing receipt data or failed reads cannot prove a reorg. Orders
// retired as obsolete have no owned fill receipt, so their current by-hash status is authoritative.
// Polling and fill completion share stateMu; I/O runs unlocked and the state is rechecked before clear.
func (s *Solver) reconcileCompletedOrder(ctx context.Context, hash common.Hash) error {
	s.stateMu.Lock()
	completedAt, completed := s.filled[hash]
	block, ownedFill := s.completedBlocks[hash]
	active := s.inFlight[hash]
	s.stateMu.Unlock()
	if !completed || active {
		return nil
	}
	if ownedFill {
		if block == nil || block.BlockNumber == nil || !block.BlockNumber.IsUint64() ||
			block.BlockHash == (common.Hash{}) || block.TxHash == (common.Hash{}) {
			return errors.New("owned fill has no valid inclusion block")
		}
		reorged, canonical, err := s.reader.reconcileFillInclusion(ctx, block)
		if err != nil {
			return errors.Errorf("read completed fill block: %w", err)
		}
		if !reorged {
			if canonical != nil {
				s.stateMu.Lock()
				if !s.inFlight[hash] && s.filled[hash].Equal(completedAt) && s.completedBlocks[hash] == block {
					s.completedBlocks[hash] = copyFillReceipt(canonical)
				}
				s.stateMu.Unlock()
			}
			return nil
		}
	}
	terminals, err := s.orders.ordersByHash(ctx, s.chainID, []common.Hash{hash})
	if err != nil {
		return errors.Errorf("lookup reopened order: %w", err)
	}
	terminal, ok := terminals[hash]
	if !ok {
		return errors.New("lookup reopened order: missing result")
	}
	switch terminal.Status {
	case orderStatusFilled, orderStatusCancelled, orderStatusExpired, orderStatusError, orderStatusInsufficientFunds:
		return nil
	case orderStatusOpen:
	default:
		return errors.Errorf("lookup reopened order: unknown status %q", terminal.Status)
	}
	s.stateMu.Lock()
	if s.inFlight[hash] || !s.filled[hash].Equal(completedAt) || s.completedBlocks[hash] != block {
		s.stateMu.Unlock()
		return nil
	}
	delete(s.filled, hash)
	delete(s.completedBlocks, hash)
	delete(s.retryAt, hash)
	delete(s.attempts, hash)
	delete(s.exclusiveTerminal, hash)
	s.stateMu.Unlock()
	s.invalidateQuotes()
	observability.Log(ctx).Info("completed order reopened after reconciliation", "orderHash", hash.Hex(), "ownedFill", ownedFill)
	return nil
}
