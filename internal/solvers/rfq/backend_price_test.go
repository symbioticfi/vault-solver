package rfq

import (
	"encoding/json"
	"math/big"
	"net/http"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestQuoteUsesBackendSnapshotWithoutRPC(t *testing.T) {
	for _, precision := range []int64{0, 1_000_000, 100_000_000} {
		for _, reserved := range []int64{0, 8_000_000, 9_000_000, 10_000_000} {
			t.Run(big.NewInt(precision).String()+"/reserved"+big.NewInt(reserved).String(), func(t *testing.T) {
				s := testServer()
				s.quotes.discountsEnabled = true
				// The quote service has no RPC reader; all candidate data comes from this snapshot.
				body := validQuoteBody()
				id := common.HexToHash("0x1234").Hex()
				decimals := 18
				entry := &body.Adapters[0]
				entry.TokenInDecimals = &decimals
				entry.Price = "1234567890123456789"
				if precision != 0 {
					entry.DiscountID = &id
				}
				entry.Discount = "50000"
				if precision == 100_000_000 {
					entry.Discount = "5000000"
				}
				entry.BlockNumber = "42"
				if precision == 100_000_000 {
					entry.DiscountPrecision = "100000000"
					s.quotes.resolveRoute = func(_, _, _ common.Address, _ int) (common.Address, common.Address, bool) {
						return common.HexToAddress("0x43"), vlt, true
					}
				}
				s.quotes.reservations = func(_ string, rows []solverInventory) liquidlane.CapacityReservations {
					if len(rows) != 1 || rows[0].Vault != vlt || rows[0].BlockNumber != 42 {
						t.Fatal("lost cached account or backend observation block")
					}
					return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, vlt, tOut): big.NewInt(reserved)}
				}
				rr := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
				if reserved == 10_000_000 || (reserved == 9_000_000 && precision != 0) {
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
				want := "1172838" // Exact two-step floor of the discounted oracle payout.
				if reserved == 9_000_000 {
					want = "1000000" // Native direct routes can absorb uncovered input.
				}
				if response.AmountOut != want {
					t.Fatalf("quote output=%s, want %s", response.AmountOut, want)
				}
			})
		}
	}
}

func TestConnectorQuoteResolvesLocalIdentitiesWithoutRPC(t *testing.T) {
	executor := common.HexToAddress("0x43")
	for _, tc := range []struct {
		name      string
		available bool
		reserved  int64
		want      int
	}{
		{"derive both", true, 0, http.StatusOK},
		{"missing or ambiguous cached route", false, 0, http.StatusNoContent},
		{"shared account reservation", true, 10_000_000, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer()
			s.quotes.discountsEnabled = true
			s.quotes.resolveRoute = func(venue, rwa, asset common.Address, _ int) (common.Address, common.Address, bool) {
				ok := tc.available && venue == vlt && rwa == tIn && asset == tOut
				return executor, vlt, ok
			}
			body := validQuoteBody()
			id := common.HexToHash("0x1234").Hex()
			entry := &body.Adapters[0]
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

func TestRepeatedConnectorInventoryQuotesOneSharedBalance(t *testing.T) {
	for _, maximum := range []string{"1000000", "2000000"} {
		t.Run(maximum, func(t *testing.T) {
			s := testServer()
			s.quotes.resolveRoute = func(_, _, _ common.Address, _ int) (common.Address, common.Address, bool) {
				return common.HexToAddress("0x43"), vlt, true
			}
			s.quotes.discountsEnabled = true
			body := validQuoteBody()
			entry := &body.Adapters[0]
			id, decimals := common.HexToHash("0x1234").Hex(), 18
			entry.TokenInDecimals, entry.Price = &decimals, "1234567890123456789"
			entry.DiscountID, entry.Discount, entry.DiscountPrecision = &id, "5000000", "100000000"
			entry.MaxAssets = "1000000"
			other := *entry
			other.MaxAssets = maximum
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

func TestUncachedNativeQuoteDeclinesWithoutRPC(t *testing.T) {
	s := testServer()
	s.quotes.discountsEnabled = true
	s.quotes.resolveRoute = (&reader{}).cachedQuoteRoute
	body := validQuoteBody()
	id := common.HexToHash("0x1234").Hex()
	body.Adapters[0].DiscountID = &id
	response := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
	if response.Code != http.StatusNoContent {
		t.Fatalf("uncached native quote = %d: %s", response.Code, response.Body.String())
	}
}

func TestQuoteRequiresSnapshotPricing(t *testing.T) {
	for _, precision := range []string{"", "1000000", "100000000"} {
		for name, change := range map[string]func(*quoteAdapter){
			"missing price":    func(v *quoteAdapter) { v.Price = "" },
			"zero price":       func(v *quoteAdapter) { v.Price = "0" },
			"overflow price":   func(v *quoteAdapter) { v.Price = new(big.Int).Lsh(big.NewInt(1), 256).String() },
			"missing decimals": func(v *quoteAdapter) { v.TokenInDecimals = nil },
		} {
			t.Run(precision+"/"+name, func(t *testing.T) {
				s := testServer()
				s.quotes.discountsEnabled = true
				body := validQuoteBody()
				if precision != "" {
					id := common.HexToHash("0x1234").Hex()
					body.Adapters[0].DiscountID = &id
					body.Adapters[0].DiscountPrecision = precision
				}
				if precision == "100000000" {
					s.quotes.resolveRoute = testConnectorQuoteRoute(common.HexToAddress("0x43"))
				}
				change(&body.Adapters[0])
				response := do(t, s.handler(), http.MethodPost, "/quote", testSecret, body)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("invalid snapshot accepted: %d %s", response.Code, response.Body.String())
				}
			})
		}
	}
}
