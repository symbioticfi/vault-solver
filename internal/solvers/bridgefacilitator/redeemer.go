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
	ctx, end := tracer.Start(ctx, "3f.redeem")
	defer end(nil) // each stage records its own failure; a scan with nothing ready is a decline

	if !s.beginRedeemPass(ctx) {
		// Scans still run, so the redeemable metric stays fresh while the backoff holds the sends.
		observability.Log(ctx).V(1).Info("redeem: backing off after an unaffordable batch; scanning only",
			"passesLeft", s.redeem.skipPasses, "batchLimit", s.redeem.limit(s.cfg.RedeemBatchSize))
	}
	var scanDuration time.Duration
	totalReady := 0
	complete := true
	successfulReads := 0
	for _, target := range s.targets {
		scanStarted := time.Now()
		ready, scanComplete, err := s.scanReadyToRedeem(ctx, target)
		scanDuration += time.Since(scanStarted)
		if err != nil {
			complete = false
			observability.Log(ctx).Error(err, "redeem: scan ready requests", "adapter", target.Adapter.Hex())
			continue
		}
		successfulReads++
		if !scanComplete {
			complete = false
			observability.Log(ctx).Info("redeem: incomplete scan; retaining last-known-good metric",
				"adapter", target.Adapter.Hex(), "validReady", len(ready))
		}
		totalReady += len(ready)
		observability.Log(ctx).V(1).Info("redeem scan", "adapter", target.Adapter.Hex(), "ready", len(ready))
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

// scanReadyToRedeem is the on-chain read stage of one adapter's redeem pass.
func (s *Solver) scanReadyToRedeem(
	ctx context.Context, target Target,
) (ready []common.Address, complete bool, err error) {
	ctx, end := tracer.Start(ctx, "3f.redeem.read", observability.AttrAdapter.String(target.Adapter.Hex()))
	defer func() { end(err) }()

	ready, complete, err = s.reader.readyToRedeem(ctx, target.Adapter)
	switch {
	case err != nil:
	case !complete:
		observability.Decline(ctx, "redeem_partial", "incomplete ready-request scan")
	case len(ready) == 0:
		observability.Decline(ctx, "redeem_skipped", "no requests ready to finalize")
	}
	return ready, complete, err
}

// redeemReady finalizes one scan's ready Requests in a single bounded
// adapter.multicall(finalizeRequest...) through the shared txmanager.
func (s *Solver) redeemReady(ctx context.Context, target Target, ready []common.Address) {
	if len(ready) == 0 {
		return
	}
	if s.redeem.holding {
		observability.Decline(ctx, "redeem_skipped", "backing off after an unaffordable redeem")
		return
	}
	// Bound the batch so the multicall calldata + gas stay predictable; the remainder is picked up on
	// the next redeem-poll cycle (Requests stay active until finalized). A backoff halves the bound.
	if limit := s.redeem.limit(s.cfg.RedeemBatchSize); len(ready) > limit {
		observability.Log(ctx).
			Info("capping redeem batch", "ready", len(ready), "limit", limit)
		ready = ready[:limit]
	}

	// finalizeRequest takes one request; batch them into the adapter's own multicall so all ready
	// requests finalize in a single tx.
	finalize := make([][]byte, len(ready))
	for i, req := range ready {
		finalize[i] = bfAdapter.PackFinalizeRequest(req)
	}
	data := bfAdapter.PackMulticall(finalize)

	// Link the settlement back to the offer span of every request it finalizes (spec §12). A miss is
	// inert: the redeem sends exactly as it would without linking.
	links, missed := s.offerLinks(target.Adapter, ready)
	var err error
	submitCtx, end := tracer.StartLinked(ctx, "3f.redeem.submit", links,
		observability.AttrAdapter.String(target.Adapter.Hex()),
		attrLinkedOffers.Int(len(links)),
	)
	defer func() { end(err) }()
	for _, key := range missed {
		observability.LinkMiss(submitCtx, key)
	}

	res := s.txManager.Send(submitCtx, txmanager.Request{
		Solver: Name,
		To:     target.Adapter,
		Data:   data,
		Label:  "redeem",
	})
	txmanager.RecordResult(submitCtx, res) // the stage
	txmanager.RecordResult(ctx, res)       // the redeem pass it belongs to
	if res.NotAdmitted {
		s.redeemNotAdmitted(submitCtx, len(ready), res.Err)
		return
	}
	if !res.Outcome.Included() {
		err = res.Err
		if err == nil {
			err = errors.Errorf("unexpected tx outcome %q", res.Outcome)
		}
		observability.Log(submitCtx).Error(err, "redeem: tx not included", "requests", len(ready), "outcome", res.Outcome)
		return
	}
	s.redeem.succeeded(s.cfg.RedeemBatchSize)
	s.observeRedeemedRequests(len(ready))
	if res.Outcome == txmanager.OutcomeIncludedUnconfirmed {
		if res.Err != nil {
			err = res.Err
			observability.Log(submitCtx).Error(res.Err, "redeem included; confirmation tracking stopped",
				"requests", len(ready), "tx", res.Hash.Hex())
		} else {
			observability.Log(submitCtx).Info("redeem included without final confirmation", "requests", len(ready), "tx", res.Hash.Hex())
		}
		return
	}
	observability.Log(submitCtx).Info("finalized ready requests", "count", len(ready), "tx", res.Hash.Hex())
}

// redeemNotAdmitted handles a redeem the manager refused before signing. It is an expected skip the
// manager already logged and counted, so the stage is declined rather than failed. An unaffordable batch
// halves the next one and backs off (redeemBackoff), logged at Info once per episode; any other refusal
// (a stale head, a paused nonce lane) is transient and the next scheduled pass retries it.
func (s *Solver) redeemNotAdmitted(ctx context.Context, requests int, refusal error) {
	reason := txmanager.NotAdmittedReason(refusal)
	observability.Decline(ctx, "redeem_not_admitted", reason)
	if !errors.Is(refusal, txmanager.ErrUnaffordable) {
		observability.Log(ctx).Info("redeem: not admitted; retrying next pass",
			"requests", requests, "reason", reason, "error", refusal)
		return
	}
	first := s.redeem.refused(requests, s.currentSignerBalance())
	fields := []any{
		"requests", requests, "reason", reason, "nextBatchLimit", s.redeem.batchLimit,
		"skippedPasses", s.redeem.skipPasses, "error", refusal,
	}
	if first {
		observability.Log(ctx).Info("redeem: the signer balance cannot fund the batch; halving it and backing off "+
			"until the lane is funded", fields...)
		return
	}
	observability.Log(ctx).V(1).Info("redeem: batch still unaffordable; backing off further", fields...)
}
