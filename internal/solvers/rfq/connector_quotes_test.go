package rfq

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestConnectorQuoteWireRequiresExplicitScaleAndSignedTerms(t *testing.T) {
	id := "0x1111111111111111111111111111111111111111111111111111111111111111"
	base := quoteAdapter{Kind: "connector", DiscountPrecision: "100000000", Adapter: "0x1111111111111111111111111111111111111111", Asset: "0x2222222222222222222222222222222222222222", MaxAssets: "73", Discount: "1000001", DiscountID: &id, AssetDecimals: 6}
	parsed, err := base.parse(0, 1, common.HexToAddress("0x3333333333333333333333333333333333333333"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Discount.String() != "1000001" || parsed.DiscountPrecision != 100_000_000 || parsed.Kind != "connector" {
		t.Fatalf("lost connector terms: %+v", parsed)
	}
	for _, change := range []func(*quoteAdapter){
		func(v *quoteAdapter) { v.DiscountPrecision = "1000000" },
		func(v *quoteAdapter) { v.DiscountID = nil },
		func(v *quoteAdapter) { v.Kind = "unknown" },
		func(v *quoteAdapter) { v.Discount = "100000000" },
	} {
		bad := base
		change(&bad)
		if _, err := bad.parse(0, 1, common.Address{}); err == nil {
			t.Fatalf("accepted malformed connector %+v", bad)
		}
	}
}
