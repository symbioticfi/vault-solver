package bridgefacilitator

import (
	"strconv"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/bigmath"

	local "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	remote "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	switch spec.Name {
	case "", local.Name:
		return local.NewFromConfig(spec.Config)
	case remote.Name:
		return remote.NewFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown 3F strategy %q (available: default, webhook)", spec.Name)
	}
}

// buildStrategyInput converts the solver-owned API/on-chain snapshot into the compact strategy request.
func buildStrategyInput(
	auctions []auction,
	offerings []adapterOffering,
	offers offerSnapshot,
	now time.Time,
) types.OfferInput {
	adapters := make([]types.AdapterSnapshot, 0, len(offerings))
	for _, off := range offerings {
		adapters = append(adapters, types.AdapterSnapshot{
			ID:            lowerAddr(off.target.Adapter),
			Adapter:       off.target.Adapter,
			Vault:         off.target.Vault,
			Collateral:    off.target.Collateral,
			Fundable:      bigmath.Clone(off.st.fundable),
			OpenCount:     off.st.openCount,
			MaxAssets:     bigmath.Clone(off.st.maxAssets),
			MinAssets:     bigmath.Clone(off.st.minAssets),
			MinYieldPpm:   bigmath.Clone(off.st.minYieldPpm),
			MaxConcurrent: maxRequests,
		})
	}

	input := types.OfferInput{Now: now, Adapters: adapters}
	for i, av := range auctions {
		if !av.quotable() {
			continue
		}
		remaining := bigmath.Clone(av.amount)
		if covered := offers.coverage[av.id]; covered != nil {
			remaining.Sub(remaining, covered)
		}
		if remaining.Sign() < 0 {
			remaining.SetInt64(0)
		}
		input.Auctions = append(input.Auctions, types.AuctionSnapshot{
			ID: strconv.FormatInt(av.id, 10), AuctionID: av.id, OriginalIndex: i,
			Request: av.request, Status: av.status, DepositAsset: av.asset,
			AmountRequested: bigmath.Clone(av.amount), RemainingAmount: remaining, MaxRateBps: *av.maxRate,
		})
	}
	for _, k := range offers.entries {
		input.LiveOffers = append(input.LiveOffers, types.LiveOffer{
			AdapterID: lowerAddr(k.adapter),
			AuctionID: k.auction,
		})
	}
	return input
}

func auctionsByID(auctions []auction) map[int64]auction {
	views := make(map[int64]auction, len(auctions))
	for i := range auctions {
		av := auctions[i]
		views[av.id] = av
	}
	return views
}
