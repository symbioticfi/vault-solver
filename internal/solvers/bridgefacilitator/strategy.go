package bridgefacilitator

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

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

// buildStrategyInput converts the solver-owned API/on-chain snapshot into the compact strategy request,
// returning a skip for each auction it leaves out.
func buildStrategyInput(
	auctions []threef.AuctionDto,
	offerings []*adapterOffering,
	offers *offerTracker,
	now time.Time,
) (types.OfferInput, []types.OfferSkip) {
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
	var skips []types.OfferSkip
	for i := range auctions {
		av := auctionView{auctions[i]}
		auction, err := buildAuctionSnapshot(av, i, offers, now)
		if err != nil {
			skips = append(skips, types.OfferSkip{AuctionID: int64(av.dto.Id), Reason: err.Error()})
			continue
		}
		input.Auctions = append(input.Auctions, auction)
	}
	for _, k := range offers.liveEntries(now) {
		st := offers.offers[k]
		input.LiveOffers = append(input.LiveOffers, types.LiveOffer{
			AdapterID:      adapterID(k.adapter),
			AuctionID:      k.auction,
			Principal:      cloneBig(st.principal),
			ExpectedReturn: cloneBig(st.expectedReturn),
			Open:           st.open,
		})
	}
	return input, skips
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
) (types.AuctionSnapshot, error) {
	auctionID := int64(av.dto.Id)
	if !av.isOpen() {
		return types.AuctionSnapshot{}, errors.Errorf("status %q is not open or solvable", av.dto.Status)
	}
	request := av.requestAddr()
	if request == (common.Address{}) {
		return types.AuctionSnapshot{}, errors.Errorf("requestId %q is not an address", av.dto.RequestId)
	}
	amountRequested := av.amountRequested()
	if amountRequested == nil || amountRequested.Sign() <= 0 {
		return types.AuctionSnapshot{}, errors.Errorf("amountRequested %q is missing or not positive", av.dto.GetAmountRequested())
	}
	rateBps, rateOk := av.maxRateBps()
	if !rateOk {
		return types.AuctionSnapshot{}, errors.New("maxRate is unresolved")
	}
	basis, basisOk := av.rateBasisSeconds()
	if !basisOk {
		return types.AuctionSnapshot{}, errors.Errorf("rateBasisSeconds %v is negative", av.dto.GetRateBasisSeconds())
	}
	depositAsset := av.depositAsset()
	if !common.IsHexAddress(depositAsset) {
		return types.AuctionSnapshot{}, errors.Errorf("depositAsset %q is not an address", depositAsset)
	}
	// Unknown live principal counts as full coverage. The auction still reaches the strategy: adapters
	// offer independently, so one adapter's live offer never hides the auction from the others.
	remaining := new(big.Int)
	if coverage := offers.liveCoverage(auctionID, now); coverage != nil {
		remaining.Sub(amountRequested, coverage)
		if remaining.Sign() < 0 {
			remaining.SetInt64(0)
		}
	}
	return types.AuctionSnapshot{
		ID:               auctionIDString(auctionID),
		AuctionID:        auctionID,
		OriginalIndex:    originalIndex,
		Request:          request,
		Status:           av.dto.Status,
		DepositAsset:     common.HexToAddress(depositAsset),
		AmountRequested:  cloneBig(amountRequested),
		RemainingAmount:  remaining,
		MaxRateBps:       rateBps,
		RateBasisSeconds: basis,
	}, nil
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
