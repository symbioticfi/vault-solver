package discounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListDiscountsRequiresCapacityObservationBlock(t *testing.T) {
	for _, tc := range []struct {
		name    string
		block   any
		want    uint64
		wantErr bool
	}{
		{name: "reported", block: "123", want: 123},
		{name: "genesis", block: "0"},
		{name: "overflow", block: "18446744073709551616", wantErr: true},
		{name: "missing", wantErr: true},
		{name: "empty", block: "", wantErr: true},
		{name: "malformed", block: "0x7b", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				item := map[string]any{
					"discountId":    "0x" + hash64,
					"adapter":       "0x0000000000000000000000000000000000000abc",
					"tokenToRedeem": "0x0000000000000000000000000000000000000def",
					"collateral":    "0x0000000000000000000000000000000000000c01", "collateralDecimals": 6,
					"discount": "10", "signer": "0x0000000000000000000000000000000000000aaa",
					"deadline": 1900000000, "maxAssets": "5000",
				}
				if tc.block != nil {
					item["blockNumber"] = tc.block
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"protocol": "0x0000000000000000000000000000000000000bbb", "discounts": []any{item}})
			}))
			t.Cleanup(srv.Close)
			list, err := NewClient(srv.URL).ListDiscounts(t.Context())
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "blockNumber") {
					t.Fatalf("err = %v, want blockNumber error", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(list.Discounts) != 1 || list.Discounts[0].BlockNumber != tc.want {
				t.Fatalf("discounts = %+v, want block %d", list.Discounts, tc.want)
			}
		})
	}
}
