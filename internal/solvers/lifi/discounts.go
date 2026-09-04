package lifi

import (
	"context"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const (
	maxPrivateDiscountsPerFill = 16
	maxConcurrentResolutions   = 4
)

type fillDiscountResolution struct {
	quote  *liquidlane.FillQuote
	signed *discounts.Signed
}

func (s *Solver) quoteDiscountInventories(
	ctx context.Context,
	bases []liquidlane.Inventory,
	now time.Time,
) []liquidlane.Inventory {
	if s.discounts == nil {
		return nil
	}
	listed, err := s.discounts.ListDiscounts(ctx)
	if err != nil {
		s.log.Error(err, "private discounts: list for quote")
		return nil
	}
	inventory, issues := discounts.MatchInventories(listed, bases, discounts.MatchOptions{Now: now})
	s.logDiscountIssues(issues)
	return inventory
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
	sort.Slice(candidates, func(i, j int) bool {
		if cmp := candidates[i].MaxRate.Cmp(candidates[j].MaxRate); cmp != 0 {
			return cmp > 0
		}
		return candidates[i].DiscountID.Hex() < candidates[j].DiscountID.Hex()
	})
	if len(candidates) > maxPrivateDiscountsPerFill {
		candidates = candidates[:maxPrivateDiscountsPerFill]
	}

	resolutions := make([]fillDiscountResolution, len(candidates))
	g, resolveCtx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentResolutions)
	for i, candidate := range candidates {
		g.Go(func() error {
			resolutions[i] = s.resolveFillDiscount(resolveCtx, candidate, baseByRoute, now)
			return nil
		})
	}
	_ = g.Wait()
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

func (s *Solver) resolveFillDiscount(
	ctx context.Context,
	candidate liquidlane.Inventory,
	baseByRoute map[liquidlane.RouteID]liquidlane.FillQuote,
	now time.Time,
) fillDiscountResolution {
	if candidate.DiscountID == nil {
		return fillDiscountResolution{}
	}
	baseQuote, ok := baseByRoute[candidate.ID]
	if !ok {
		return fillDiscountResolution{}
	}
	selection := discounts.Selection{
		DiscountID: *candidate.DiscountID,
		Adapter:    candidate.Adapter, TokenIn: candidate.TokenIn,
	}
	resolved, err := s.discounts.Resolve(ctx, candidate.DiscountID.Hex())
	if err != nil {
		s.log.Error(err, "private discounts: resolve", "discountId", candidate.DiscountID.Hex())
		return fillDiscountResolution{}
	}
	signed, err := discounts.ParseAndValidate(resolved, selection, baseQuote, now)
	if err != nil {
		s.logInvalidDiscount(candidate.DiscountID.Hex(), err)
		return fillDiscountResolution{}
	}
	maxAmountOut := liquidlane.AmountOutAfterDiscount(baseQuote.GrossAmountOut, signed.Terms.Discount)
	if maxAmountOut.Sign() <= 0 {
		return fillDiscountResolution{}
	}
	candidate.ValidUntil = discounts.ValidUntil(signed)
	return fillDiscountResolution{
		quote: &liquidlane.FillQuote{
			Inventory: candidate, AmountIn: liquidlane.CloneBig(baseQuote.AmountIn),
			GrossAmountOut: liquidlane.CloneBig(baseQuote.GrossAmountOut), MaxAmountOut: maxAmountOut,
			MinDiscount: liquidlane.CloneBig(baseQuote.MinDiscount),
		},
		signed: signed,
	}
}

func (s *Solver) logInvalidDiscount(discountID string, err error) {
	s.log.V(1).Info("private discounts: ignored", "discountId", discountID, "error", err.Error())
}

func (s *Solver) logDiscountIssues(issues []discounts.OfferIssue) {
	for _, issue := range issues {
		s.logInvalidDiscount(issue.DiscountID, issue.Err)
	}
}
