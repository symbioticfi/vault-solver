package greedy

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
		rate := source.MaxRate
		if source.DiscountID == nil {
			rate = liquidlane.RateForAmountOut(
				quote.MaxAmountOut,
				amountIn,
				source.TokenInDecimals,
				source.TokenOutDecimals,
			)
			if source.MaxRate == nil || source.MaxRate.Cmp(rate) < 0 {
				continue
			}
		}
		if rate == nil || rate.Sign() <= 0 {
			continue
		}
		source.MaxRate = rate
		candidate := NewQuoteCandidate(source, capacity)
		if candidate == nil || seen[candidate.ID] {
			continue
		}
		seen[candidate.ID] = true
		out = append(out, *candidate)
	}
	return out
}
