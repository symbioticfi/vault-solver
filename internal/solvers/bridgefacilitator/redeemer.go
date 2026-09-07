package bridgefacilitator

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type transactionSender interface {
	Send(ctx context.Context, req txmanager.Request) txmanager.Result
}

// redeemAll consumes each adapter scan before moving on; aggregate freshness advances only after full coverage.
func (s *Solver) redeemAll(ctx context.Context) {
	var scanDuration time.Duration
	totalReady := 0
	complete := true
	successfulReads := 0
	for _, target := range s.targets {
		scanStarted := time.Now()
		ready, scanComplete, err := s.reader.readyToRedeem(ctx, target.Adapter)
		scanDuration += time.Since(scanStarted)
		if err != nil {
			complete = false
			s.log.Error(err, "redeem: scan ready requests", "adapter", target.Adapter.Hex())
			continue
		}
		successfulReads++
		if !scanComplete {
			complete = false
			s.log.Info("redeem: incomplete scan; retaining last-known-good metric",
				"adapter", target.Adapter.Hex(), "validReady", len(ready))
		}
		totalReady += len(ready)
		s.log.V(1).Info("redeem scan", "adapter", target.Adapter.Hex(), "ready", len(ready))
		s.redeemReady(ctx, target, ready)
	}
	s.observeTargetDerivedState(threeFStateRedeemable, totalReady, complete)
	observability.ObserveOperation(ctx, s.operations.redeemableRefresh,
		s.snapshotOutcome(len(s.targets), successfulReads, complete), scanDuration)
}

// redeemReady finalizes one scan's ready Requests in a single bounded
// adapter.multicall(finalizeRequest...) through the shared txmanager.
func (s *Solver) redeemReady(ctx context.Context, target Target, ready []common.Address) {
	count := min(len(ready), s.cfg.RedeemBatchSize)
	if count == 0 {
		return
	}
	log := s.log.WithValues("adapter", target.Adapter.Hex(), "requests", count)
	if count < len(ready) {
		log.Info("capping redeem batch", "ready", len(ready), "limit", count)
	}
	// Requests remain active until finalized, so the next poll safely picks up
	// the tail. Use the adapter's multicall to make each bounded batch atomic.
	calls := make([][]byte, count)
	for i, request := range ready[:count] {
		calls[i] = bfAdapter.PackFinalizeRequest(request)
	}
	result := s.txManager.Send(ctx, txmanager.Request{
		Solver: Name, To: target.Adapter, Label: "redeem", Data: bfAdapter.PackMulticall(calls),
	})
	if !result.Outcome.Included() {
		err := result.Err
		if err == nil {
			err = errors.Errorf("unexpected tx outcome %q", result.Outcome)
		}
		log.Error(err, "redeem: tx not included", "outcome", result.Outcome)
		return
	}
	s.observeRedeemedRequests(count)
	log = log.WithValues("tx", result.Hash.Hex())
	switch {
	case result.Outcome != txmanager.OutcomeIncludedUnconfirmed:
		log.Info("finalized ready requests", "count", count)
	case result.Err != nil:
		log.Error(result.Err, "redeem included; confirmation tracking stopped")
	default:
		log.Info("redeem included without final confirmation")
	}
}
