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

type discountRouteSet struct {
	routes   []liquidlane.Route
	listed   *liquiddiscounts.List
	complete bool
}

func (s *Solver) listDiscounts(ctx context.Context) (*liquiddiscounts.List, error) {
	if s.discounts == nil {
		return &liquiddiscounts.List{}, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.cfg.Discounts.HTTPTimeout)
	defer cancel()
	return s.discounts.ListDiscounts(requestCtx)
}

func (s *Solver) quoteRoutesWithDiscounts(
	ctx context.Context,
	configured []liquidlane.Route,
	now time.Time,
) (discountRouteSet, error) {
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
	result, err := s.routesWithDiscounts(ctx, configured, now, advertisedRouteFilter{
		tokenIn: tokenIn, tokenOut: tokenOut,
	})
	return result.routes, result.listed, err
}

func (s *Solver) routesWithDiscounts(
	ctx context.Context,
	configured []liquidlane.Route,
	now time.Time,
	filter advertisedRouteFilter,
) (discountRouteSet, error) {
	if !s.cfg.usesDiscounts() {
		return discountRouteSet{routes: configured, complete: true}, nil
	}
	listed, err := s.listDiscounts(ctx)
	if err != nil {
		return discountRouteSet{routes: configured}, err
	}
	dynamic, complete := s.resolveAdvertisedRoutes(ctx, listed, configured, now, filter)
	return discountRouteSet{
		routes: mergeRoutes(configured, dynamic), listed: listed, complete: complete,
	}, nil
}

func (s *Solver) resolveAdvertisedRoutes(ctx context.Context, listed *liquiddiscounts.List,
	configured []liquidlane.Route, now time.Time, filter advertisedRouteFilter,
) ([]liquidlane.Route, bool) {
	type routeKey struct {
		adapter, tokenIn, tokenOut common.Address
		decimals                   int
	}
	// true means already covered; false means advertised and still awaiting a verified read.
	covered := make(map[routeKey]bool, len(configured))
	for _, route := range configured {
		covered[routeKey{route.Adapter, route.TokenIn, route.TokenOut, route.TokenOutDecimals}] = true
	}
	offers, issues := liquiddiscounts.LiveOffers(listed, now)
	s.logDiscountIssues(issues)
	adapters := make(map[common.Address]bool)
	missing, skipped := 0, 0
	for _, offer := range offers {
		if !s.cfg.TokenPolicy.Allows(offer.TokenToRedeem) ||
			filter.adapters != nil && !filter.adapters[offer.Adapter] ||
			filter.tokenIn != (common.Address{}) && offer.TokenToRedeem != filter.tokenIn ||
			filter.tokenOut != (common.Address{}) && offer.Collateral != filter.tokenOut {
			continue
		}
		key := routeKey{offer.Adapter, offer.TokenToRedeem, offer.Collateral, offer.CollateralDecimals}
		if _, exists := covered[key]; exists {
			continue
		}
		if missing == maxAdvertisedDiscountRoutes {
			skipped++
			continue
		}
		covered[key] = false
		missing++
		adapters[offer.Adapter] = true
	}
	if skipped > 0 {
		s.log.V(1).Info("ignore advertised discount routes above safety cap", "cap", maxAdvertisedDiscountRoutes, "skipped", skipped)
	}
	ordered := make([]common.Address, 0, len(adapters))
	for adapter := range adapters {
		ordered = append(ordered, adapter)
	}
	slices.SortFunc(ordered, func(a, b common.Address) int { return a.Cmp(b) })
	var resolved []liquidlane.Route
	if len(ordered) != 0 {
		for _, route := range s.resolveAdvertisedAdapters(ctx, ordered) {
			key := routeKey{route.Adapter, route.TokenIn, route.TokenOut, route.TokenOutDecimals}
			ready, wanted := covered[key]
			if !wanted || ready {
				continue
			}
			if err := s.reader.ValidateGasTokens([]liquidlane.Route{route}); err != nil {
				s.log.V(1).Info("skip advertised discount route", "adapter", route.Adapter.Hex(), "tokenOut", route.TokenOut.Hex(), "error", err.Error())
				continue
			}
			covered[key] = true
			missing--
			resolved = append(resolved, route)
		}
	}
	return resolved, len(issues) == 0 && skipped == 0 && missing == 0
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
