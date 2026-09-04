package bridgefacilitator

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func newPlanner(spec StrategyConfig) (Planner, error) {
	switch spec.Name {
	case defaultStrategyName:
		return newDefaultPlannerFromConfig(spec.Config)
	case webhookStrategyName:
		return newWebhookPlannerFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown 3F strategy %q", spec.Name)
	}
}

func validatePlannerConfig(spec StrategyConfig) error {
	switch spec.Name {
	case defaultStrategyName:
		return validateDefaultPlannerConfig(spec.Config)
	case webhookStrategyName:
		return webhook.ValidateConfig(spec.Config)
	default:
		return errors.Errorf("unknown 3F strategy %q", spec.Name)
	}
}

// buildStrategyInput converts the solver-owned API/on-chain snapshot into the compact strategy request.
func buildStrategyInput(
	auctions []threef.AuctionDto,
	offerings []*adapterOffering,
	offers *offerTracker,
	now time.Time,
) (OfferInput, map[int64]auctionView) {
	adapters := make([]AdapterSnapshot, 0, len(offerings))
	for _, off := range offerings {
		adapters = append(adapters, AdapterSnapshot{
			ID:            adapterID(off.target.Adapter),
			Adapter:       off.target.Adapter,
			Vault:         off.target.Vault,
			Collateral:    off.target.Collateral,
			Fundable:      cloneBig(off.st.fundable),
			OpenCount:     off.st.openCount,
			MaxAssets:     cloneBig(off.st.maxAssets),
			MinAssets:     cloneBig(off.st.minAssets),
			MinYieldPpm:   cloneBig(off.st.minYieldPpm),
			MaxConcurrent: maxRequests,
		})
	}

	input := OfferInput{Now: now, Adapters: adapters}
	views := make(map[int64]auctionView, len(auctions))
	for i := range auctions {
		av := auctionView{auctions[i]}
		views[int64(av.dto.Id)] = av
		auction, ok := buildAuctionSnapshot(av, i, offers, now)
		if !ok {
			continue
		}
		input.Auctions = append(input.Auctions, auction)
	}
	for _, k := range offers.liveEntries(now) {
		input.LiveOffers = append(input.LiveOffers, LiveOffer{
			AdapterID: adapterID(k.adapter),
			AuctionID: k.auction,
		})
	}
	return input, views
}

func buildAuctionSnapshot(
	av auctionView,
	originalIndex int,
	offers *offerTracker,
	now time.Time,
) (AuctionSnapshot, bool) {
	auctionID := int64(av.dto.Id)
	if !av.isOpen() {
		return AuctionSnapshot{}, false
	}
	request := av.requestAddr()
	if request == (common.Address{}) {
		return AuctionSnapshot{}, false
	}
	amountRequested := av.amountRequested()
	if amountRequested == nil || amountRequested.Sign() <= 0 {
		return AuctionSnapshot{}, false
	}
	rateBps, rateOk := av.maxRateBps()
	if !rateOk {
		return AuctionSnapshot{}, false
	}
	depositAsset := av.depositAsset()
	if !common.IsHexAddress(depositAsset) {
		return AuctionSnapshot{}, false
	}
	remaining := new(big.Int).Sub(amountRequested, offers.liveCoverage(auctionID, now))
	if remaining.Sign() < 0 {
		remaining = new(big.Int)
	}
	return AuctionSnapshot{
		ID:              auctionIDString(auctionID),
		AuctionID:       auctionID,
		OriginalIndex:   originalIndex,
		Request:         request,
		Status:          av.dto.Status,
		DepositAsset:    common.HexToAddress(depositAsset),
		AmountRequested: cloneBig(amountRequested),
		RemainingAmount: remaining,
		MaxRateBps:      rateBps,
	}, true
}

func adapterID(adapter common.Address) string {
	return strings.ToLower(adapter.Hex())
}

func auctionIDString(auctionID int64) string {
	return strconv.FormatInt(auctionID, 10)
}

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}
