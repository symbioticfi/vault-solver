package uniswapx

import (
	"context"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const maxAdvertisedDiscountRoutes = 256

type advertisedRouteFilter struct {
	adapters map[common.Address]bool
	tokenIn  common.Address
	tokenOut common.Address
}

func (s *Solver) listDiscounts(ctx context.Context) (*liquiddiscounts.List, error) {
	requestCtx, cancel := context.WithTimeout(ctx, s.cfg.Discounts.HTTPTimeout)
	defer cancel()
	return s.discounts.ListDiscounts(requestCtx)
}

func (s *Solver) quoteRoutesWithDiscounts(
	ctx context.Context,
	configured []liquidlane.Route,
	now time.Time,
) ([]liquidlane.Route, *liquiddiscounts.List, error) {
	filter := advertisedRouteFilter{}
	if s.cfg.quoteScopesToAdapters() {
		filter.adapters = adapterSet(s.cfg.Adapters)
	}
	return s.routesWithDiscounts(ctx, configured, now, filter)
}

func (s *Solver) fillRoutesWithDiscounts(
	ctx context.Context,
	configured []liquidlane.Route,
	tokenIn, tokenOut common.Address,
	now time.Time,
) ([]liquidlane.Route, *liquiddiscounts.List, error) {
	return s.routesWithDiscounts(ctx, configured, now, advertisedRouteFilter{
		tokenIn: tokenIn, tokenOut: tokenOut,
	})
}

func (s *Solver) routesWithDiscounts(
	ctx context.Context,
	configured []liquidlane.Route,
	now time.Time,
	filter advertisedRouteFilter,
) ([]liquidlane.Route, *liquiddiscounts.List, error) {
	if !s.cfg.usesDiscounts() {
		return configured, nil, nil
	}
	listed, err := s.listDiscounts(ctx)
	if err != nil {
		return configured, nil, err
	}
	dynamic := s.resolveAdvertisedRoutes(ctx, listed, configured, now, filter)
	return mergeRoutes(configured, dynamic), listed, nil
}

func (s *Solver) resolveAdvertisedRoutes(
	ctx context.Context,
	listed *liquiddiscounts.List,
	configured []liquidlane.Route,
	now time.Time,
	filter advertisedRouteFilter,
) []liquidlane.Route {
	offers, issues := liquiddiscounts.LiveOffers(listed, now)
	for _, issue := range issues {
		s.log.V(1).Info(
			"ignore invalid advertised discount",
			"discountId", issue.DiscountID,
			"error", issue.Err.Error(),
		)
	}
	type routeKey struct {
		adapter  common.Address
		tokenIn  common.Address
		tokenOut common.Address
		decimals int
	}
	expected := make(map[routeKey]bool, len(offers))
	known := make(map[routeKey]bool, len(configured))
	for _, route := range configured {
		known[routeKey{
			adapter: route.Adapter, tokenIn: route.TokenIn,
			tokenOut: route.TokenOut, decimals: route.TokenOutDecimals,
		}] = true
	}
	adapters := make(map[common.Address]bool)
	skipped := 0
	for _, offer := range offers {
		if !s.cfg.TokenPolicy.Allows(offer.TokenToRedeem) ||
			filter.adapters != nil && !filter.adapters[offer.Adapter] ||
			filter.tokenIn != (common.Address{}) && offer.TokenToRedeem != filter.tokenIn ||
			filter.tokenOut != (common.Address{}) && offer.Collateral != filter.tokenOut {
			continue
		}
		key := routeKey{
			adapter: offer.Adapter, tokenIn: offer.TokenToRedeem,
			tokenOut: offer.Collateral, decimals: offer.CollateralDecimals,
		}
		if known[key] || expected[key] {
			continue
		}
		if len(expected) == maxAdvertisedDiscountRoutes {
			skipped++
			continue
		}
		expected[key] = true
	}
	for key := range expected {
		adapters[key.adapter] = true
	}
	if skipped > 0 {
		s.log.V(1).Info(
			"ignore advertised discount routes above safety cap",
			"cap", maxAdvertisedDiscountRoutes,
			"skipped", skipped,
		)
	}
	orderedAdapters := make([]common.Address, 0, len(adapters))
	for adapter := range adapters {
		orderedAdapters = append(orderedAdapters, adapter)
	}
	if len(orderedAdapters) == 0 {
		return nil
	}
	slices.SortFunc(orderedAdapters, func(a, b common.Address) int { return a.Cmp(b) })

	routes := s.resolveAdvertisedAdapters(ctx, orderedAdapters)
	resolved := make([]liquidlane.Route, 0, len(expected))
	for _, route := range routes {
		if !expected[routeKey{
			adapter: route.Adapter, tokenIn: route.TokenIn,
			tokenOut: route.TokenOut, decimals: route.TokenOutDecimals,
		}] {
			continue
		}
		if err := s.reader.ValidateGasTokens([]liquidlane.Route{route}); err != nil {
			s.log.V(1).Info(
				"skip advertised discount route",
				"adapter", route.Adapter.Hex(),
				"tokenIn", route.TokenIn.Hex(),
				"tokenOut", route.TokenOut.Hex(),
				"error", err.Error(),
			)
			continue
		}
		resolved = append(resolved, route)
	}
	return resolved
}

func (s *Solver) resolveAdvertisedAdapters(
	ctx context.Context,
	adapters []common.Address,
) []liquidlane.Route {
	routes, err := s.reader.ResolveRoutes(ctx, adapters)
	if err == nil {
		return routes
	}
	if len(adapters) == 1 {
		s.log.Error(err, "skip unresolved advertised discount adapter", "adapter", adapters[0].Hex())
		return nil
	}
	s.log.Error(err, "batch advertised adapter resolution failed; retry individually")

	var resolved []liquidlane.Route
	for _, adapter := range adapters {
		adapterRoutes, err := s.reader.ResolveRoutes(ctx, []common.Address{adapter})
		if err != nil {
			s.log.Error(err, "skip unresolved advertised discount adapter", "adapter", adapter.Hex())
			continue
		}
		resolved = append(resolved, adapterRoutes...)
	}
	return mergeRoutes(resolved)
}

func (s *Solver) discountInventories(
	listed *liquiddiscounts.List,
	physical []liquidlane.Inventory,
	now time.Time,
) []liquidlane.Inventory {
	inventory, issues := liquiddiscounts.MatchInventories(listed, physical, liquiddiscounts.MatchOptions{
		Now: now, AllowsToken: s.cfg.TokenPolicy.Allows,
	})
	s.logDiscountIssues(issues)
	return inventory
}

func (s *Solver) discountFillQuotes(
	listed *liquiddiscounts.List,
	physical []liquidlane.FillQuote,
	now time.Time,
) []liquidlane.FillQuote {
	quotes, issues := liquiddiscounts.AdvertisedFillQuotes(listed, physical, liquiddiscounts.MatchOptions{
		Now: now, AllowsToken: s.cfg.TokenPolicy.Allows,
	})
	s.logDiscountIssues(issues)
	return quotes
}

func (s *Solver) resolveDiscount(
	ctx context.Context,
	selection liquiddiscounts.Selection,
	physical []liquidlane.FillQuote,
	now time.Time,
) (*liquiddiscounts.Signed, error) {
	if s.discounts == nil || s.cfg.Discounts == nil || selection.DiscountID == (common.Hash{}) {
		return nil, errors.New("discount route cannot be resolved")
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.cfg.Discounts.HTTPTimeout)
	defer cancel()
	return liquiddiscounts.ResolveSelected(
		requestCtx,
		s.discounts,
		selection,
		physical,
		now.Add(s.cfg.Discounts.MinimumValidity),
	)
}

func (s *Solver) logDiscountIssues(issues []liquiddiscounts.OfferIssue) {
	for _, issue := range issues {
		s.log.V(1).Info(
			"skip invalid advertised discount", "discountId", issue.DiscountID, "error", issue.Err.Error(),
		)
	}
}

func adapterSet(adapters []common.Address) map[common.Address]bool {
	set := make(map[common.Address]bool, len(adapters))
	for _, adapter := range adapters {
		set[adapter] = true
	}
	return set
}

func mergeRoutes(groups ...[]liquidlane.Route) []liquidlane.Route {
	seen := make(map[liquidlane.RouteID]bool)
	var routes []liquidlane.Route
	for _, group := range groups {
		for _, route := range group {
			if route.ID == "" || seen[route.ID] {
				continue
			}
			seen[route.ID] = true
			routes = append(routes, route)
		}
	}
	return routes
}

func directInventoriesForAdapters(
	inventory []liquidlane.Inventory,
	adapters []common.Address,
) []liquidlane.Inventory {
	allowed := adapterSet(adapters)
	direct := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if allowed[item.Adapter] {
			direct = append(direct, item)
		}
	}
	return direct
}

func directFillQuotesForAdapters(
	quotes []liquidlane.FillQuote,
	adapters []common.Address,
) []liquidlane.FillQuote {
	allowed := adapterSet(adapters)
	direct := make([]liquidlane.FillQuote, 0, len(quotes))
	for _, quote := range quotes {
		if allowed[quote.Adapter] {
			direct = append(direct, quote)
		}
	}
	return direct
}
