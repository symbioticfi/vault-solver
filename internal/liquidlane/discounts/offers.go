package discounts

import (
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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

// MatchInventories binds advertised terms to the current physical capacity.
func MatchInventories(listed *List, physical []liquidlane.Inventory, options MatchOptions) ([]liquidlane.Inventory, []OfferIssue) {
	byRoute := make(map[routeKey]liquidlane.Inventory, len(physical))
	for _, base := range physical {
		byRoute[routeKey{base.Adapter, base.TokenIn, base.TokenOut}] = base
	}
	var out []liquidlane.Inventory
	issues := matchOffers(listed, options, func(offer Offer) (bool, error) {
		base, ok := byRoute[routeKey{offer.Adapter, offer.TokenToRedeem, offer.Collateral}]
		if !ok {
			return false, nil
		}
		candidate, err := bindOffer(offer, base, base.AdapterMinDiscount)
		if err != nil || candidate == nil {
			return false, err
		}
		out = append(out, *candidate)
		return true, nil
	})
	return out, issues
}

// AdvertisedFillQuotes applies the signed discount once to an amount-specific gross quote.
func AdvertisedFillQuotes(listed *List, physical []liquidlane.FillQuote, options MatchOptions) ([]liquidlane.FillQuote, []OfferIssue) {
	byRoute := make(map[routeKey]liquidlane.FillQuote, len(physical))
	for _, base := range physical {
		byRoute[routeKey{base.Adapter, base.TokenIn, base.TokenOut}] = base
	}
	var out []liquidlane.FillQuote
	issues := matchOffers(listed, options, func(offer Offer) (bool, error) {
		base, ok := byRoute[routeKey{offer.Adapter, offer.TokenToRedeem, offer.Collateral}]
		if !ok {
			return false, nil
		}
		inventory, err := bindOffer(offer, base.Inventory, base.MinDiscount)
		if err != nil || inventory == nil {
			return false, err
		}
		net := liquidlane.AmountOutAfterDiscount(base.GrossAmountOut, offer.Discount)
		rate := liquidlane.RateForAmountOut(net, base.AmountIn, base.TokenInDecimals, base.TokenOutDecimals)
		inventory.MaxRate = minPositive(rate, offer.MaxRate)
		amountOut := liquidlane.AmountOutForRate(base.AmountIn, inventory.MaxRate, base.TokenInDecimals, base.TokenOutDecimals)
		if amountOut.Sign() <= 0 {
			return false, nil
		}
		out = append(out, liquidlane.FillQuote{
			Inventory: *inventory, AmountIn: bigmath.Clone(base.AmountIn),
			GrossAmountOut: bigmath.Clone(base.GrossAmountOut),
			MaxAmountOut:   amountOut, MinDiscount: bigmath.Clone(offer.Discount),
		})
		return true, nil
	})
	return out, issues
}

// An ID is consumed only after a usable candidate was built. A malformed duplicate
// must not hide a later valid advertisement for that ID.
func matchOffers(listed *List, options MatchOptions, accept func(Offer) (bool, error)) []OfferIssue {
	offers, issues := LiveOffers(listed, options.Now)
	seen := make(map[common.Hash]bool, len(offers))
	for _, offer := range offers {
		if seen[offer.DiscountID] || (options.AllowsToken != nil && !options.AllowsToken(offer.TokenToRedeem)) {
			continue
		}
		accepted, err := accept(offer)
		if err != nil {
			issues = append(issues, OfferIssue{DiscountID: offer.DiscountID.Hex(), Err: err})
		}
		if accepted {
			seen[offer.DiscountID] = true
		}
	}
	return issues
}

func bindOffer(offer Offer, base liquidlane.Inventory, minimum *big.Int) (*liquidlane.Inventory, error) {
	if offer.CollateralDecimals != base.TokenOutDecimals {
		return nil, nil
	}
	if base.MaxRate == nil || base.MaxRate.Sign() <= 0 || offer.MaxRate.Cmp(base.MaxRate) > 0 {
		return nil, errors.New("advertised discount rate exceeds current adapter max rate")
	}
	if minimum == nil || minimum.Sign() < 0 || offer.Discount.Cmp(minimum) < 0 {
		return nil, errors.New("advertised discount is below current adapter minimum")
	}
	capacity := minPositive(offer.MaxAssets, base.MaxAssets)
	if capacity.Sign() <= 0 {
		return nil, nil
	}
	out := liquidlane.DiscountInventory(base.Route, capacity, offer.MaxRate, offer.DiscountID, time.Unix(offer.Deadline, 0))
	out.AdapterMinDiscount = bigmath.Clone(base.AdapterMinDiscount)
	return &out, nil
}

type routeKey struct {
	adapter  common.Address
	tokenIn  common.Address
	tokenOut common.Address
}

func minPositive(left, right *big.Int) *big.Int {
	if left == nil || right == nil || left.Sign() <= 0 || right.Sign() <= 0 {
		return new(big.Int)
	}
	return bigmath.Min(left, right)
}
