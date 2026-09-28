package greedy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

func TestNormalizeOracleInventoryPricesEachPhysicalRoute(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1")
	tokenOut := common.HexToAddress("0x2")
	first := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"), tokenIn, tokenOut, 18, 6)
	second := liquidlane.NewRoute(1, common.HexToAddress("0xb"), common.HexToAddress("0x20"), tokenIn, tokenOut, 18, 6)
	amountIn := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	sources := []liquidlane.Inventory{
		liquidlane.DirectInventory(first, big.NewInt(2_000_000), big.NewInt(900_000_000_000_000_000)),
		liquidlane.DirectInventory(second, big.NewInt(2_000_000), big.NewInt(800_000_000_000_000_000)),
	}
	physical := []liquidlane.FillQuote{
		{Inventory: liquidlane.DirectInventory(first, big.NewInt(1_500_000), nil), AmountIn: amountIn, MaxAmountOut: big.NewInt(900_000)},
		{Inventory: liquidlane.DirectInventory(second, big.NewInt(2_000_000), nil), AmountIn: amountIn, MaxAmountOut: big.NewInt(800_000)},
	}

	got := NormalizeOracleInventory(amountIn, sources, physical)
	if len(got) != 2 || got[0].Rate.String() != "900000000000000000" || got[0].MaxAmountOut.String() != "1500000" ||
		got[1].Rate.String() != "800000000000000000" {
		t.Fatalf("normalized = %#v", got)
	}
}

func TestNormalizeOracleInventoryUsesSignedRateForPrivateAlternative(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1")
	tokenOut := common.HexToAddress("0x2")
	route := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"), tokenIn, tokenOut, 18, 6)
	discountID := common.HexToHash("0xd")
	amountIn := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	sources := []liquidlane.Inventory{
		liquidlane.DiscountInventory(
			route,
			big.NewInt(1_000_000),
			big.NewInt(750_000_000_000_000_000),
			discountID,
			time.Time{},
		),
	}
	physical := []liquidlane.FillQuote{{
		Inventory: liquidlane.DirectInventory(route, big.NewInt(1_000_000), nil),
		AmountIn:  amountIn, MaxAmountOut: big.NewInt(800_000),
	}}

	got := NormalizeOracleInventory(amountIn, sources, physical)
	// The advertised 0.75 rate wins over the physical route's 0.8, but is re-derived conservatively:
	// the backend pre-applies and floors the discount while the adapter floors getAmountOut first, so
	// the candidate prices one output unit (1e12 rate units at 18→6 decimals) below the advertised rate.
	if len(got) != 1 || got[0].Rate.String() != "749999000000000000" || got[0].DiscountID == nil {
		t.Fatalf("normalized = %#v", got)
	}
	if out := liquidlane.AmountOutForRate(amountIn, got[0].Rate, 18, 6); out.String() != "749999" {
		t.Fatalf("amountOut at normalized rate = %s, want one unit below the advertised 750000", out)
	}
}

// A discount candidate whose advertised rate would over-predict must be normalized to a rate that
// never prices above what the adapter pays for the same amountIn.
func TestNormalizeOracleInventoryKeepsDiscountRateBelowAdapter(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1")
	tokenOut := common.HexToAddress("0x2")
	route := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"), tokenIn, tokenOut, 18, 18)
	discountID := common.HexToHash("0xd")
	amountIn, _ := new(big.Int).SetString("1000000000000000", 10)
	price, _ := new(big.Int).SetString("1034567891234567890", 10)

	// maxRate as the backend derives it: price with a 1 ppm discount applied, floored.
	advertised := new(big.Int).Mul(price, big.NewInt(liquidlane.DiscountPrecision-1))
	advertised.Div(advertised, big.NewInt(liquidlane.DiscountPrecision))
	// What the adapter actually pays: floor getAmountOut first, then apply the same discount.
	adapterOut := liquidlane.AmountOutAfterDiscount(
		liquidlane.AmountOutForRate(amountIn, price, 18, 18), big.NewInt(1),
	)
	if raw := liquidlane.AmountOutForRate(amountIn, advertised, 18, 18); raw.Cmp(adapterOut) <= 0 {
		t.Fatalf("fixture no longer reproduces the over-prediction: raw %s, adapter %s", raw, adapterOut)
	}

	sources := []liquidlane.Inventory{
		liquidlane.DiscountInventory(route, big.NewInt(1_000_000_000_000_000_000), advertised, discountID, time.Time{}),
	}
	physical := []liquidlane.FillQuote{{
		Inventory: liquidlane.DirectInventory(route, big.NewInt(1_000_000_000_000_000_000), nil),
		AmountIn:  amountIn, MaxAmountOut: big.NewInt(1),
	}}

	got := NormalizeOracleInventory(amountIn, sources, physical)
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want one", len(got))
	}
	if out := liquidlane.AmountOutForRate(amountIn, got[0].Rate, 18, 18); out.Cmp(adapterOut) > 0 {
		t.Fatalf("normalized rate prices %s, above the adapter's %s", out, adapterOut)
	}
}

// With the oracle price, candidates price like the adapter: private legs at their signed discount,
// direct legs at the adapter minimum. Without it they keep the fixed-rate model.
func TestNewQuoteCandidatePricesWithOraclePrice(t *testing.T) {
	t.Parallel()
	route := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"),
		common.HexToAddress("0x1"), common.HexToAddress("0x2"), 18, 6)
	price := big.NewInt(1_128_372_140_000_000_000) // mHYPER's 8-decimal oracle price on mainnet
	capacity := big.NewInt(1_174_510_000)
	private := liquidlane.DiscountInventory(route, capacity, big.NewInt(1_128_146_000_000_000_000),
		common.HexToHash("0xd"), time.Time{})
	private.Price, private.Discount = price, big.NewInt(200)
	direct := liquidlane.DirectInventory(route, capacity, liquidlane.DiscountedRate(price, big.NewInt(0)))
	direct.Price, direct.AdapterMinDiscount = price, big.NewInt(0)

	for name, item := range map[string]liquidlane.Inventory{"private": private, "direct": direct} {
		candidate := NewQuoteCandidate(item, capacity)
		discount := item.PayoutDiscount()
		if candidate == nil || !candidate.ExactPricing() || candidate.Discount.Cmp(discount) != 0 ||
			candidate.Rate.Cmp(liquidlane.DiscountedRate(price, discount)) != 0 {
			t.Fatalf("%s candidate = %+v", name, candidate)
		}
		payoutAt := func(amountIn *big.Int) *big.Int {
			return liquidlane.DiscountedAmountOut(amountIn, price, discount, 18, 6)
		}
		// MaxAmountIn is the largest input whose payout still fits the capacity.
		if payoutAt(candidate.MaxAmountIn).Cmp(capacity) > 0 ||
			payoutAt(new(big.Int).Add(candidate.MaxAmountIn, big.NewInt(1))).Cmp(capacity) <= 0 {
			t.Fatalf("%s max input %s does not bound payout by capacity %s", name, candidate.MaxAmountIn, capacity)
		}
	}

	private.Price = nil
	if candidate := NewQuoteCandidate(private, capacity); candidate == nil || candidate.ExactPricing() ||
		candidate.Rate.Cmp(private.MaxRate) != 0 {
		t.Fatalf("unpriced candidate = %+v, want the fixed-rate model", candidate)
	}
}

// Quotes from exactly priced candidates equal what the adapter pays, in both directions.
func TestSolveQuoteWithOraclePriceMatchesAdapterPayout(t *testing.T) {
	t.Parallel()
	route := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"),
		common.HexToAddress("0x1"), common.HexToAddress("0x2"), 18, 6)
	price := big.NewInt(1_128_372_140_000_000_000)
	discount := big.NewInt(200)
	item := liquidlane.DiscountInventory(route, big.NewInt(10_000_000_000), liquidlane.DiscountedRate(price, discount),
		common.HexToHash("0xd"), time.Time{})
	item.Price, item.Discount = price, discount
	candidates := []liquidlane.QuoteCandidate{*NewQuoteCandidate(item, item.MaxAssets)}

	for _, raw := range []string{"1000000000000000000000", "123456789012345678901", "7123456789012345678", "1"} {
		amountIn, _ := new(big.Int).SetString(raw, 10)
		payout := liquidlane.DiscountedAmountOut(amountIn, price, discount, 18, 6)
		exactIn, err := SolveQuote(strategies.QuoteTask{
			ExactInput: amountIn, Candidates: candidates, MaxRoutes: 1, InputPolicy: strategies.RejectUncoveredInput,
		})
		if payout.Sign() == 0 {
			if err != nil || exactIn != nil {
				t.Fatalf("dust %s: quote = %+v, err %v", raw, exactIn, err)
			}
			continue
		}
		if err != nil || exactIn == nil || exactIn.AmountOut.Cmp(payout) != 0 {
			t.Fatalf("exact input %s: quote = %+v, err %v; want the adapter payout %s", raw, exactIn, err, payout)
		}
		exactOut, err := SolveQuote(strategies.QuoteTask{ExactOutput: payout, Candidates: candidates, MaxRoutes: 1})
		minimum := liquidlane.MinAmountInForDiscountedAmountOut(payout, price, discount, 18, 6)
		if err != nil || exactOut == nil || exactOut.AmountIn.Cmp(minimum) != 0 ||
			liquidlane.DiscountedAmountOut(exactOut.AmountIn, price, discount, 18, 6).Cmp(payout) < 0 {
			t.Fatalf("exact output %s: quote = %+v, err %v; want minimal input %s", payout, exactOut, err, minimum)
		}
	}
}

// RFQ inventory that carries the signed discount prices off the adapter quote at the order amount,
// not the backend's per-token estimate, and drops a discount the adapter would now reject.
func TestNormalizeOracleInventoryPricesSignedDiscountFromAdapterQuote(t *testing.T) {
	t.Parallel()
	route := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"),
		common.HexToAddress("0x1"), common.HexToAddress("0x2"), 18, 6)
	amountIn, _ := new(big.Int).SetString("1000000000000000000000", 10)
	source := liquidlane.DiscountInventory(route, big.NewInt(2_000_000_000),
		big.NewInt(1_128_146_000_000_000_000), common.HexToHash("0xd"), time.Time{})
	source.Discount = big.NewInt(200)
	gross := big.NewInt(1_128_372_140) // mainnet getAmountOut for 1000 mHYPER
	physical := liquidlane.FillQuote{
		Inventory: liquidlane.DirectInventory(route, big.NewInt(2_000_000_000), nil),
		AmountIn:  amountIn, GrossAmountOut: gross, MaxAmountOut: gross, MinDiscount: new(big.Int),
	}

	got := NormalizeOracleInventory(amountIn, []liquidlane.Inventory{source}, []liquidlane.FillQuote{physical})
	payout := liquidlane.AmountOutAfterDiscount(gross, source.Discount) // 1128146465, as executed on the fork
	if len(got) != 1 || got[0].ExactPricing() {
		t.Fatalf("normalized = %+v", got)
	}
	out := got[0].AmountOutFor(amountIn)
	if out.Cmp(payout) > 0 || new(big.Int).Sub(payout, out).Cmp(big.NewInt(1)) > 0 {
		t.Fatalf("candidate prices %s, want the adapter payout %s within one unit", out, payout)
	}

	physical.MinDiscount = big.NewInt(300)
	if rejected := NormalizeOracleInventory(
		amountIn, []liquidlane.Inventory{source}, []liquidlane.FillQuote{physical},
	); len(rejected) != 0 {
		t.Fatalf("discount below the adapter minimum survived: %+v", rejected)
	}
}
