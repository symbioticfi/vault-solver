package rfq

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Every quote decline names the check that failed and the values it compared.
func TestQuoteDeclineNamesFailedCheck(t *testing.T) {
	discountID := "0x" + strings.Repeat("d", 64)
	for name, tc := range map[string]struct {
		change func(*quoteService, *quoteRequest)
		want   string
	}{
		"wrong chain": {
			change: func(_ *quoteService, q *quoteRequest) { q.TokenOutChainID = 8453 },
			want:   "not_quotable: tokenOutChainId 8453 is not chain 1",
		},
		"no adapters": {
			change: func(_ *quoteService, q *quoteRequest) { q.Adapters = nil },
			want:   "not_quotable: none of 0 adapters resolves to a configured route",
		},
		"below minimum": {
			change: func(qs *quoteService, _ *quoteRequest) {
				qs.minAmountsIn = map[common.Address]*big.Int{tIn: big.NewInt(2_000_000_000_000_000_000)}
			},
			want: "below_minimum: amount in 1000000000000000000 is below configured minimum 2000000000000000000",
		},
		"not whitelisted": {
			change: func(qs *quoteService, _ *quoteRequest) {
				qs.whitelist = buildAdapterWhitelist(true, []recoveryVault{{Adapter: common.HexToAddress("0xbad")}})
			},
			want: "no_candidates: none of 1 resolved inventories uses a whitelisted adapter",
		},
		"discounts disabled": {
			change: func(qs *quoteService, q *quoteRequest) {
				qs.discountsEnabled = false
				q.Adapters[0].DiscountID = &discountID
			},
			want: "no_candidates: all 1 whitelisted inventories are discounts and discounts are disabled",
		},
		"reserved capacity": {
			change: func(qs *quoteService, _ *quoteRequest) {
				qs.reservations = func(_ string, inventory []solverInventory) liquidlane.CapacityReservations {
					held := liquidlane.CapacityReservations{}
					for _, item := range inventory {
						held.Add(liquidlane.RouteCapacityID(item.Route), item.MaxAssets)
					}
					return held
				}
			},
			want: "no_candidates: no candidates from 1 inventories, dropped: no unreserved capacity x1; " +
				"last: liquidlane inventory declined reason=no unreserved capacity",
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := testServer()
			body := validQuoteBody()
			tc.change(srv.quotes, &body)

			decision, err := srv.quotes.evaluate(t.Context(), &body)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if got := string(decision.outcome) + ": " + decision.reason; !strings.HasPrefix(got, tc.want) {
				t.Fatalf("decline = %q, want prefix %q", got, tc.want)
			}
		})
	}
}
