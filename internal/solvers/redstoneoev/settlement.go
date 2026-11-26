package redstoneoev

import (
	"context"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/executor"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// revertDecoder covers every contract a settlement or Executor read can revert in: the Executor, our
// callback, Morpho, and the LiquidLane contracts the callback redeems through.
var revertDecoder = liquidlane.RevertDecoder(
	&executor.RedStoneExecutorMetaData, &callback.SymbioticOevSolverMetaData, &morphobinding.MorphoMetaData,
)

// settlementReplayer is the slice of the chain client a failed-settlement replay needs.
type settlementReplayer interface {
	TransactionByHash(ctx context.Context, hash common.Hash) (*ethtypes.Transaction, bool, error)
	TransactionReceipt(ctx context.Context, hash common.Hash) (*ethtypes.Receipt, error)
	chain.MinedCallReplayer
}

// replayFailedSettlement names why our settlement failed, off the WS read loop. RedStone's frame
// carries no revert data, so the mined tx is replayed. One replay at a time: failures are rare
// and the breaker halts bidding after a few, so a busy replayer just drops the extra one.
func (s *Solver) replayFailedSettlement(ctx context.Context, rawHash string) {
	if s.replayer == nil {
		return
	}
	hash, ok := parseTxHash(rawHash)
	if !ok {
		observability.Log(ctx).V(1).Info("skip settlement replay: no valid txHash", "txHash", rawHash)
		return
	}
	if !s.replayBusy.CompareAndSwap(false, true) {
		observability.Log(ctx).V(1).Info("skip settlement replay: a previous replay is still running", "txHash", rawHash)
		return
	}
	s.replays.Go(func() {
		defer s.replayBusy.Store(false)
		replayCtx, end := tracer.Start(ctx, "oev.liquidation.replay", observability.AttrTxHash.String(hash.Hex()))
		reason, err := settlementRevert(replayCtx, s.replayer, ethtypes.LatestSignerForChainID(s.chainID), hash)
		// The replay is diagnostic: its own failure is logged, never a span error or a page.
		end(nil)
		log := observability.Log(replayCtx)
		if err != nil {
			log.Info("liquidation-failure-replay", "txHash", hash.Hex(),
				"revert", "unavailable", "replayError", err.Error())
			return
		}
		log.Info("liquidation-failure-replay", "txHash", hash.Hex(), "revert", reason)
	})
}

// settlementRevert fetches a settlement tx and its receipt and, when it reverted, names the revert
// through the shared mined-call replay.
func settlementRevert(
	ctx context.Context, c settlementReplayer, signer ethtypes.Signer, hash common.Hash,
) (string, error) {
	tx, _, err := c.TransactionByHash(ctx, hash)
	if err != nil {
		return "", errors.Errorf("fetch settlement tx: %w", err)
	}
	receipt, err := c.TransactionReceipt(ctx, hash)
	if err != nil {
		return "", errors.Errorf("fetch settlement receipt: %w", err)
	}
	if receipt.Status == ethtypes.ReceiptStatusSuccessful {
		return "settlement tx succeeded on-chain (status 1); nothing to replay", nil
	}
	from, err := ethtypes.Sender(signer, tx)
	if err != nil {
		return "", errors.Errorf("recover settlement sender: %w", err)
	}
	return revertDecoder.MinedRevert(ctx, c, ethereum.CallMsg{
		From: from, To: tx.To(), Gas: tx.Gas(), Value: tx.Value(), Data: tx.Data(),
	}, receipt.BlockNumber), nil
}

func parseTxHash(raw string) (common.Hash, bool) {
	b, err := hexutil.Decode(strings.TrimSpace(raw))
	if err != nil || len(b) != common.HashLength {
		return common.Hash{}, false
	}
	return common.BytesToHash(b), true
}
