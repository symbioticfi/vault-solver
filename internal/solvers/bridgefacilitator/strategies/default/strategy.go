package defaultstrategy

import (
	"context"
	"math/big"
	"slices"

	"github.com/symbioticfi/vault-solver/internal/bigmath"
	"github.com/symbioticfi/vault-solver/internal/parse"

	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	"gopkg.in/yaml.v3"
)

const Name = "default"

type Config struct{}
type Strategy struct{}

func New() *Strategy { return &Strategy{} }

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	if err := parse.DecodeStrict(raw, &Config{}); err != nil {
		return nil, err
	}
	return New(), nil
}

type candidate struct {
	index    int
	capacity *big.Int
}

func (s *Strategy) DecideOffers(ctx context.Context, input types.OfferInput) (types.OfferOutput, error) {
	// Own mutable funding and concurrency budgets; every other snapshot field is read-only.
	budgets := make([]types.AdapterSnapshot, len(input.Adapters))
	for i, adapter := range input.Adapters {
		budgets[i] = adapter
		budgets[i].Fundable = bigmath.OrZero(adapter.Fundable)
	}
	live := make(map[types.LiveOffer]bool, len(input.LiveOffers))
	for _, offer := range input.LiveOffers {
		live[offer] = true
	}
	var result types.OfferOutput
	for _, auction := range input.Auctions {
		if err := ctx.Err(); err != nil {
			return types.OfferOutput{}, err
		}
		if auction.RemainingAmount == nil || auction.RemainingAmount.Sign() <= 0 {
			continue
		}
		remaining := new(big.Int).Set(auction.RemainingAmount)
		var candidates []candidate
		for i, a := range budgets {
			if live[types.LiveOffer{AdapterID: a.ID, AuctionID: auction.AuctionID}] || a.Collateral != auction.DepositAsset {
				continue
			}
			if a.MaxConcurrent > 0 && a.OpenCount >= a.MaxConcurrent {
				continue
			}
			amount := new(big.Int).Set(a.Fundable)
			if a.MaxAssets != nil && a.MaxAssets.Cmp(amount) < 0 {
				amount.Set(a.MaxAssets)
			}
			if amount.Sign() <= 0 || a.MinAssets != nil && amount.Cmp(a.MinAssets) < 0 {
				continue
			}
			candidates = append(candidates, candidate{index: i, capacity: amount})
		}
		slices.SortStableFunc(candidates, func(a, b candidate) int { return b.capacity.Cmp(a.capacity) })
		for _, candidate := range candidates {
			if remaining.Sign() == 0 {
				break
			}
			budget := &budgets[candidate.index]
			principal := bigmath.Min(candidate.capacity, remaining)
			if floor := budget.MinAssets; floor != nil && principal.Cmp(floor) < 0 {
				continue
			}
			expected := priceReturn(principal, budget.MinYieldPpm, auction.MaxRateBps)
			if types.ValidateYield(expected, principal, budget.MinYieldPpm, auction.MaxRateBps) != nil {
				continue
			}
			result.Offers = append(result.Offers, types.OfferExecution{
				AuctionID: auction.AuctionID, Request: auction.Request, Maker: budget.Adapter,
				Principal: principal, ExpectedReturn: expected,
			})
			budget.Fundable.Sub(budget.Fundable, principal)
			budget.OpenCount++
			remaining.Sub(remaining, principal)
			live[types.LiveOffer{AdapterID: budget.ID, AuctionID: auction.AuctionID}] = true
		}
	}
	return result, nil
}

func priceReturn(principal, floor *big.Int, capBps float64) *big.Int {
	maximum := types.ExpectedReturn(principal, capBps)
	// Partial consumes round down. Preserve the margin over the on-chain yield
	// floor, clipping it only when the auction cap itself still clears that floor.
	required := types.PartialSafeMinYieldReturn(principal, floor)
	if required.Sign() <= 0 {
		return maximum
	}
	if maximum.Sign() > 0 && required.Cmp(maximum) > 0 && types.MeetsMinYield(maximum, principal, floor) {
		return maximum
	}
	return required
}
