package bridgefacilitator

import (
	"context"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// redeemReady finds the target's redeemable Requests (batched canWithdraw via multicall) and finalizes
// them in a single bounded adapter.multicall(finalizeRequest...) through the shared txmanager.
func (s *Solver) redeemReady(ctx context.Context, target Target) (int, error) {
	ready, err := s.reader.readyToRedeem(ctx, target.Adapter)
	if err != nil {
		return 0, err
	}
	s.log.V(1).Info("redeem scan", "adapter", target.Adapter.Hex(), "ready", len(ready))
	if len(ready) == 0 {
		return 0, nil
	}
	// Bound the batch so the multicall calldata + gas stay predictable; the remainder is picked up on
	// the next redeem-poll cycle (Requests stay active until finalized).
	if len(ready) > s.cfg.RedeemBatchSize {
		s.log.Info("capping redeem batch", "ready", len(ready), "limit", s.cfg.RedeemBatchSize)
		ready = ready[:s.cfg.RedeemBatchSize]
	}

	// finalizeRequest takes one request; batch them into the adapter's own multicall so all ready
	// requests finalize in a single tx.
	finalize := make([][]byte, len(ready))
	for i, req := range ready {
		finalize[i] = bfAdapter.PackFinalizeRequest(req)
	}
	data := bfAdapter.PackMulticall(finalize)

	res := s.txm.Send(ctx, txmanager.Request{
		To:    target.Adapter,
		Data:  data,
		Label: "redeem",
	})
	if res.Err != nil {
		return len(ready), errors.Errorf("submit %d ready requests: %w", len(ready), res.Err)
	}
	s.metrics.redeemed(len(ready))
	s.log.Info("finalized ready requests", "count", len(ready), "tx", res.Hash.Hex())
	return len(ready), nil
}
