package bridgefacilitator

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// redeemReady finds the target's redeemable Requests (batched canWithdraw via multicall) and finalizes
// them in a single bounded adapter.multicall(finalizeRequest...) through the shared txmanager.
func (s *Solver) redeemReady(ctx context.Context, target Target) {
	ready, scanErr := s.reader.readyToRedeem(ctx, target.Adapter)
	ready, err := s.reconcileReadyRedemptions(target.Adapter, ready, scanErr)
	if err != nil {
		s.log.Error(err, "redeem: scan ready requests", "adapter", target.Adapter.Hex())
		return
	}
	s.log.V(1).Info("redeem scan", "adapter", target.Adapter.Hex(), "ready", len(ready))
	if len(ready) == 0 {
		return
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

	res := s.deps.TxManager.Send(ctx, txmanager.Request{
		To:    target.Adapter,
		Data:  data,
		Label: "redeem",
	})
	s.handleRedeemResult(target.Adapter, ready, res)
}

func (s *Solver) recordPendingRedemptions(adapter common.Address, requests []common.Address) {
	for _, request := range requests {
		s.pendingRedemptions[redeemKey{adapter: adapter, request: request}] = struct{}{}
	}
}

func (s *Solver) reconcilePendingRedemptions(adapter common.Address, ready []common.Address) {
	present := make(map[common.Address]struct{}, len(ready))
	for _, request := range ready {
		present[request] = struct{}{}
	}
	for key := range s.pendingRedemptions {
		if key.adapter == adapter {
			if _, ok := present[key.request]; !ok {
				delete(s.pendingRedemptions, key)
			}
		}
	}
}

func (s *Solver) filterPendingRedemptions(adapter common.Address, ready []common.Address) []common.Address {
	out := make([]common.Address, 0, len(ready))
	for _, request := range ready {
		if _, pending := s.pendingRedemptions[redeemKey{adapter: adapter, request: request}]; !pending {
			out = append(out, request)
		}
	}
	return out
}

// reconcileReadyRedemptions applies an authoritative ready-set only after its scan succeeds. An
// error leaves unresolved suppression untouched; a successful empty scan clears this adapter's set.
func (s *Solver) reconcileReadyRedemptions(
	adapter common.Address,
	ready []common.Address,
	scanErr error,
) ([]common.Address, error) {
	if scanErr != nil {
		return nil, scanErr
	}
	s.reconcilePendingRedemptions(adapter, ready)
	return s.filterPendingRedemptions(adapter, ready), nil
}

func (s *Solver) handleRedeemResult(adapter common.Address, batch []common.Address, res txmanager.Result) {
	switch res.State {
	case txmanager.StateConfirmed:
		s.log.Info("finalized ready requests", "count", len(batch), "tx", res.Hash.Hex())
	case txmanager.StateUnresolved:
		s.recordPendingRedemptions(adapter, batch)
		s.log.Error(res.Err, "redeem transaction unresolved; suppressing batch until chain resync",
			"requests", len(batch), "tx", res.Hash.Hex(), "nonce", res.Nonce)
	case txmanager.StateNotBroadcast, txmanager.StateRejected, txmanager.StateReverted:
		s.log.Error(res.Err, "redeem transaction failed definitively",
			"requests", len(batch), "tx", res.Hash.Hex(), "state", res.State)
	case txmanager.StateBroadcastUnknown, txmanager.StatePending:
		fallthrough
	default:
		s.recordPendingRedemptions(adapter, batch)
		s.log.Error(errors.Errorf("unexpected txmanager state %q", res.State),
			"redeem transaction state invalid; suppressing conservatively", "requests", len(batch))
	}
}
