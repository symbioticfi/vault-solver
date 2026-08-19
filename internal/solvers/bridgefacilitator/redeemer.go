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

type redemptionScan struct {
	target Target
	ready  []common.Address
}

// redeemAll scans every adapter before submitting any transaction. An aggregate snapshot is published
// before the first blocking Send only when both the target source and every scan are complete; otherwise
// the last-known-good metric is retained. The observation identifies candidate adapters, but each one is
// scanned again immediately before building calldata so metrics collection never supplies stale action data.
func (s *Solver) redeemAll(ctx context.Context) {
	refreshStarted := time.Now()
	scans := make([]redemptionScan, 0, len(s.targets))
	totalReady := 0
	complete := true
	successfulReads := 0
	for _, target := range s.targets {
		ready, scanComplete, err := s.reader.readyToRedeem(ctx, target.Adapter)
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
		if len(ready) != 0 {
			scans = append(scans, redemptionScan{target: target, ready: ready})
		}
	}
	s.observeTargetDerivedState(threeFStateRedeemable, totalReady, complete)
	outcome := observability.ExternalOperationSuccess
	switch {
	case ctx.Err() != nil:
		outcome = observability.ExternalOperationSkipped
	case len(s.targets) != 0 && successfulReads == 0:
		outcome = observability.ExternalOperationError
	case !s.targetsAuthoritative || !complete:
		outcome = observability.ExternalOperationDegraded
	}
	s.operations.redeemableRefresh.Observe(outcome, time.Since(refreshStarted))
	for _, scan := range scans {
		ready, _, err := s.reader.readyToRedeem(ctx, scan.target.Adapter)
		if err != nil {
			s.log.Error(err, "redeem: revalidate ready requests", "adapter", scan.target.Adapter.Hex())
			continue
		}
		s.log.V(1).Info("redeem revalidation", "adapter", scan.target.Adapter.Hex(),
			"observedReady", len(scan.ready), "ready", len(ready))
		s.redeemReady(ctx, scan.target, ready)
	}
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
