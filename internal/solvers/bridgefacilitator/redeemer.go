package bridgefacilitator

import (
	"context"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// adapterABI is the parsed BridgeFacilitatorAdapter ABI, used to pack redeem() calldata and the
// multicall sub-calls in chainreader.go. (mustABI is defined in chainreader.go.)
var adapterABI = mustABI(adapter.BridgeFacilitatorAdapterMetaData)

// redeemReady finds the target's redeemable Requests (batched canWithdraw via multicall) and submits
// a single bounded redeem through the shared txmanager.
func (s *Solver) redeemReady(ctx context.Context, target Target) {
	ready, err := s.reader.readyToRedeem(ctx, target.Adapter)
	if err != nil {
		s.log.Error(err, "redeem: scan ready requests", "adapter", target.Adapter.Hex())
		return
	}
	s.log.V(1).Info("redeem scan", "adapter", target.Adapter.Hex(), "ready", len(ready))
	if len(ready) == 0 {
		return
	}
	// Bound the batch so redeem() calldata + gas stay predictable; the remainder is picked up on
	// the next redeem-poll cycle (Requests stay active until redeemed).
	if len(ready) > s.cfg.RedeemBatchSize {
		s.log.Info("capping redeem batch", "ready", len(ready), "limit", s.cfg.RedeemBatchSize)
		ready = ready[:s.cfg.RedeemBatchSize]
	}

	data, err := adapterABI.Pack("redeem", ready)
	if err != nil {
		s.log.Error(err, "redeem: pack calldata")
		return
	}

	res := s.deps.TxManager.Send(ctx, txmanager.Request{
		To:    target.Adapter,
		Data:  data,
		Label: "redeem",
	})
	if res.Err != nil {
		s.log.Error(res.Err, "redeem: tx failed", "requests", len(ready))
		return
	}
	s.log.Info("redeemed ready requests", "count", len(ready), "tx", res.Hash.Hex())
}
