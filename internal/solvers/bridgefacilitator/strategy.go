package bridgefacilitator

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	name := spec.Name
	if name == "" {
		name = defaultStrategyName
	}
	return strategies.New(name, spec.Config, strategies.Deps{})
}

// buildStrategyInput converts the solver-owned API/on-chain snapshot into the compact strategy request.
func buildStrategyInput(
	auctions []threef.AuctionDto,
	offerings []*adapterOffering,
	offers *offerTracker,
	now time.Time,
) types.OfferInput {
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
			MinYieldBps:   cloneBig(off.st.minYieldBps),
			MaxConcurrent: maxRequests,
		})
	}

	input := types.OfferInput{Now: now, Adapters: adapters}
	for i := range auctions {
		av := auctionView{auctions[i]}
		auction, ok := buildAuctionSnapshot(av, i, offers, now)
		if !ok {
			continue
		}
		input.Auctions = append(input.Auctions, auction)
		for _, off := range offerings {
			input.Candidates = append(input.Candidates, buildOfferCandidate(auction, off, offers, now))
		}
	}
	return input
}

func auctionViewsByID(auctions []threef.AuctionDto) map[int64]auctionView {
	views := make(map[int64]auctionView, len(auctions))
	for i := range auctions {
		av := auctionView{auctions[i]}
		views[int64(av.dto.Id)] = av
	}
	return views
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

func buildOfferCandidate(
	auction types.AuctionSnapshot,
	off *adapterOffering,
	offers *offerTracker,
	now time.Time,
) types.OfferCandidate {
	capacity, ok := sizeOffer(sizeInputs{
		fundable:      off.st.fundable,
		maxAssets:     off.st.maxAssets,
		minAssets:     off.st.minAssets,
		openCount:     off.st.openCount,
		maxConcurrent: maxRequests,
	})
	if !ok {
		capacity = new(big.Int)
	}
	adapter := adapterID(off.target.Adapter)
	return types.OfferCandidate{
		ID:           offerCandidateID(adapter, auction.AuctionID),
		AdapterID:    adapter,
		AuctionID:    auction.AuctionID,
		Capacity:     capacity,
		HasLiveOffer: offers.hasLive(off.target.Adapter, auction.AuctionID, now),
	}
}

func adapterID(adapter common.Address) string {
	return strings.ToLower(adapter.Hex())
}

func auctionIDString(auctionID int64) string {
	return strconv.FormatInt(auctionID, 10)
}

func offerCandidateID(adapter string, auctionID int64) string {
	return adapter + ":" + auctionIDString(auctionID)
}

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}
