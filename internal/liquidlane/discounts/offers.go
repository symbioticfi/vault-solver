package discounts

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// OfferIssue describes an advertised discount rejected as malformed or inconsistent with current state.
type OfferIssue struct {
	DiscountID string
	Err        error
}

// MatchOptions contains solver policy applied before an offer becomes a LiquidLane candidate.
type MatchOptions struct {
	Now         time.Time
	AllowsToken func(common.Address) bool
}

// LiveOffers parses advertised discounts and removes entries already expired at now.
func LiveOffers(listed *List, now time.Time) ([]Offer, []OfferIssue) {
	if listed == nil {
		return nil, nil
	}
	offers := make([]Offer, 0, len(listed.Discounts))
	issues := make([]OfferIssue, 0)
	for _, item := range listed.Discounts {
		offer, err := ParseOffer(item)
		if err != nil {
			issues = append(issues, OfferIssue{DiscountID: item.DiscountID, Err: err})
			continue
		}
		if offer.Deadline > now.Unix() {
			offers = append(offers, *offer)
		}
	}
	return offers, issues
}

// MatchInventories maps advertised discounts onto current physical routes.
func MatchInventories(
	listed *List,
	physical []liquidlane.Inventory,
	options MatchOptions,
) ([]liquidlane.Inventory, []OfferIssue) {
	byRoute := inventoryByRoute(physical)
	offers, issues := LiveOffers(listed, options.Now)
	seen := make(map[common.Hash]bool, len(offers))
	inventory := make([]liquidlane.Inventory, 0, len(offers))
	for _, offer := range offers {
		if seen[offer.DiscountID] || !tokenAllowed(offer.TokenToRedeem, options) {
			continue
		}
		base, ok := byRoute[newRouteKey(offer.Adapter, offer.TokenToRedeem, offer.Collateral)]
		if !ok || offer.CollateralDecimals != base.TokenOutDecimals {
			continue
		}
		if err := validateOfferEconomics(offer, base.MaxRate, base.AdapterMinDiscount); err != nil {
			issues = append(issues, OfferIssue{DiscountID: offer.DiscountID.Hex(), Err: err})
			continue
		}
		maxAssets := minPositive(offer.MaxAssets, base.MaxAssets)
		if maxAssets.Sign() <= 0 {
			continue
		}
		seen[offer.DiscountID] = true
		candidate := liquidlane.DiscountInventory(
			base.Route,
			maxAssets,
			offer.MaxRate,
			offer.DiscountID,
			time.Unix(offer.Deadline, 0),
		)
		candidate.AdapterMinDiscount = liquidlane.CloneBig(base.AdapterMinDiscount)
		inventory = append(inventory, candidate)
	}
	return inventory, issues
}

// AdvertisedFillQuotes prices advertised discounts against current amount-specific adapter quotes.
func AdvertisedFillQuotes(
	listed *List,
	physical []liquidlane.FillQuote,
	options MatchOptions,
) ([]liquidlane.FillQuote, []OfferIssue) {
	byRoute := fillQuotesByRoute(physical)
	offers, issues := LiveOffers(listed, options.Now)
	seen := make(map[common.Hash]bool, len(offers))
	quotes := make([]liquidlane.FillQuote, 0, len(offers))
	for _, offer := range offers {
		if seen[offer.DiscountID] || !tokenAllowed(offer.TokenToRedeem, options) {
			continue
		}
		base, ok := byRoute[newRouteKey(offer.Adapter, offer.TokenToRedeem, offer.Collateral)]
		if !ok || offer.CollateralDecimals != base.TokenOutDecimals {
			continue
		}
		if err := validateOfferEconomics(offer, base.MaxRate, base.MinDiscount); err != nil {
			issues = append(issues, OfferIssue{DiscountID: offer.DiscountID.Hex(), Err: err})
			continue
		}
		amountOut := liquidlane.AmountOutAfterDiscount(base.GrossAmountOut, offer.Discount)
		currentRate := liquidlane.RateForAmountOut(
			amountOut,
			base.AmountIn,
			base.TokenInDecimals,
			base.TokenOutDecimals,
		)
		maxRate := minPositive(currentRate, offer.MaxRate)
		maxAmountOut := liquidlane.AmountOutForRate(
			base.AmountIn,
			maxRate,
			base.TokenInDecimals,
			base.TokenOutDecimals,
		)
		maxAssets := minPositive(offer.MaxAssets, base.MaxAssets)
		if maxRate.Sign() <= 0 || maxAmountOut.Sign() <= 0 || maxAssets.Sign() <= 0 {
			continue
		}
		seen[offer.DiscountID] = true
		inventory := liquidlane.DiscountInventory(
			base.Route,
			maxAssets,
			maxRate,
			offer.DiscountID,
			time.Unix(offer.Deadline, 0),
		)
		inventory.AdapterMinDiscount = liquidlane.CloneBig(base.AdapterMinDiscount)
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory:      inventory,
			AmountIn:       liquidlane.CloneBig(base.AmountIn),
			GrossAmountOut: liquidlane.CloneBig(base.GrossAmountOut),
			MaxAmountOut:   maxAmountOut,
			MinDiscount:    liquidlane.CloneBig(offer.Discount),
		})
	}
	return quotes, issues
}

func validateOfferEconomics(offer Offer, currentMaxRate, currentMinDiscount *big.Int) error {
	if currentMaxRate == nil || currentMaxRate.Sign() <= 0 || offer.MaxRate.Cmp(currentMaxRate) > 0 {
		return errors.New("advertised discount rate exceeds current adapter max rate")
	}
	if currentMinDiscount == nil || currentMinDiscount.Sign() < 0 || offer.Discount.Cmp(currentMinDiscount) < 0 {
		return errors.New("advertised discount is below current adapter minimum")
	}
	return nil
}

type routeKey struct {
	adapter  common.Address
	tokenIn  common.Address
	tokenOut common.Address
}

func newRouteKey(adapter, tokenIn, tokenOut common.Address) routeKey {
	return routeKey{adapter: adapter, tokenIn: tokenIn, tokenOut: tokenOut}
}

func inventoryByRoute(inventory []liquidlane.Inventory) map[routeKey]liquidlane.Inventory {
	byRoute := make(map[routeKey]liquidlane.Inventory, len(inventory))
	for _, item := range inventory {
		byRoute[newRouteKey(item.Adapter, item.TokenIn, item.TokenOut)] = item
	}
	return byRoute
}

func fillQuotesByRoute(quotes []liquidlane.FillQuote) map[routeKey]liquidlane.FillQuote {
	byRoute := make(map[routeKey]liquidlane.FillQuote, len(quotes))
	for _, quote := range quotes {
		byRoute[newRouteKey(quote.Adapter, quote.TokenIn, quote.TokenOut)] = quote
	}
	return byRoute
}

func tokenAllowed(token common.Address, options MatchOptions) bool {
	return options.AllowsToken == nil || options.AllowsToken(token)
}

func minPositive(left, right *big.Int) *big.Int {
	if left == nil || right == nil || left.Sign() <= 0 || right.Sign() <= 0 {
		return new(big.Int)
	}
	if left.Cmp(right) <= 0 {
		return liquidlane.CloneBig(left)
	}
	return liquidlane.CloneBig(right)
}
