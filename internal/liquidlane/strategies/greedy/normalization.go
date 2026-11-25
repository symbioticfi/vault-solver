package greedy

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

// NewQuoteCandidate converts an inventory alternative and its already-buffered
// output capacity into the canonical greedy quote shape. With the oracle price
// and payout discount it prices amounts exactly like the adapter; otherwise it
// uses the inventory's fixed rate.
func NewQuoteCandidate(
	item liquidlane.Inventory,
	maxAmountOut *big.Int,
) *liquidlane.QuoteCandidate {
	return NewQuoteCandidateWithTrace(item, maxAmountOut, nil)
}

// NewQuoteCandidateWithTrace also records the check that dropped a candidate.
func NewQuoteCandidateWithTrace(
	item liquidlane.Inventory, maxAmountOut *big.Int, trace strategies.DecisionTrace,
) *liquidlane.QuoteCandidate {
	if maxAmountOut == nil || maxAmountOut.Sign() <= 0 {
		trace.Decline("inventory", "buffered output capacity is not positive", "route", item.ID, "capacity", maxAmountOut)
		return nil
	}
	candidate := liquidlane.QuoteCandidate{
		DiscountPrecision: item.DiscountPrecision,
		ID:                liquidlane.NewCandidateID(item.Route, item.DiscountID),
		Route:             item.Route,
		Rate:              liquidlane.CloneBig(item.MaxRate),
		MaxAmountOut:      liquidlane.CloneBig(maxAmountOut),
		DiscountID:        liquidlane.CloneHash(item.DiscountID),
		ValidUntil:        item.ValidUntil,
	}
	discount := item.PayoutDiscount()
	if rate := liquidlane.DiscountedRatePrecision(item.Price, discount, candidate.PricingPrecision()); rate.Sign() > 0 {
		candidate.Rate = rate
		candidate.Price = liquidlane.CloneBig(item.Price)
		candidate.Discount = liquidlane.CloneBig(discount)
		candidate.MaxAmountIn = liquidlane.MaxAmountInForDiscountedAmountOutPrecision(
			maxAmountOut, item.Price, discount, item.TokenInDecimals, item.TokenOutDecimals, candidate.PricingPrecision(),
		)
	} else {
		if item.MaxRate == nil || item.MaxRate.Sign() <= 0 {
			trace.Decline("inventory", "no usable oracle price and fixed rate is not positive", "route", item.ID,
				"price", item.Price, "discount", discount, "fixedRate", item.MaxRate)
			return nil
		}
		candidate.MaxAmountIn = liquidlane.MaxAmountInForRate(
			maxAmountOut, item.MaxRate, item.TokenInDecimals, item.TokenOutDecimals,
		)
	}
	if candidate.MaxAmountIn.Sign() <= 0 {
		trace.Decline("inventory", "buffered capacity rounds maximum input below one input unit", "route", item.ID,
			"capacity", maxAmountOut, "rate", candidate.Rate, "maxInput", candidate.MaxAmountIn)
		return nil
	}
	if out := candidate.AmountOutFor(candidate.MaxAmountIn); out.Sign() <= 0 {
		trace.Decline("inventory", "maximum input rounds down to zero output", "route", item.ID,
			"maxInput", candidate.MaxAmountIn, "rate", candidate.Rate)
		return nil
	}
	return &candidate
}

// NormalizeOracleInventory turns amount-independent RFQ inventory into exact-input
// greedy candidates using current, per-physical-route adapter quotes. Each source it drops is
// declined on trace with the failed check and its values.
func NormalizeOracleInventory(
	amountIn *big.Int,
	sources []liquidlane.Inventory,
	physical []liquidlane.FillQuote,
	trace strategies.DecisionTrace,
) []liquidlane.QuoteCandidate {
	if amountIn == nil || amountIn.Sign() <= 0 {
		trace.Decline("inventory", "amount in is not positive", "amountIn", amountIn, "sources", len(sources))
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
		switch {
		case !ok:
			trace.Decline("inventory", "no adapter quote with output for amount in", "route", source.ID, "amountIn", amountIn)
			continue
		case source.MaxAssets == nil || source.MaxAssets.Sign() <= 0:
			trace.Decline("inventory", "inventory capacity is not positive", "route", source.ID, "maxAssets", source.MaxAssets)
			continue
		case quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0:
			trace.Decline("inventory", "adapter capacity is not positive", "route", source.ID, "maxAssets", quote.MaxAssets)
			continue
		}
		capacity := liquidlane.CloneBig(source.MaxAssets)
		if quote.MaxAssets.Cmp(capacity) < 0 {
			capacity.Set(quote.MaxAssets)
		}
		discount, reason := payoutDiscount(source, quote)
		if discount == nil {
			trace.Decline("inventory", reason, "route", source.ID,
				"discount", source.Discount, "adapterMinDiscount", quote.MinDiscount)
			continue
		}
		// Price from the amount-specific quote, which is fresher than a snapshot price. A floored
		// rate would price amountIn below the adapter's payout and miss an award quoted exactly.
		source.Price = liquidlane.ImpliedPrice(amountIn, quote.GrossAmountOut, source.TokenInDecimals, source.TokenOutDecimals)
		source.MaxRate = nil
		if source.DiscountID != nil {
			source.Discount = discount
		} else {
			source.AdapterMinDiscount = discount
		}
		candidate := NewQuoteCandidate(source, capacity)
		if candidate == nil {
			trace.Decline("inventory", "candidate prices no output", "route", source.ID,
				"capacity", capacity, "price", source.Price, "discount", discount)
			continue
		}
		if seen[candidate.ID] {
			trace.Decline("inventory", "duplicate candidate", "candidate", candidate.ID)
			continue
		}
		seen[candidate.ID] = true
		out = append(out, *candidate)
	}
	return out
}

// payoutDiscount is the discount source pays at in the adapter's current state, or nil and the
// failed check when the source cannot execute. A signed discount below the adapter's minimum
// reverts. A direct swap is bounded by the adapter's current minimum; a higher minimum the
// inventory was offered at stands.
func payoutDiscount(source liquidlane.Inventory, quote liquidlane.FillQuote) (*big.Int, string) {
	if quote.MinDiscount == nil {
		return nil, "adapter quote has no minimum discount"
	}
	if source.DiscountID != nil {
		if source.Discount == nil {
			return nil, "signed discount has no value"
		}
		if source.Discount.Cmp(quote.MinDiscount) < 0 {
			return nil, "signed discount is below adapter minimum discount"
		}
		return source.Discount, ""
	}
	if source.AdapterMinDiscount != nil && source.AdapterMinDiscount.Cmp(quote.MinDiscount) > 0 {
		return source.AdapterMinDiscount, ""
	}
	return quote.MinDiscount, ""
}
