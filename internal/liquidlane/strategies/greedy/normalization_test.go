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
	third := liquidlane.NewRoute(1, common.HexToAddress("0xc"), common.HexToAddress("0x30"), tokenIn, tokenOut, 18, 6)
	amountIn := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	direct := func(route liquidlane.Route, capacity, minDiscount int64) liquidlane.Inventory {
		item := liquidlane.DirectInventory(route, big.NewInt(capacity), nil)
		item.AdapterMinDiscount = big.NewInt(minDiscount)
		return item
	}
	quote := func(route liquidlane.Route, capacity, gross int64) liquidlane.FillQuote {
		return liquidlane.FillQuote{
			Inventory: liquidlane.DirectInventory(route, big.NewInt(capacity), nil), AmountIn: amountIn,
			GrossAmountOut: big.NewInt(gross), MaxAmountOut: big.NewInt(gross), MinDiscount: new(big.Int),
		}
	}
	sources := []liquidlane.Inventory{
		direct(first, 2_000_000, 0), direct(second, 2_000_000, 0),
		// The inventory was offered at a 0.1% minimum the chain has since lowered: price at 0.1%.
		direct(third, 2_000_000, 1_000),
	}
	physical := []liquidlane.FillQuote{
		quote(first, 1_500_000, 900_000), quote(second, 2_000_000, 800_000), quote(third, 2_000_000, 900_000),
	}

	got := NormalizeOracleInventory(amountIn, sources, physical)
	if len(got) != 3 || got[0].Rate.String() != "900000000000000000" || got[0].MaxAmountOut.String() != "1500000" ||
		got[1].Rate.String() != "800000000000000000" || got[2].Rate.String() != "899100000000000000" {
		t.Fatalf("normalized = %#v", got)
	}
}

// A discount leg priced from the adapter quote at the order amount never predicts more than the
// adapter pays and gives up at most one unit, across prices, decimals, discounts and sizes.
func TestNormalizeOracleInventoryKeepsDiscountLegsWithinOneUnitOfPayout(t *testing.T) {
	t.Parallel()
	discountID := common.HexToHash("0xd")
	for _, dec := range [][2]int{{18, 6}, {6, 6}, {18, 18}} {
		route := liquidlane.NewRoute(1, common.HexToAddress("0xa"), common.HexToAddress("0x10"),
			common.HexToAddress("0x1"), common.HexToAddress("0x2"), dec[0], dec[1])
		for _, rawPrice := range []int64{112_837_214, 106_177_400, 111_727_500, 100_000_000} {
			price := new(big.Int).Mul(big.NewInt(rawPrice), big.NewInt(10_000_000_000))
			for _, discount := range []int64{0, 1, 200, 40_000} {
				for step := int64(1); step <= 30; step++ {
					amountIn := new(big.Int).Mul(big.NewInt(step*step*7_919+step), pow10ForTest(dec[0]-2))
					gross := liquidlane.AmountOutForRate(amountIn, price, dec[0], dec[1])
					payout := liquidlane.AmountOutAfterDiscount(gross, big.NewInt(discount))
					source := liquidlane.DiscountInventory(route, maxUint256ForTest(), nil, discountID, time.Time{})
					source.Discount = big.NewInt(discount)
					physical := liquidlane.FillQuote{
						Inventory: liquidlane.DirectInventory(route, maxUint256ForTest(), nil), AmountIn: amountIn,
						GrossAmountOut: gross, MaxAmountOut: gross, MinDiscount: new(big.Int),
					}
					got := NormalizeOracleInventory(amountIn, []liquidlane.Inventory{source}, []liquidlane.FillQuote{physical})
					if payout.Sign() == 0 {
						continue
					}
					if len(got) != 1 {
						t.Fatalf("amountIn %s: candidates = %d, want one", amountIn, len(got))
					}
					out := got[0].AmountOutFor(amountIn)
					if out.Cmp(payout) > 0 || new(big.Int).Sub(payout, out).Cmp(big.NewInt(1)) > 0 {
						t.Fatalf("amountIn %s: candidate prices %s, adapter pays %s", amountIn, out, payout)
					}
				}
			}
		}
	}
}

func pow10ForTest(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(exponent, 0))), nil)
}

func maxUint256ForTest() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
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
