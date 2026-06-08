package rfq

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

var (
	tIn  = common.HexToAddress("0x0000000000000000000000000000000000000001")
	tOut = common.HexToAddress("0x0000000000000000000000000000000000000002")
	vlt  = common.HexToAddress("0x0000000000000000000000000000000000000003")
)

// baseReq: swap 1e18 of an 18-decimal tokenIn for a 6-decimal asset (tokenOut).
func baseReq(t *testing.T) strategyRequest {
	t.Helper()
	return strategyRequest{RequestID: "r", QuoteID: "q", TokenIn: tIn, TokenOut: tOut, Amount: mustBig(t, "1000000000000000000")}
}

func TestSelectBestStrategy_DirectFill(t *testing.T) {
	req := baseReq(t)
	inv := []solverInventory{{
		Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"),            // 10 USDC of liquidity
		MaxRate:   mustBig(t, "1000000000000000000"), // 1e18 ≥ our private rate
	}}
	oracle := map[common.Address]*big.Int{tOut: mustBig(t, "1000000")} // oracle: 1e18 tokenIn → 1.000000 USDC

	got := selectBestStrategy(req, inv, 18, 1000, oracle, time.Unix(0, 0)) // 10% discount
	if got == nil {
		t.Fatal("expected a strategy")
	}
	// 1.0 USDC oracle, minus 10% discount = 0.9 USDC = 900000 base units.
	if got.QuotedAmountOut.String() != "900000" {
		t.Fatalf("quotedAmountOut = %s, want 900000", got.QuotedAmountOut)
	}
	if len(got.Legs) != 1 {
		t.Fatalf("legs = %d, want 1", len(got.Legs))
	}
	if got.Legs[0].AmountIn.String() != "1000000000000000000" || got.Legs[0].AmountOut.String() != "900000" {
		t.Fatalf("leg = (in %s, out %s), want (1e18, 900000)", got.Legs[0].AmountIn, got.Legs[0].AmountOut)
	}
	if got.Legs[0].DiscountID != nil {
		t.Fatalf("direct leg should have nil discountId")
	}
}

func TestSelectBestStrategy_RejectsMaxRateBelowPrivateRate(t *testing.T) {
	req := baseReq(t)
	inv := []solverInventory{{
		Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"),
		MaxRate:   mustBig(t, "800000000000000000"), // 0.8e18 < private rate (0.9e18) → ineligible
	}}
	oracle := map[common.Address]*big.Int{tOut: mustBig(t, "1000000")}

	if got := selectBestStrategy(req, inv, 18, 1000, oracle, time.Unix(0, 0)); got != nil {
		t.Fatalf("expected nil (vault won't honor the rate), got %v", got)
	}
}

func TestSelectBestStrategy_AssetMustEqualTokenOut(t *testing.T) {
	req := baseReq(t)
	other := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	inv := []solverInventory{{
		Adapter: vlt, Asset: other, AssetDecimals: 6, // asset != tokenOut
		MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
	}}
	oracle := map[common.Address]*big.Int{other: mustBig(t, "1000000")}

	if got := selectBestStrategy(req, inv, 18, 1000, oracle, time.Unix(0, 0)); got != nil {
		t.Fatalf("expected nil (asset != tokenOut), got %v", got)
	}
}

func TestSelectBestStrategy_NoOraclePriceSkips(t *testing.T) {
	req := baseReq(t)
	inv := []solverInventory{{
		Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
	}}
	// Empty oracle map → collateral can't be priced → no strategy.
	if got := selectBestStrategy(req, inv, 18, 1000, map[common.Address]*big.Int{}, time.Unix(0, 0)); got != nil {
		t.Fatalf("expected nil (no oracle price), got %v", got)
	}
}

func TestSelectBestStrategy_ZeroDiscountUsesOracleRate(t *testing.T) {
	req := baseReq(t)
	inv := []solverInventory{{
		Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets: mustBig(t, "10000000"), MaxRate: mustBig(t, "1000000000000000000"),
	}}
	oracle := map[common.Address]*big.Int{tOut: mustBig(t, "1000000")}

	got := selectBestStrategy(req, inv, 18, 0, oracle, time.Unix(0, 0)) // no discount
	if got == nil || got.QuotedAmountOut.String() != "1000000" {
		t.Fatalf("want full 1000000 with zero discount, got %v", got)
	}
}

func TestSelectBestStrategy_DiscountLegUsesMaxRate(t *testing.T) {
	req := baseReq(t)
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	inv := []solverInventory{{
		Adapter: vlt, Asset: tOut, AssetDecimals: 6,
		MaxAssets:  mustBig(t, "10000000"),
		MaxRate:    mustBig(t, "1000000000000000000"), // 1.0 — the vault's advertised discount rate
		DiscountID: &h,
	}}
	// Oracle is deliberately lower than the discount rate; a discount leg must price off maxRate, not it.
	oracle := map[common.Address]*big.Int{tOut: mustBig(t, "500000")}

	got := selectBestStrategy(req, inv, 18, 1000, oracle, time.Unix(0, 0))
	if got == nil || len(got.Legs) != 1 || got.Legs[0].DiscountID == nil {
		t.Fatalf("expected one discount leg, got %v", got)
	}
	if got.QuotedAmountOut.String() != "1000000" { // 1e18 * 1.0 → 1.000000 collateral, ignoring the oracle
		t.Fatalf("quotedAmountOut = %s, want 1000000 (maxRate, not discounted oracle)", got.QuotedAmountOut)
	}
}

func TestApplyQuoteDiscount(t *testing.T) {
	cases := []struct {
		in   string
		bps  uint64
		want string
	}{
		{"1000000", 0, "1000000"},   // no discount
		{"1000000", 1000, "900000"}, // 10%
		{"1000000", 10000, "0"},     // 100%
		{"1000000", 250, "975000"},  // 2.5%
	}
	for _, c := range cases {
		got := applyQuoteDiscount(mustBig(t, c.in), c.bps).String()
		if got != c.want {
			t.Fatalf("applyQuoteDiscount(%s, %d) = %s, want %s", c.in, c.bps, got, c.want)
		}
	}
}
