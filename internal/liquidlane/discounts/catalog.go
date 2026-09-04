package discounts

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type OfferIssue struct {
	DiscountID string
	Err        error
}

type MatchOptions struct {
	Now         time.Time
	AllowsToken func(common.Address) bool
}

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

func MatchInventories(
	listed *List,
	physical []liquidlane.Inventory,
	options MatchOptions,
) ([]liquidlane.Inventory, []OfferIssue) {
	baseByRoute := make(map[routeIdentity]liquidlane.Inventory, len(physical))
	for _, base := range physical {
		baseByRoute[routeOf(base.Adapter, base.TokenIn, base.TokenOut)] = base
	}
	matcher := newMatcher(listed, options)
	result := make([]liquidlane.Inventory, 0, len(matcher.offers))
	for _, offer := range matcher.offers {
		if !matcher.acceptIdentity(offer) {
			continue
		}
		base, exists := baseByRoute[routeOf(offer.Adapter, offer.TokenToRedeem, offer.Collateral)]
		if !exists || offer.CollateralDecimals != base.TokenOutDecimals {
			continue
		}
		if err := validateEconomics(offer, base.MaxRate, base.AdapterMinDiscount); err != nil {
			matcher.reject(offer, err)
			continue
		}
		capacity := positiveMinimum(offer.MaxAssets, base.MaxAssets)
		if capacity.Sign() <= 0 {
			continue
		}
		matcher.commit(offer)
		candidate := liquidlane.DiscountInventory(
			base.Route, capacity, offer.MaxRate, offer.DiscountID, time.Unix(offer.Deadline, 0),
		)
		candidate.AdapterMinDiscount = liquidlane.CloneBig(base.AdapterMinDiscount)
		result = append(result, candidate)
	}
	return result, matcher.issues
}

func AdvertisedFillQuotes(
	listed *List,
	physical []liquidlane.FillQuote,
	options MatchOptions,
) ([]liquidlane.FillQuote, []OfferIssue) {
	baseByRoute := make(map[routeIdentity]liquidlane.FillQuote, len(physical))
	for _, base := range physical {
		baseByRoute[routeOf(base.Adapter, base.TokenIn, base.TokenOut)] = base
	}
	matcher := newMatcher(listed, options)
	result := make([]liquidlane.FillQuote, 0, len(matcher.offers))
	for _, offer := range matcher.offers {
		if !matcher.acceptIdentity(offer) {
			continue
		}
		base, exists := baseByRoute[routeOf(offer.Adapter, offer.TokenToRedeem, offer.Collateral)]
		if !exists || offer.CollateralDecimals != base.TokenOutDecimals {
			continue
		}
		if err := validateEconomics(offer, base.MaxRate, base.MinDiscount); err != nil {
			matcher.reject(offer, err)
			continue
		}
		rate, amountOut := advertisedOutput(base, offer)
		capacity := positiveMinimum(offer.MaxAssets, base.MaxAssets)
		if rate.Sign() <= 0 || amountOut.Sign() <= 0 || capacity.Sign() <= 0 {
			continue
		}
		matcher.commit(offer)
		inventory := liquidlane.DiscountInventory(
			base.Route, capacity, rate, offer.DiscountID, time.Unix(offer.Deadline, 0),
		)
		inventory.AdapterMinDiscount = liquidlane.CloneBig(base.AdapterMinDiscount)
		result = append(result, liquidlane.FillQuote{
			Inventory: inventory, AmountIn: liquidlane.CloneBig(base.AmountIn),
			GrossAmountOut: liquidlane.CloneBig(base.GrossAmountOut), MaxAmountOut: amountOut,
			MinDiscount: liquidlane.CloneBig(offer.Discount),
		})
	}
	return result, matcher.issues
}

type matcher struct {
	offers  []Offer
	issues  []OfferIssue
	options MatchOptions
	used    map[common.Hash]struct{}
}

func newMatcher(listed *List, options MatchOptions) *matcher {
	offers, issues := LiveOffers(listed, options.Now)
	return &matcher{offers: offers, issues: issues, options: options, used: make(map[common.Hash]struct{})}
}

func (matcher *matcher) acceptIdentity(offer Offer) bool {
	if _, used := matcher.used[offer.DiscountID]; used {
		return false
	}
	return matcher.options.AllowsToken == nil || matcher.options.AllowsToken(offer.TokenToRedeem)
}

func (matcher *matcher) commit(offer Offer) { matcher.used[offer.DiscountID] = struct{}{} }

func (matcher *matcher) reject(offer Offer, err error) {
	matcher.issues = append(matcher.issues, OfferIssue{DiscountID: offer.DiscountID.Hex(), Err: err})
}

type routeIdentity struct {
	adapter, tokenIn, tokenOut common.Address
}

func routeOf(adapter, tokenIn, tokenOut common.Address) routeIdentity {
	return routeIdentity{adapter: adapter, tokenIn: tokenIn, tokenOut: tokenOut}
}

func validateEconomics(offer Offer, maxRate, minDiscount *big.Int) error {
	if maxRate == nil || maxRate.Sign() <= 0 || offer.MaxRate.Cmp(maxRate) > 0 {
		return errors.New("advertised discount rate exceeds current adapter max rate")
	}
	if minDiscount == nil || minDiscount.Sign() < 0 || offer.Discount.Cmp(minDiscount) < 0 {
		return errors.New("advertised discount is below current adapter minimum")
	}
	return nil
}

func advertisedOutput(base liquidlane.FillQuote, offer Offer) (rate, amountOut *big.Int) {
	adapterOutput := liquidlane.AmountOutAfterDiscount(base.GrossAmountOut, offer.Discount)
	currentRate := liquidlane.RateForAmountOut(
		adapterOutput, base.AmountIn, base.TokenInDecimals, base.TokenOutDecimals,
	)
	rate = positiveMinimum(currentRate, offer.MaxRate)
	amountOut = liquidlane.AmountOutForRate(base.AmountIn, rate, base.TokenInDecimals, base.TokenOutDecimals)
	return rate, amountOut
}

func positiveMinimum(left, right *big.Int) *big.Int {
	if left == nil || right == nil || left.Sign() <= 0 || right.Sign() <= 0 {
		return new(big.Int)
	}
	if left.Cmp(right) <= 0 {
		return liquidlane.CloneBig(left)
	}
	return liquidlane.CloneBig(right)
}
