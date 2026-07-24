package greedy

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
	if len(got) != 1 || got[0].Rate.String() != "750000000000000000" || got[0].DiscountID == nil {
		t.Fatalf("normalized = %#v", got)
	}
}
