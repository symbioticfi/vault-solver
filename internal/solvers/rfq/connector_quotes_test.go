package rfq

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestConnectorQuoteWireRequiresExplicitScaleAndSignedTerms(t *testing.T) {
	id := "0x1111111111111111111111111111111111111111111111111111111111111111"
	base := quoteAdapter{DiscountPrecision: "100000000", Adapter: "0x1111111111111111111111111111111111111111", Asset: "0x2222222222222222222222222222222222222222", MaxAssets: "73", Discount: "1000001", DiscountID: &id, AssetDecimals: 6}
	executor := common.HexToAddress("0x4444444444444444444444444444444444444444")
	parsed, err := base.parse(0, 1, common.HexToAddress("0x3333333333333333333333333333333333333333"), executor)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Discount.String() != "1000001" || parsed.DiscountPrecision != 100_000_000 || parsed.RecipeExecutor != executor {
		t.Fatalf("lost connector terms: %+v", parsed)
	}
	for _, change := range []func(*quoteAdapter){
		func(v *quoteAdapter) { v.DiscountPrecision = "1000000" },
		func(v *quoteAdapter) { v.DiscountID = nil },
		func(v *quoteAdapter) { v.Discount = "100000000" },
	} {
		bad := base
		change(&bad)
		if _, err := bad.parse(0, 1, common.Address{}, executor); err == nil {
			t.Fatalf("accepted malformed connector %+v", bad)
		}
	}
}

func TestAdapterModeRejectsConnectorInventory(t *testing.T) {
	id := "0x1111111111111111111111111111111111111111111111111111111111111111"
	entry := quoteAdapter{DiscountPrecision: "100000000", Adapter: common.HexToAddress("0x42").Hex(), Asset: common.HexToAddress("0x32").Hex(), MaxAssets: "100", Discount: "1000001", DiscountID: &id}
	if _, err := entry.parse(0, 1, common.HexToAddress("0x31"), common.Address{}); err == nil {
		t.Fatal("native solver accepted connector precision")
	}
}
