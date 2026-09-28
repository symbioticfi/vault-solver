package greedy

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// NewQuoteCandidate converts an inventory alternative and its already-buffered
// output capacity into the canonical greedy quote shape. With the oracle price
// and payout discount it prices amounts exactly like the adapter; otherwise it
// uses the inventory's fixed rate.
func NewQuoteCandidate(
	item liquidlane.Inventory,
	maxAmountOut *big.Int,
) *liquidlane.QuoteCandidate {
	if maxAmountOut == nil || maxAmountOut.Sign() <= 0 {
		return nil
	}
	candidate := liquidlane.QuoteCandidate{
		ID:           liquidlane.NewCandidateID(item.Route, item.DiscountID),
		Route:        item.Route,
		Rate:         liquidlane.CloneBig(item.MaxRate),
		MaxAmountOut: liquidlane.CloneBig(maxAmountOut),
		DiscountID:   liquidlane.CloneHash(item.DiscountID),
		ValidUntil:   item.ValidUntil,
	}
	discount := item.PayoutDiscount()
	if rate := liquidlane.DiscountedRate(item.Price, discount); rate.Sign() > 0 {
		candidate.Rate = rate
		candidate.Price = liquidlane.CloneBig(item.Price)
		candidate.Discount = liquidlane.CloneBig(discount)
		candidate.MaxAmountIn = liquidlane.MaxAmountInForDiscountedAmountOut(
			maxAmountOut, item.Price, discount, item.TokenInDecimals, item.TokenOutDecimals,
		)
	} else {
		if item.MaxRate == nil || item.MaxRate.Sign() <= 0 {
			return nil
		}
		candidate.MaxAmountIn = liquidlane.MaxAmountInForRate(
			maxAmountOut, item.MaxRate, item.TokenInDecimals, item.TokenOutDecimals,
		)
	}
	if candidate.MaxAmountIn.Sign() <= 0 || candidate.AmountOutFor(candidate.MaxAmountIn).Sign() <= 0 {
		return nil
	}
	return &candidate
}

// NormalizeOracleInventory turns amount-independent RFQ inventory into exact-input
// greedy candidates using current, per-physical-route adapter quotes.
func NormalizeOracleInventory(
	amountIn *big.Int,
	sources []liquidlane.Inventory,
	physical []liquidlane.FillQuote,
) []liquidlane.QuoteCandidate {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}
	quotes := make(map[liquidlane.RouteID]liquidlane.FillQuote, len(physical))
	for _, quote := range physical {
		if quote.ID == "" || quote.AmountIn == nil || quote.AmountIn.Cmp(amountIn) != 0 ||
			quote.MaxAmountOut == nil || quote.MaxAmountOut.Sign() <= 0 {
			continue
		}
		quotes[quote.ID] = quote
	}
	seen := make(map[liquidlane.CandidateID]bool, len(sources))
	out := make([]liquidlane.QuoteCandidate, 0, len(sources))
	for _, source := range sources {
		quote, ok := quotes[source.ID]
		if !ok || source.MaxAssets == nil || source.MaxAssets.Sign() <= 0 ||
			quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0 {
			continue
		}
		capacity := liquidlane.CloneBig(source.MaxAssets)
		if quote.MaxAssets.Cmp(capacity) < 0 {
			capacity.Set(quote.MaxAssets)
		}
		var rate *big.Int
		switch {
		case source.DiscountID == nil:
			rate = liquidlane.RateForAmountOut(
				quote.MaxAmountOut,
				amountIn,
				source.TokenInDecimals,
				source.TokenOutDecimals,
			)
			if source.MaxRate == nil || source.MaxRate.Cmp(rate) < 0 {
				continue
			}
		case source.Discount != nil:
			// The signed discount prices the leg exactly like the adapter at this amount. Below the
			// adapter minimum the swap would revert.
			if quote.MinDiscount == nil || source.Discount.Cmp(quote.MinDiscount) < 0 {
				continue
			}
			rate = liquidlane.RateForAmountOut(
				liquidlane.AmountOutAfterDiscount(quote.GrossAmountOut, source.Discount),
				amountIn,
				source.TokenInDecimals,
				source.TokenOutDecimals,
			)
		default:
			// Without the signed discount only the backend's advertised maxRate is known. It already has
			// the discount applied and floored, while the adapter floors getAmountOut first and discounts
			// second. Re-derive a rate that cannot predict above what the adapter pays.
			rate = liquidlane.ConservativeAdvertisedRate(
				amountIn,
				source.MaxRate,
				source.TokenInDecimals,
				source.TokenOutDecimals,
			)
		}
		if rate == nil || rate.Sign() <= 0 {
			continue
		}
		source.MaxRate = rate
		// The amount-specific quote is fresher than a snapshot price, so price at its rate.
		source.Price, source.Discount = nil, nil
		candidate := NewQuoteCandidate(source, capacity)
		if candidate == nil || seen[candidate.ID] {
			continue
		}
		seen[candidate.ID] = true
		out = append(out, *candidate)
	}
	return out
}
