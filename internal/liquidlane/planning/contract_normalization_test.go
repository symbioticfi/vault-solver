package planning

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
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
