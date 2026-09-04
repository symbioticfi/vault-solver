package bridgefacilitator

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/observability"

	"github.com/symbioticfi/vault-solver/api/threef"
)

func (solver *Solver) discoverAndOffer(ctx context.Context) {
	timer := observability.StartOperation(solver.metrics.operation(offerRefreshOperation))
	outcome := observability.ExternalOperationSuccess
	defer func() { timer.Finish(ctx, outcome) }()
	if !solver.canCreateOffer() {
		outcome = observability.ExternalOperationSkipped
		solver.log.V(1).Info("skipping offer discovery: transaction lane not ready")
		return
	}
	if len(solver.targets) == 0 {
		outcome = observability.ExternalOperationSkipped
		return
	}
	pass := solver.loadOfferPass(ctx)
	if pass == nil {
		outcome = observability.ExternalOperationDegraded
		return
	}
	decision, err := solver.planner.DecideOffers(ctx, pass.input)
	if err != nil {
		outcome = observability.ExternalOperationError
		solver.log.Error(err, "offer: planner")
		return
	}
	solver.submitOfferPlan(ctx, pass, decision.Offers)
}

func (solver *Solver) loadOfferPass(ctx context.Context) *offerPass {
	auctions, err := solver.api.listAuctions(ctx)
	if err != nil {
		solver.log.Error(err, "discover: list auctions")
		return nil
	}
	solver.log.V(1).Info("discovered auctions", "count", len(auctions))
	solver.reconcileOffers(ctx, solver.targets)

	offerings := make([]*adapterOffering, 0, len(solver.targets))
	for _, target := range solver.targets {
		state, readErr := solver.reader.liquidityAndExposure(ctx, target.Adapter)
		if readErr != nil {
			solver.log.Error(readErr, "offer: liquidity/exposure", "adapter", target.Adapter.Hex())
			continue
		}
		solver.log.V(1).Info("adapter liquidity",
			"adapter", target.Adapter.Hex(), "fundable", state.fundable.String(), "openRequests", state.openCount,
			"maxAssets", state.maxAssets.String(), "minAssets", state.minAssets.String(),
			"minYieldPpm", state.minYieldPpm.String())
		offerings = append(offerings, &adapterOffering{target: target, st: state})
	}
	if len(offerings) == 0 {
		return nil
	}

	input, auctionByID := buildStrategyInput(auctions, offerings, solver.offers, time.Now())
	if len(input.Auctions) == 0 {
		return nil
	}
	exposure := make(map[common.Address]exposureState, len(offerings))
	for _, offering := range offerings {
		exposure[offering.target.Adapter] = offering.st
	}
	return &offerPass{input: input, auctionByID: auctionByID, exposureByAdapter: exposure}
}

func (solver *Solver) submitOfferPlan(
	ctx context.Context,
	pass *offerPass,
	offers []OfferExecution,
) {
	for _, offer := range offers {
		auction, exists := pass.auctionByID[offer.AuctionID]
		if !exists {
			solver.log.Error(errors.Errorf("auction %d not found", offer.AuctionID), "offer: build")
			continue
		}
		exposure, exists := pass.exposureByAdapter[offer.Maker]
		if !exists {
			solver.log.Error(
				errors.Errorf("offer for adapter %s absent from this pass's snapshot", offer.Maker.Hex()),
				"offer: unknown maker; skipping", "auctionId", offer.AuctionID,
			)
			continue
		}
		maxRate, validRate := auction.maxRateBps()
		if !validRate {
			solver.log.Error(errors.Errorf("auction %d has no resolved maxRate", offer.AuctionID),
				"offer: unbiddable auction; skipping", "adapter", offer.Maker.Hex())
			continue
		}
		if err := ValidateYield(
			offer.ExpectedReturn, offer.Principal, exposure.minAssets, exposure.minYieldPpm, maxRate,
		); err != nil {
			solver.log.Error(err, "offer: yield out of bounds; skipping",
				"auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
			continue
		}
		dto, err := solver.buildSignedOffer(auction, offer)
		if err != nil {
			solver.metrics.offer("error", common.Address{}, nil, nil)
			solver.log.Error(err, "offer: build", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
			continue
		}
		submitted, err := solver.submitOfferIfLaneReady(ctx, dto)
		if !submitted {
			solver.log.V(1).Info("stopping offer submission: transaction lane no longer ready")
			return
		}
		if err != nil {
			solver.metrics.offer("error", common.Address{}, nil, nil)
			solver.log.Error(err, "offer: submit", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
			continue
		}
		solver.metrics.offer(
			"success", common.HexToAddress(auction.depositAsset()), offer.Principal,
			new(big.Int).Sub(offer.ExpectedReturn, offer.Principal),
		)
		solver.log.Info("offer submitted",
			"auctionId", offer.AuctionID, "adapter", offer.Maker.Hex(), "request", offer.Request.Hex(),
			"principal", offer.Principal.String(), "expectedReturn", dto.ExpectedReturn, "strategyReason", offer.Reason)
	}
}

func (solver *Solver) reconcileOffers(ctx context.Context, targets []Target) {
	now := time.Now()
	for _, target := range targets {
		offers, err := solver.api.listOffers(ctx, target.Adapter)
		if err != nil {
			solver.log.Error(err, "reconcile offers: list offers", "adapter", target.Adapter.Hex())
			continue
		}
		live := make(map[int64]offerState)
		for _, offer := range offers {
			if !activeOffer(offer.Status) {
				continue
			}
			expiry, parseErr := parseUnixTime(offer.Expiration)
			if parseErr != nil || !expiry.After(now) {
				continue
			}
			principal, ok := parsePrincipal(offer.Amount)
			if !ok {
				solver.log.V(1).Info("reconcile offers: unparseable amount; coverage may undercount",
					"adapter", target.Adapter.Hex(), "amount", offer.Amount)
				principal = new(big.Int)
			}
			auctionID := int64(offer.AuctionId)
			current, exists := live[auctionID]
			if !exists || expiry.After(current.expiry) {
				live[auctionID] = offerState{expiry: expiry, principal: principal}
			}
		}
		solver.offers.reconcileAdapter(target.Adapter, live)
	}
	solver.metrics.state("offers", solver.offers.count())
}

func (solver *Solver) canCreateOffer() bool {
	return solver.laneReady != nil && solver.laneReady()
}

func (solver *Solver) submitOfferIfLaneReady(ctx context.Context, dto threef.CreateOfferDto) (bool, error) {
	if !solver.canCreateOffer() {
		return false, nil
	}
	return true, solver.api.createOffer(ctx, dto)
}
