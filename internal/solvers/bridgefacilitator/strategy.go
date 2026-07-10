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
	}
	for _, k := range offers.liveEntries(now) {
		input.LiveOffers = append(input.LiveOffers, types.LiveOffer{
			AdapterID: adapterID(k.adapter),
			AuctionID: k.auction,
		})
	}
	return input
}

func auctionViewsByID(auctions []threef.AuctionDto) map[int64]auctionView {
	views := make(map[int64]auctionView, len(auctions))
	for i := range auctions {
		av := auctionView{auctions[i]}
		views[av.dto.Id] = av
	}
	return views
}

func buildAuctionSnapshot(
	av auctionView,
	originalIndex int,
	offers *offerTracker,
	now time.Time,
) (types.AuctionSnapshot, bool) {
	auctionID := av.dto.Id
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
	rateDeciBps, rateOk := av.maxRateDeciBps()
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
		MaxRateDeciBps:  cloneBig(rateDeciBps),
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
