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
	outcome := observability.ExternalOperationSuccess
	switch {
	case len(s.targets) != 0 && successfulReads == 0:
		outcome = observability.ExternalOperationError
	case !s.targetsAuthoritative || !complete:
		outcome = observability.ExternalOperationDegraded
	}
	observability.ObserveOperation(ctx, s.operations.redeemableRefresh, outcome, scanDuration)
}

// redeemReady finalizes one scan's ready Requests in a single bounded
// adapter.multicall(finalizeRequest...) through the shared txmanager.
func (s *Solver) redeemReady(ctx context.Context, target Target, ready []common.Address) {
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

	res := s.txManager.Send(ctx, txmanager.Request{
		To:    target.Adapter,
		Data:  data,
		Label: "redeem",
	})
	if !res.Outcome.Included() {
		err := res.Err
		if err == nil {
			err = errors.Errorf("unexpected tx outcome %q", res.Outcome)
		}
		s.log.Error(err, "redeem: tx not included", "requests", len(ready), "outcome", res.Outcome)
		return
	}
	s.observeRedeemedRequests(len(ready))
	if res.Outcome == txmanager.OutcomeIncludedUnconfirmed {
		if res.Err != nil {
			s.log.Error(res.Err, "redeem included; confirmation tracking stopped",
				"requests", len(ready), "tx", res.Hash.Hex())
		} else {
			s.log.Info("redeem included without final confirmation", "requests", len(ready), "tx", res.Hash.Hex())
		}
		return
	}
	s.log.Info("finalized ready requests", "count", len(ready), "tx", res.Hash.Hex())
}
