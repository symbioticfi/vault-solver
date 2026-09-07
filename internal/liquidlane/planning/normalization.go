package planning

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// NormalizeFixedInventory prices already-allocated inventory at its advertised rate.
// Private alternatives reserve the price buffer before their input capacity is derived.
func NormalizeFixedInventory(inventory []liquidlane.Inventory, priceBufferBps int) []liquidlane.QuoteCandidate {
	candidates := make([]liquidlane.QuoteCandidate, 0, len(inventory))
	for _, item := range inventory {
		capacity := item.MaxAssets
		if item.DiscountID != nil {
			capacity = applyBpsDown(capacity, bpsDenominator-priceBufferBps)
		}
		if candidate := newQuoteCandidate(item, capacity); candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}
	return candidates
}

// newQuoteCandidate converts a fixed-rate inventory alternative and its
// already-buffered output capacity into the canonical greedy quote shape.
func newQuoteCandidate(item liquidlane.Inventory, maxAmountOut *big.Int) *liquidlane.QuoteCandidate {
	if item.MaxRate == nil || item.MaxRate.Sign() <= 0 || maxAmountOut == nil || maxAmountOut.Sign() <= 0 {
		return nil
	}
	candidate := &liquidlane.QuoteCandidate{
		ID: liquidlane.NewCandidateID(item.Route, item.DiscountID), Route: item.Route,
		Rate: bigmath.Clone(item.MaxRate), MaxAmountOut: bigmath.Clone(maxAmountOut),
		DiscountID: liquidlane.CloneHash(item.DiscountID), ValidUntil: item.ValidUntil,
	}
	candidate.MaxAmountIn = liquidlane.MaxAmountInForRate(maxAmountOut, candidate.Rate, item.TokenInDecimals, item.TokenOutDecimals)
	payable := liquidlane.AmountOutForRate(candidate.MaxAmountIn, candidate.Rate, item.TokenInDecimals, item.TokenOutDecimals)
	if candidate.MaxAmountIn.Sign() <= 0 || payable.Sign() <= 0 {
		return nil
	}
	return candidate
}

// NormalizeOracleInventory joins current physical observations to advertised
// alternatives. Capacity uses the tighter bound; pricing follows direct/private
// contract rounding before producing the common allocation representation.
func NormalizeOracleInventory(amountIn *big.Int, sources []liquidlane.Inventory, physical []liquidlane.FillQuote) []liquidlane.QuoteCandidate {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}
	observed := make(map[liquidlane.RouteID]liquidlane.FillQuote)
	for _, quote := range physical {
		if quote.ID != "" && quote.AmountIn != nil && quote.AmountIn.Cmp(amountIn) == 0 && quote.MaxAmountOut != nil && quote.MaxAmountOut.Sign() > 0 {
			observed[quote.ID] = quote
		}
	}
	var candidates []liquidlane.QuoteCandidate
	used := make(map[liquidlane.CandidateID]bool)
	for _, source := range sources {
		quote, found := observed[source.ID]
		if !found {
			continue
		}
		candidate := normalizeOracleSource(source, quote, amountIn)
		if candidate != nil && !used[candidate.ID] {
			used[candidate.ID] = true
			candidates = append(candidates, *candidate)
		}
	}
	return candidates
}

func normalizeOracleSource(source liquidlane.Inventory, quote liquidlane.FillQuote, amountIn *big.Int) *liquidlane.QuoteCandidate {
	if source.MaxAssets == nil || quote.MaxAssets == nil || source.MaxAssets.Sign() <= 0 || quote.MaxAssets.Sign() <= 0 {
		return nil
	}
	capacity := source.MaxAssets
	if quote.MaxAssets.Cmp(capacity) < 0 {
		capacity = quote.MaxAssets
	}
	if source.DiscountID != nil {
		// Adapter oracle output is floored before discounting. The advertised net rate
		// must be made conservative for that order of operations.
		source.MaxRate = liquidlane.ConservativeAdvertisedRate(amountIn, source.MaxRate, source.TokenInDecimals, source.TokenOutDecimals)
	} else {
		rate := liquidlane.RateForAmountOut(quote.MaxAmountOut, amountIn, source.TokenInDecimals, source.TokenOutDecimals)
		if source.MaxRate == nil || source.MaxRate.Cmp(rate) < 0 {
			return nil
		}
		source.MaxRate = rate
	}
	return newQuoteCandidate(source, capacity)
}
