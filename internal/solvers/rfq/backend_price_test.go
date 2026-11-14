package rfq

import (
	"encoding/json"
	"math/big"
	"net/http"
	"reflect"
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
					s.quotes.recipeExecutors = []common.Address{common.HexToAddress("0x43")}
					entry.DiscountPrecision = "100000000"
					entry.RecipeExecutor = common.HexToAddress("0x43").Hex()
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

func TestConnectorQuoteResolvesLocalIdentitiesWithoutRPC(t *testing.T) {
	executor := common.HexToAddress("0x43")
	for _, tc := range []struct {
		name, account, hint string
		available           bool
		reserved            int64
		want                int
	}{
		{"derive both", "", "", true, 0, http.StatusOK},
		{"validate supplied identities", vlt.Hex(), executor.Hex(), true, 0, http.StatusOK},
		{"conflicting account", tIn.Hex(), "", true, 0, http.StatusNoContent},
		{"unconfigured hint", "", tIn.Hex(), true, 0, http.StatusNoContent},
		{"missing or ambiguous cached route", "", "", false, 0, http.StatusNoContent},
		{"shared account reservation", "", "", true, 10_000_000, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer()
			s.quotes.reader = nil // Any quote-time RPC access fails the test.
			s.quotes.recipeExecutors = []common.Address{executor}
			s.quotes.discountsEnabled = true
			s.quotes.resolveRoute = func(hint, venue, rwa, asset common.Address) (common.Address, common.Address, bool) {
				ok := tc.available && (hint == (common.Address{}) || hint == executor) && venue == vlt && rwa == tIn && asset == tOut
				return executor, vlt, ok
			}
			body := validQuoteBody()
			id := common.HexToHash("0x1234").Hex()
			entry := &body.Adapters[0]
			entry.Account, entry.RecipeExecutor = tc.account, tc.hint
			entry.DiscountID, entry.DiscountPrecision = &id, "100000000"
			original := body
			original.Adapters = append([]quoteAdapter(nil), body.Adapters...)
			s.quotes.reservations = func(_ string, rows []solverInventory) liquidlane.CapacityReservations {
				if len(rows) != 1 || rows[0].Vault != vlt || rows[0].RecipeExecutor != executor || rows[0].BlockNumber != 1 {
					t.Fatal("lost cached identities or backend observation block")
				}
				return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, vlt, tOut): big.NewInt(tc.reserved)}
			}
			decision, err := s.quotes.quote(t.Context(), &body)
			if err != nil || (decision.response != nil) != (tc.want == http.StatusOK) {
				t.Fatalf("quote=%+v err=%v", decision, err)
			}
			if !reflect.DeepEqual(body, original) {
				t.Fatal("cache resolution modified the caller's payload")
			}
			rr := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
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

func TestOverlappingRecipeExecutorsQuoteOneSharedBalance(t *testing.T) {
	for _, maximum := range []string{"1000000", "2000000"} {
		t.Run(maximum, func(t *testing.T) {
			s := testServer()
			s.quotes.recipeExecutors = []common.Address{common.HexToAddress("0x43"), common.HexToAddress("0x44")}
			s.quotes.discountsEnabled = true
			s.quotes.reader = nil
			body := validQuoteBody()
			entry := &body.Adapters[0]
			id, decimals := common.HexToHash("0x1234").Hex(), 18
			entry.Account, entry.TokenInDecimals, entry.Price = vlt.Hex(), &decimals, "1234567890123456789"
			entry.DiscountID, entry.Discount, entry.DiscountPrecision = &id, "5000000", "100000000"
			entry.RecipeExecutor, entry.MaxAssets = s.quotes.recipeExecutors[0].Hex(), "1000000"
			other := *entry
			other.RecipeExecutor, other.MaxAssets = s.quotes.recipeExecutors[1].Hex(), maximum
			body.Adapters = append(body.Adapters, other)
			rr := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
			want := http.StatusNoContent
			if maximum == "2000000" {
				want = http.StatusOK
			}
			if rr.Code != want {
				t.Fatalf("overlapping capacity: status=%d want=%d body=%s", rr.Code, want, rr.Body.String())
			}
		})
	}
}
