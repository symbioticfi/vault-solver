package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

// Exactly priced legs pay like the adapter, which floors getAmountOut before its discount. A
// published range rate must still never promise more than the greedy plan executes at any amount.
func TestPriceQuoteRangeNeverOverquotesExactlyPricedLegs(t *testing.T) {
	strategy, err := New(Config{MinAmount: "1", RangeCount: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	type leg struct {
		adapter  string
		price    int64 // 1e10 units: 8-decimal oracle prices
		discount int64
		private  bool
		capacity int64
	}
	build := func(legs []leg) []liquidlane.QuoteCandidate {
		candidates := make([]liquidlane.QuoteCandidate, 0, len(legs))
		for index, leg := range legs {
			route := liquidlane.NewRoute(1, common.HexToAddress(leg.adapter), common.HexToAddress(leg.adapter),
				tokenIn, tokenOut, 18, 6)
			price := new(big.Int).Mul(big.NewInt(leg.price), big.NewInt(10_000_000_000))
			discount := big.NewInt(leg.discount)
			item := liquidlane.DirectInventory(route, big.NewInt(leg.capacity), liquidlane.DiscountedRate(price, discount))
			item.Price, item.AdapterMinDiscount = price, new(big.Int)
			if leg.private {
				item = liquidlane.DiscountInventory(route, item.MaxAssets, item.MaxRate,
					common.BigToHash(big.NewInt(int64(index+1))), time.Time{})
				item.Price, item.AdapterMinDiscount, item.Discount = price, new(big.Int), discount
			}
			candidate := liquidgreedy.NewQuoteCandidate(item, item.MaxAssets)
			if candidate == nil || !candidate.ExactPricing() {
				t.Fatalf("leg %d candidate = %+v", index, candidate)
			}
			candidates = append(candidates, *candidate)
		}
		return candidates
	}
	legSets := []struct {
		name string
		legs []leg
	}{
		{"three routes", []leg{
			{"0xa", 112_837_214, 200, true, 500_000_000},
			{"0xb", 106_177_400, 0, false, 700_000_000},
			{"0xc", 115_013_646, 1_000, true, 300_000_000},
		}},
		// JTRSY at a 4% discount: its 6-decimal price makes the discounted per-token rate exact,
		// so only the adapter's double floor separates the range rate from the payout.
		{"one exact-rate route", []leg{{"0xd", 111_727_500, 40_000, true, 1_000_000_000}}},
	}
	pricing, err := liquidstrategies.NewGasPricing(big.NewInt(0), tokenOut, nil, nil, 0, liquidstrategies.GasEnvelope{})
	if err != nil {
		t.Fatalf("NewGasPricing: %v", err)
	}

	token := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	for _, set := range legSets {
		name, candidates := set.name, build(set.legs)
		routes, private := len(set.legs), 0
		for _, leg := range set.legs {
			if leg.private {
				private++
			}
		}
		for _, bounds := range [][2]int64{{1, 400}, {400, 900}, {1, 800}} {
			lower := new(big.Int).Mul(big.NewInt(bounds[0]), token)
			upper := new(big.Int).Mul(big.NewInt(bounds[1]), token)
			quoteRange, err := strategy.priceQuoteRange(
				candidates, routes, lower, upper, pricing.MaxCost(routes, private), routes, pricing,
			)
			if err != nil || quoteRange == nil {
				t.Fatalf("%s range %v: %+v, %v", name, bounds, quoteRange, err)
			}
			// The range quote is output per input in whole tokens; scale it back to a 1e18 base-unit rate.
			quote, ok := new(big.Rat).SetString(quoteRange.Quote)
			if !ok {
				t.Fatalf("invalid quote rate %q", quoteRange.Quote)
			}
			scaled := new(big.Rat).Mul(quote, new(big.Rat).SetInt(token))
			if !scaled.IsInt() {
				t.Fatalf("quote %q has more than 18 decimals", quoteRange.Quote)
			}
			rate := scaled.Num()
			// 3,001 amounts spread over the whole range in a scrambled order, endpoints included,
			// each shifted off round numbers by an odd base-unit tail.
			span := new(big.Int).Sub(quoteRange.MaxAmount, quoteRange.MinAmount)
			for sample := range int64(3_001) {
				amount := new(big.Int).Mul(span, big.NewInt(sample*7_919%3_001))
				amount.Div(amount, big.NewInt(3_000)).Add(amount, quoteRange.MinAmount)
				if tail := big.NewInt(sample * 104_729 % 1_000_003); new(big.Int).Add(amount, tail).Cmp(quoteRange.MaxAmount) <= 0 {
					amount.Add(amount, tail)
				}
				actual, solveErr := liquidgreedy.SolveQuote(liquidstrategies.QuoteTask{
					ExactInput: amount, Candidates: candidates, MaxRoutes: routes,
					MinInput: strategy.minAmount, InputPolicy: liquidstrategies.RejectUncoveredInput,
				})
				if solveErr != nil || actual == nil {
					t.Fatalf("SolveQuote(%s) = %+v, %v", amount, actual, solveErr)
				}
				quoted := liquidlane.AmountOutForRate(amount, rate, 18, 6)
				if quoted.Cmp(actual.AmountOut) > 0 {
					t.Fatalf("%s: amount %s quoted %s above executable %s in range %+v",
						name, amount, quoted, actual.AmountOut, quoteRange)
				}
			}
		}
	}
}
