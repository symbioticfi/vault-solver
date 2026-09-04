package planning

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// NewQuoteCandidate converts a fixed-rate inventory alternative and its
// already-buffered output capacity into the canonical greedy quote shape.
func NewQuoteCandidate(
	item liquidlane.Inventory,
	maxAmountOut *big.Int,
) *liquidlane.QuoteCandidate {
	if item.MaxRate == nil || item.MaxRate.Sign() <= 0 ||
		maxAmountOut == nil || maxAmountOut.Sign() <= 0 {
		return nil
	}
	maxAmountIn := liquidlane.MaxAmountInForRate(
		maxAmountOut,
		item.MaxRate,
		item.TokenInDecimals,
		item.TokenOutDecimals,
	)
	if maxAmountIn.Sign() <= 0 || liquidlane.AmountOutForRate(
		maxAmountIn,
		item.MaxRate,
		item.TokenInDecimals,
		item.TokenOutDecimals,
	).Sign() <= 0 {
		return nil
	}
	return &liquidlane.QuoteCandidate{
		ID:           liquidlane.NewCandidateID(item.Route, item.DiscountID),
		Route:        item.Route,
		Rate:         liquidlane.CloneBig(item.MaxRate),
		MaxAmountIn:  maxAmountIn,
		MaxAmountOut: liquidlane.CloneBig(maxAmountOut),
		DiscountID:   liquidlane.CloneHash(item.DiscountID),
		ValidUntil:   item.ValidUntil,
	}
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
	quotes := indexPhysicalQuotes(amountIn, physical)
	seen := make(map[liquidlane.CandidateID]struct{}, len(sources))
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
		rate := normalizedRate(amountIn, source, quote)
		if rate == nil || rate.Sign() <= 0 {
			continue
		}
		source.MaxRate = rate
		candidate := NewQuoteCandidate(source, capacity)
		if candidate == nil {
			continue
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			continue
		}
		seen[candidate.ID] = struct{}{}
		out = append(out, *candidate)
	}
	return out
}

func indexPhysicalQuotes(amountIn *big.Int, physical []liquidlane.FillQuote) map[liquidlane.RouteID]liquidlane.FillQuote {
	quotes := make(map[liquidlane.RouteID]liquidlane.FillQuote, len(physical))
	for _, quote := range physical {
		if quote.ID != "" && quote.AmountIn != nil && quote.AmountIn.Cmp(amountIn) == 0 &&
			quote.MaxAmountOut != nil && quote.MaxAmountOut.Sign() > 0 {
			quotes[quote.ID] = quote
		}
	}
	return quotes
}

func normalizedRate(amountIn *big.Int, source liquidlane.Inventory, quote liquidlane.FillQuote) *big.Int {
	if source.DiscountID != nil {
		// The advertised discount rate is already floored. Re-derive the largest
		// fixed-point rate that cannot exceed the adapter's integer payout.
		return liquidlane.ConservativeAdvertisedRate(
			amountIn, source.MaxRate, source.TokenInDecimals, source.TokenOutDecimals,
		)
	}
	rate := liquidlane.RateForAmountOut(
		quote.MaxAmountOut, amountIn, source.TokenInDecimals, source.TokenOutDecimals,
	)
	if source.MaxRate == nil || source.MaxRate.Cmp(rate) < 0 {
		return nil
	}
	return rate
}
