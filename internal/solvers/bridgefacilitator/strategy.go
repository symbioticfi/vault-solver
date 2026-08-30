package bridgefacilitator

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/threef"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	switch spec.Name {
	case defaultstrategy.Name:
		return defaultstrategy.NewFromConfig(spec.Config)
	case webhookstrategy.Name:
		return webhookstrategy.NewFromConfig(spec.Config)
	default:
		return nil, unknownStrategyError(spec.Name)
	}
}

func validateStrategyConfig(spec StrategyConfig) error {
	switch spec.Name {
	case defaultstrategy.Name:
		return defaultstrategy.ValidateConfig(spec.Config)
	case webhookstrategy.Name:
		return webhookstrategy.ValidateConfig(spec.Config)
	default:
		return unknownStrategyError(spec.Name)
	}
}

func unknownStrategyError(name string) error {
	return errors.Errorf("unknown 3F strategy %q (registered: %v)", name, strategyNames())
}

func strategyNames() []string {
	return []string{defaultstrategy.Name, webhookstrategy.Name}
}

// buildStrategyInput converts the solver-owned API/on-chain snapshot into the compact strategy request.
func buildStrategyInput(
	auctions []threef.AuctionDto,
	offerings []*adapterOffering,
	offers *offerTracker,
	now time.Time,
) (types.OfferInput, map[int64]auctionView) {
	adapters := make([]types.AdapterSnapshot, 0, len(offerings))
	for _, off := range offerings {
		adapters = append(adapters, types.AdapterSnapshot{
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

	input := types.OfferInput{Now: now, Adapters: adapters}
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
		input.LiveOffers = append(input.LiveOffers, types.LiveOffer{
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
) (types.AuctionSnapshot, bool) {
	auctionID := int64(av.dto.Id)
	if !av.isOpen() {
		return types.AuctionSnapshot{}, false
	}
	request := av.requestAddr()
	if request == (common.Address{}) {
		return types.AuctionSnapshot{}, false
	}
	amountRequested := av.amountRequested()
	if amountRequested == nil || amountRequested.Sign() <= 0 {
		return types.AuctionSnapshot{}, false
	}
	rateBps, rateOk := av.maxRateBps()
	if !rateOk {
		return types.AuctionSnapshot{}, false
	}
	depositAsset := av.depositAsset()
	if !common.IsHexAddress(depositAsset) {
		return types.AuctionSnapshot{}, false
	}
	remaining := new(big.Int).Sub(amountRequested, offers.liveCoverage(auctionID, now))
	if remaining.Sign() < 0 {
		remaining = new(big.Int)
	}
	return types.AuctionSnapshot{
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
