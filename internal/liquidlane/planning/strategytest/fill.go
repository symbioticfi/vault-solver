// Package strategytest contains contract scenarios run against each direct executor strategy.
// It is imported only by tests, never by the application.
package strategytest

import (
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

// CheckFill runs the same capacity and route-selection contracts through the real strategy.
// Both zero-decimal and 18-decimal fixtures are retained from the integration-local tests.
func CheckFill(t *testing.T, decide func(planning.FillInput) ([]planning.FillRoute, error)) {
	t.Helper()
	for _, decimals := range []int{0, 18} {
		t.Run(strconv.Itoa(decimals)+" decimals", func(t *testing.T) {
			checkFill(t, decimals, decide)
		})
	}
}

func checkFill(t *testing.T, decimals int, decide func(planning.FillInput) ([]planning.FillRoute, error)) {
	t.Helper()
	tokenIn, tokenOut := common.Address{1}, common.Address{2}
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	quote := func(adapter byte, capacityID liquidlane.CapacityID, amount, capacity, output int64) liquidlane.FillQuote {
		return liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{ID: liquidlane.RouteID(strconv.Itoa(int(adapter))), CapacityID: capacityID,
					Adapter: common.Address{adapter}, TokenIn: tokenIn, TokenOut: tokenOut,
					TokenInDecimals: decimals, TokenOutDecimals: decimals},
				MaxAssets: big.NewInt(capacity),
			},
			AmountIn: big.NewInt(amount), GrossAmountOut: big.NewInt(output), MaxAmountOut: big.NewInt(output),
		}
	}
	private := quote(1, "capacity-1", 1_000, 1_000, 950)
	private.DiscountID, private.MinDiscount = &discountID, big.NewInt(100_000)
	for _, tt := range []struct {
		name           string
		amount, output int64
		quotes         []liquidlane.FillQuote
		reservations   liquidlane.CapacityReservations
		count          int
		adapter        common.Address
		private        bool
		singleRoute    bool
	}{
		{name: "multi-route", amount: 1_000, output: 900, count: 2, quotes: []liquidlane.FillQuote{
			quote(1, "capacity-1", 1_000, 500, 1_000), quote(2, "capacity-2", 1_000, 500, 1_000),
		}},
		{name: "single route", amount: 1_000, output: 900, singleRoute: true, quotes: []liquidlane.FillQuote{
			quote(1, "capacity-1", 1_000, 500, 1_000), quote(2, "capacity-2", 1_000, 500, 1_000),
		}},
		{name: "shared capacity", amount: 1_000, output: 900, quotes: []liquidlane.FillQuote{
			quote(1, "shared", 1_000, 600, 1_000), quote(2, "shared", 1_000, 600, 1_000),
		}},
		{name: "best route", amount: 1_000, output: 900, count: 1, adapter: common.Address{2}, quotes: []liquidlane.FillQuote{
			quote(1, "capacity-1", 1_000, 2_000, 1_000), quote(2, "capacity-2", 1_000, 2_000, 1_100),
		}},
		{name: "best route with implicit capacity", amount: 1_000_000, output: 990_000, count: 1, adapter: common.Address{2}, quotes: []liquidlane.FillQuote{
			quote(1, "", 1_000_000, 2_000_000, 1_000_000), quote(2, "", 1_000_000, 2_000_000, 1_100_000),
		}},
		{name: "private discount", amount: 1_000, output: 850, count: 1, private: true, quotes: []liquidlane.FillQuote{
			quote(1, "capacity-1", 1_000, 1_000, 900), private,
		}},
		{name: "pending reservation", amount: 100, output: 90,
			quotes:       []liquidlane.FillQuote{quote(1, "capacity-1", 100, 100, 100)},
			reservations: liquidlane.CapacityReservations{"capacity-1": big.NewInt(60)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := decide(planning.FillInput{
				TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(tt.amount), OutputAmount: big.NewInt(tt.output),
				RequireSingleRoute: tt.singleRoute, ChainTime: time.Unix(1_800_000_000, 0), MaxFeePerGas: new(big.Int), Quotes: tt.quotes, Reservations: tt.reservations,
			})
			testcheck.NoError(t, err)
			if len(routes) != tt.count || tt.count == 0 && routes != nil {
				t.Fatalf("routes = %+v, want %d routes", routes, tt.count)
			}
			if tt.count == 0 {
				return
			}
			if tt.adapter != (common.Address{}) && routes[0].Adapter != tt.adapter {
				t.Fatalf("selected adapter = %s, want %s", routes[0].Adapter, tt.adapter)
			}
			if tt.private && (routes[0].DiscountID == nil || *routes[0].DiscountID != discountID) {
				t.Fatalf("selected discount = %v, want %s", routes[0].DiscountID, discountID)
			}
			totalIn, minimumOut := new(big.Int), new(big.Int)
			for _, route := range routes {
				totalIn.Add(totalIn, route.AmountIn)
				minimumOut.Add(minimumOut, route.MinAmountOut)
			}
			if totalIn.Cmp(big.NewInt(tt.amount)) != 0 || minimumOut.Cmp(big.NewInt(tt.output)) != 0 {
				t.Fatalf("totals = %s/%s, want %d/%d", totalIn, minimumOut, tt.amount, tt.output)
			}
		})
	}
}
