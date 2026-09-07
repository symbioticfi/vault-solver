package lifi

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const (
	maxPrivateDiscountsPerFill = 16
	maxConcurrentResolutions   = 4
)

func (s *Solver) quoteDiscountInventories(
	ctx context.Context,
	bases []liquidlane.Inventory,
	now time.Time,
) ([]liquidlane.Inventory, bool) {
	if s.discounts == nil {
		return nil, false
	}
	listed, err := s.discounts.ListDiscounts(ctx)
	if err != nil {
		s.log.Error(err, "private discounts: list for quote")
		return nil, true
	}
	inventory, issues := discounts.MatchInventories(listed, bases, discounts.MatchOptions{Now: now})
	s.logDiscountIssues(issues)
	return inventory, len(issues) > 0
}

func (s *Solver) fillDiscountQuotes(
	ctx context.Context,
	bases []liquidlane.FillQuote,
	now time.Time,
) ([]liquidlane.FillQuote, map[common.Hash]*discounts.Signed) {
	if s.discounts == nil || len(bases) == 0 {
		return nil, nil
	}
	inventory := make([]liquidlane.Inventory, 0, len(bases))
	baseByRoute := make(map[liquidlane.RouteID]liquidlane.FillQuote, len(bases))
	for _, quote := range bases {
		inventory = append(inventory, quote.Inventory)
		baseByRoute[quote.ID] = quote
	}
	listed, err := s.discounts.ListDiscounts(ctx)
	if err != nil {
		s.log.Error(err, "private discounts: list for fill")
		return nil, nil
	}
	candidates, issues := discounts.MatchInventories(listed, inventory, discounts.MatchOptions{Now: now})
	s.logDiscountIssues(issues)
	slices.SortFunc(candidates, func(a, b liquidlane.Inventory) int {
		if order := b.MaxRate.Cmp(a.MaxRate); order != 0 {
			return order
		}
		return a.DiscountID.Cmp(*b.DiscountID)
	})
	if len(candidates) > maxPrivateDiscountsPerFill {
		candidates = candidates[:maxPrivateDiscountsPerFill]
	}

	resolutions := make([]discountResolution, len(candidates))
	var next atomic.Uint64
	var workers sync.WaitGroup
	for range min(maxConcurrentResolutions, len(candidates)) {
		workers.Go(func() {
			for ctx.Err() == nil {
				i := int(next.Add(1) - 1)
				if i >= len(candidates) {
					return
				}
				candidate := candidates[i]
				if base, ok := baseByRoute[candidate.ID]; ok {
					resolutions[i] = s.resolveFillDiscount(ctx, candidate, base, now)
				}
			}
		})
	}
	workers.Wait()
	quotes := make([]liquidlane.FillQuote, 0, len(candidates))
	resolvedByID := make(map[common.Hash]*discounts.Signed, len(candidates))
	for _, resolution := range resolutions {
		if resolution.quote == nil || resolution.signed == nil {
			continue
		}
		quotes = append(quotes, *resolution.quote)
		resolvedByID[resolution.signed.DiscountID] = resolution.signed
	}
	return quotes, resolvedByID
}

type discountResolution struct {
	quote  *liquidlane.FillQuote
	signed *discounts.Signed
}

// Resolve at most one signed candidate per result slot; a failed candidate never
// cancels independent routes. The owner joins all workers before exposing results.
func (s *Solver) resolveFillDiscount(ctx context.Context, candidate liquidlane.Inventory,
	base liquidlane.FillQuote, now time.Time,
) discountResolution {
	if candidate.DiscountID == nil {
		return discountResolution{}
	}
	id := candidate.DiscountID.Hex()
	resolved, err := s.discounts.Resolve(ctx, id)
	if err != nil {
		s.log.Error(err, "private discounts: resolve", "discountId", id)
		return discountResolution{}
	}
	signed, err := discounts.ParseAndValidate(resolved, discounts.Selection{
		DiscountID: *candidate.DiscountID, Adapter: candidate.Adapter, TokenIn: candidate.TokenIn,
	}, base, now)
	if err != nil {
		s.logInvalidDiscount(id, err)
		return discountResolution{}
	}
	amountOut := liquidlane.AmountOutAfterDiscount(base.GrossAmountOut, signed.Terms.Discount)
	if amountOut.Sign() <= 0 {
		return discountResolution{}
	}
	candidate.ValidUntil = discounts.ValidUntil(signed)
	return discountResolution{signed: signed, quote: &liquidlane.FillQuote{
		Inventory: candidate, AmountIn: bigmath.Clone(base.AmountIn),
		GrossAmountOut: bigmath.Clone(base.GrossAmountOut), MaxAmountOut: amountOut,
		MinDiscount: bigmath.Clone(base.MinDiscount),
	}}
}

func (s *Solver) logInvalidDiscount(discountID string, err error) {
	s.log.V(1).Info("private discounts: ignored", "discountId", discountID, "error", err.Error())
}

func (s *Solver) logDiscountIssues(issues []discounts.OfferIssue) {
	for _, issue := range issues {
		s.logInvalidDiscount(issue.DiscountID, issue.Err)
	}
}
