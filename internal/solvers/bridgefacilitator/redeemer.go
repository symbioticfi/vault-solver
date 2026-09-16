package bridgefacilitator

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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
	log := observability.TraceLogger(ctx, s.log)

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
			log.Error(err, "redeem: scan ready requests", "adapter", target.Adapter.Hex())
			continue
		}
		successfulReads++
		if !scanComplete {
			complete = false
			log.Info("redeem: incomplete scan; retaining last-known-good metric",
				"adapter", target.Adapter.Hex(), "validReady", len(ready))
		}
		totalReady += len(ready)
		log.V(1).Info("redeem scan", "adapter", target.Adapter.Hex(), "ready", len(ready))
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
	// Bound the batch so the multicall calldata + gas stay predictable; the remainder is picked up on
	// the next redeem-poll cycle (Requests stay active until finalized).
	if len(ready) > s.cfg.RedeemBatchSize {
		observability.TraceLogger(ctx, s.log).
			Info("capping redeem batch", "ready", len(ready), "limit", s.cfg.RedeemBatchSize)
		ready = ready[:s.cfg.RedeemBatchSize]
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
	links, missed := s.offerLinks(ready)
	var err error
	submitCtx, end := tracer.StartLinked(ctx, "3f.redeem.submit", links,
		observability.AttrAdapter.String(target.Adapter.Hex()),
		attrLinkedOffers.Int(len(links)),
	)
	defer func() { end(err) }()
	for _, key := range missed {
		trace.SpanFromContext(submitCtx).AddEvent("link_miss", trace.WithAttributes(attribute.String("key", key)))
	}
	log := observability.TraceLogger(submitCtx, s.log)

	res := s.txManager.Send(submitCtx, txmanager.Request{
		Solver: Name,
		To:     target.Adapter,
		Data:   data,
		Label:  "redeem",
	})
	txAttrs := []attribute.KeyValue{
		observability.AttrTxHash.String(res.Hash.Hex()),
		observability.AttrTxOutcome.String(string(res.Outcome)),
	}
	observability.SetAttributes(submitCtx, txAttrs...) // the stage
	observability.SetAttributes(ctx, txAttrs...)       // the redeem pass it belongs to
	if !res.Outcome.Included() {
		err = res.Err
		if err == nil {
			err = errors.Errorf("unexpected tx outcome %q", res.Outcome)
		}
		log.Error(err, "redeem: tx not included", "requests", len(ready), "outcome", res.Outcome)
		return
	}
	s.observeRedeemedRequests(len(ready))
	if res.Outcome == txmanager.OutcomeIncludedUnconfirmed {
		if res.Err != nil {
			err = res.Err
			log.Error(res.Err, "redeem included; confirmation tracking stopped",
				"requests", len(ready), "tx", res.Hash.Hex())
		} else {
			log.Info("redeem included without final confirmation", "requests", len(ready), "tx", res.Hash.Hex())
		}
		return
	}
	log.Info("finalized ready requests", "count", len(ready), "tx", res.Hash.Hex())
}
