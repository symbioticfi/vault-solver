package rfq

import (
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestDiscountQuoteUsesBackendSnapshotWithoutRPC(t *testing.T) {
	for _, precision := range []int64{1_000_000, 100_000_000} {
		for _, reserved := range []int64{0, 8_000_000, 9_000_000} {
			t.Run(big.NewInt(precision).String()+"/reserved"+big.NewInt(reserved).String(), func(t *testing.T) {
				s := testServer()
				s.quotes.discountsEnabled = true
				// Any chain-reader access panics and makes the HTTP assertion fail.
				s.quotes.reader = nil
				body := validQuoteBody()
				id := common.HexToHash("0x1234").Hex()
				decimals := 18
				entry := &body.Adapters[0]
				entry.Account = vlt.Hex()
				entry.TokenInDecimals = &decimals
				entry.Price = "1234567890123456789"
				entry.DiscountID = &id
				entry.Discount = big.NewInt(precision / 20).String()
				entry.BlockNumber = "42"
				if precision == 100_000_000 {
					s.quotes.recipeExecutor = common.HexToAddress("0x43")
					entry.DiscountPrecision = "100000000"
				}
				s.quotes.reservations = func(_ string, rows []solverInventory) liquidlane.CapacityReservations {
					if len(rows) != 1 || rows[0].Vault != vlt || rows[0].BlockNumber != 42 {
						t.Fatal("lost backend account or observation block")
					}
					return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, vlt, tOut): big.NewInt(reserved)}
				}
				rr := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
				if reserved == 9_000_000 {
					if rr.Code != http.StatusNoContent {
						t.Fatalf("overcommitted discounted quote: %d %s", rr.Code, rr.Body.String())
					}
					return
				}
				if rr.Code != http.StatusOK {
					t.Fatalf("quote status=%d: %s", rr.Code, rr.Body.String())
				}
				var response quoteResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.AmountOut != "1172838" {
					t.Fatalf("quote output=%s, want the exact two-step payout 1172838", response.AmountOut)
				}
			})
		}
	}
}

func TestDiscountQuoteRequiresBackendPricing(t *testing.T) {
	id := common.HexToHash("0x1234").Hex()
	decimals := 18
	base := quoteAdapter{Adapter: vlt.Hex(), Asset: tOut.Hex(), AssetDecimals: 6, MaxAssets: "100", BlockNumber: "42", Discount: "1", DiscountID: &id, Account: vlt.Hex(), Price: "1000000000000000000", TokenInDecimals: &decimals}
	for name, change := range map[string]func(*quoteAdapter){
		"missing price":    func(v *quoteAdapter) { v.Price = "" },
		"zero price":       func(v *quoteAdapter) { v.Price = "0" },
		"overflow price":   func(v *quoteAdapter) { v.Price = new(big.Int).Lsh(big.NewInt(1), 256).String() },
		"missing account":  func(v *quoteAdapter) { v.Account = "" },
		"missing decimals": func(v *quoteAdapter) { v.TokenInDecimals = nil },
	} {
		t.Run(name, func(t *testing.T) {
			entry := base
			change(&entry)
			if _, err := entry.parse(0, 1, tIn, common.Address{}); err == nil || !strings.Contains(err.Error(), "discounted quote") {
				t.Fatalf("invalid pricing accepted: %v", err)
			}
		})
	}
}
