package lifi

import (
	"context"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"golang.org/x/sync/errgroup"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const (
	maxPrivateDiscountsPerFill = 16
	maxConcurrentResolutions   = 4
)

type discountClient interface {
	ListDiscounts(ctx context.Context) (*discounts.List, error)
	Resolve(ctx context.Context, discountID string) (*discounts.Resolved, error)
}

type discountRouteKey struct {
	adapter  common.Address
	tokenIn  common.Address
	tokenOut common.Address
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
	return matchingDiscountInventories(listed, bases, now, s.logInvalidDiscount)
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
	candidates := matchingDiscountInventories(listed, inventory, now, s.logInvalidDiscount)
	sort.Slice(candidates, func(i, j int) bool {
		if cmp := candidates[i].MaxRate.Cmp(candidates[j].MaxRate); cmp != 0 {
			return cmp > 0
		}
		return candidates[i].DiscountID.Hex() < candidates[j].DiscountID.Hex()
	})
	if len(candidates) > maxPrivateDiscountsPerFill {
		candidates = candidates[:maxPrivateDiscountsPerFill]
	}

	type resolution struct {
		quote  *liquidlane.FillQuote
		signed *discounts.Signed
	}
	resolutions := make([]resolution, len(candidates))
	g, resolveCtx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentResolutions)
	for i, candidate := range candidates {
		g.Go(func() error {
			if candidate.DiscountID == nil {
				return nil
			}
			resolved, resolveErr := s.discounts.Resolve(resolveCtx, candidate.DiscountID.Hex())
			if resolveErr != nil {
				s.log.Error(resolveErr, "private discounts: resolve", "discountId", candidate.DiscountID.Hex())
				return nil
			}
			signed, parseErr := discounts.ParseSigned(resolved)
			if parseErr != nil {
				s.logInvalidDiscount(candidate.DiscountID.Hex(), parseErr)
				return nil
			}
			baseQuote, ok := baseByRoute[candidate.ID]
			if !ok {
				return nil
			}
			if validateErr := validateResolvedDiscount(candidate, baseQuote, signed, now); validateErr != nil {
				s.logInvalidDiscount(candidate.DiscountID.Hex(), validateErr)
				return nil
			}
			maxAmountOut := liquidlane.AmountOutAfterDiscount(baseQuote.GrossAmountOut, signed.Terms.Discount)
			if maxAmountOut.Sign() <= 0 {
				return nil
			}
			candidate.ValidUntil = resolvedDiscountValidUntil(signed)
			quote := &liquidlane.FillQuote{
				Inventory:      candidate,
				AmountIn:       liquidlane.CloneBig(baseQuote.AmountIn),
				GrossAmountOut: liquidlane.CloneBig(baseQuote.GrossAmountOut),
				MaxAmountOut:   maxAmountOut,
				MinDiscount:    liquidlane.CloneBig(baseQuote.MinDiscount),
			}
			resolutions[i] = resolution{quote: quote, signed: signed}
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

func matchingDiscountInventories(
	listed *discounts.List,
	bases []liquidlane.Inventory,
	now time.Time,
	invalid func(string, error),
) []liquidlane.Inventory {
	if listed == nil {
		return nil
	}
	byRoute := make(map[discountRouteKey]liquidlane.Inventory, len(bases))
	for _, item := range bases {
		byRoute[discountRouteKey{adapter: item.Adapter, tokenIn: item.TokenIn, tokenOut: item.TokenOut}] = item
	}
	seen := make(map[common.Hash]bool)
	out := make([]liquidlane.Inventory, 0, len(listed.Discounts))
	for _, item := range listed.Discounts {
		offer, err := discounts.ParseOffer(item)
		if err != nil {
			if invalid != nil {
				invalid(item.DiscountID, err)
			}
			continue
		}
		if seen[offer.DiscountID] || offer.Deadline <= now.Unix() {
			continue
		}
		base, ok := byRoute[discountRouteKey{
			adapter: offer.Adapter, tokenIn: offer.TokenToRedeem, tokenOut: offer.Collateral,
		}]
		if !ok || offer.CollateralDecimals != base.TokenOutDecimals {
			continue
		}
		if base.MaxRate == nil || base.MaxRate.Sign() <= 0 || offer.MaxRate.Cmp(base.MaxRate) > 0 {
			if invalid != nil {
				invalid(item.DiscountID, errors.New("advertised discount rate exceeds current adapter max rate"))
			}
			continue
		}
		maxAssets := new(big.Int).Set(offer.MaxAssets)
		if base.MaxAssets == nil || base.MaxAssets.Sign() <= 0 {
			continue
		}
		if maxAssets.Cmp(base.MaxAssets) > 0 {
			maxAssets.Set(base.MaxAssets)
		}
		seen[offer.DiscountID] = true
		out = append(out, liquidlane.DiscountInventory(
			base.Route,
			maxAssets,
			offer.MaxRate,
			offer.DiscountID,
			time.Unix(offer.Deadline, 0),
		))
	}
	return out
}

func validateResolvedDiscount(
	candidate liquidlane.Inventory,
	base liquidlane.FillQuote,
	signed *discounts.Signed,
	now time.Time,
) error {
	if candidate.DiscountID == nil || signed.DiscountID != *candidate.DiscountID {
		return errors.New("resolved discount id does not match advertised candidate")
	}
	if signed.Adapter != candidate.Adapter {
		return errors.New("resolved discount adapter does not match advertised candidate")
	}
	if signed.Terms.TokenToRedeem != candidate.TokenIn {
		return errors.New("resolved discount token does not match route")
	}
	if base.GrossAmountOut == nil || base.GrossAmountOut.Sign() <= 0 ||
		base.MinDiscount == nil || base.MinDiscount.Sign() < 0 {
		return errors.New("fill base is missing discount facts")
	}
	if signed.Terms.Discount.Sign() < 0 ||
		signed.Terms.Discount.Cmp(big.NewInt(liquidlane.DiscountPrecision)) > 0 ||
		signed.Terms.Discount.Cmp(base.MinDiscount) < 0 {
		return errors.New("resolved discount is outside adapter bounds")
	}
	nowUnix := big.NewInt(now.Unix())
	if signed.Terms.Deadline.Cmp(nowUnix) <= 0 || signed.ProtocolDeadline.Cmp(nowUnix) <= 0 {
		return errors.New("resolved discount is expired")
	}
	return nil
}

func refreshResolvedDiscountQuotes(
	candidates []liquidlane.FillQuote,
	resolved map[common.Hash]*discounts.Signed,
	bases []liquidlane.FillQuote,
	now time.Time,
	invalid func(string, error),
) []liquidlane.FillQuote {
	baseByRoute := make(map[liquidlane.RouteID]liquidlane.FillQuote, len(bases))
	for _, base := range bases {
		baseByRoute[base.ID] = base
	}
	out := make([]liquidlane.FillQuote, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.DiscountID == nil {
			continue
		}
		signed := resolved[*candidate.DiscountID]
		base, ok := baseByRoute[candidate.ID]
		if signed == nil || !ok {
			continue
		}
		if candidate.MaxRate == nil || base.MaxRate == nil || candidate.MaxRate.Cmp(base.MaxRate) > 0 {
			if invalid != nil {
				invalid(candidate.DiscountID.Hex(), errors.New("resolved discount rate exceeds refreshed adapter max rate"))
			}
			continue
		}
		candidate.MaxAssets = liquidlane.CloneBig(candidate.MaxAssets)
		if candidate.MaxAssets == nil || base.MaxAssets == nil || base.MaxAssets.Sign() <= 0 {
			continue
		}
		if candidate.MaxAssets.Cmp(base.MaxAssets) > 0 {
			candidate.MaxAssets.Set(base.MaxAssets)
		}
		if err := validateResolvedDiscount(candidate.Inventory, base, signed, now); err != nil {
			if invalid != nil {
				invalid(candidate.DiscountID.Hex(), err)
			}
			continue
		}
		maxAmountOut := liquidlane.AmountOutAfterDiscount(base.GrossAmountOut, signed.Terms.Discount)
		if maxAmountOut.Sign() <= 0 {
			continue
		}
		candidate.AmountIn = liquidlane.CloneBig(base.AmountIn)
		candidate.GrossAmountOut = liquidlane.CloneBig(base.GrossAmountOut)
		candidate.MaxAmountOut = maxAmountOut
		candidate.MinDiscount = liquidlane.CloneBig(base.MinDiscount)
		candidate.ValidUntil = resolvedDiscountValidUntil(signed)
		out = append(out, candidate)
	}
	return out
}

func resolvedDiscountValidUntil(signed *discounts.Signed) time.Time {
	deadline := signed.Terms.Deadline
	if signed.ProtocolDeadline.Cmp(deadline) < 0 {
		deadline = signed.ProtocolDeadline
	}
	return time.Unix(deadline.Int64(), 0)
}

func (s *Solver) logInvalidDiscount(discountID string, err error) {
	s.log.V(1).Info("private discounts: ignored", "discountId", discountID, "error", err.Error())
}
