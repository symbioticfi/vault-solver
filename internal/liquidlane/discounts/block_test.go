package discounts

import (
	"strings"
	"testing"
)

func TestParseOfferBlockNumber(t *testing.T) {
	base := ListItem{
		DiscountID: "0x00000000000000000000000000000000000000000000000000000000000000ab",
		Adapter:    "0x0000000000000000000000000000000000000003", TokenToRedeem: "0x0000000000000000000000000000000000000001",
		Collateral: "0x0000000000000000000000000000000000000002", CollateralDecimals: 6,
		Discount: "500", Deadline: 4_102_444_800, MaxRate: "1000000000000000000", MaxAssets: "10000000",
	}
	for _, tc := range []struct {
		name    string
		block   string
		want    uint64
		wantErr string
	}{
		{name: "reported", block: "123", want: 123},
		{name: "not reported", block: ""},
		{name: "malformed", block: "0x7b", wantErr: "blockNumber"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := base
			item.BlockNumber = tc.block
			offer, err := ParseOffer(item)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if offer.BlockNumber != tc.want {
				t.Fatalf("BlockNumber = %d, want %d", offer.BlockNumber, tc.want)
			}
		})
	}
}
