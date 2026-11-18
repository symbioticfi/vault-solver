package discounts

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/single"
)

const testOfferID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestMatchInventoriesScopesCapsAndPricesSignedDiscount(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	listed := &List{Discounts: []ListItem{
		testOffer(base, "2000", now.Add(time.Minute)),
		{
			DiscountID: testOfferID,
			Adapter:    common.HexToAddress("0xdead").Hex(), TokenToRedeem: base.TokenIn.Hex(),
			Collateral: base.TokenOut.Hex(), CollateralDecimals: base.TokenOutDecimals,
			Discount: "100000", Deadline: now.Add(time.Minute).Unix(), MaxAssets: "2000",
		},
	}}

	inventory, issues := MatchInventories(listed, []liquidlane.Inventory{base}, MatchOptions{Now: now})
	if len(issues) != 0 || len(inventory) != 1 {
		t.Fatalf("inventory=%+v issues=%+v", inventory, issues)
	}
	got := inventory[0]
	if got.MaxAssets.String() != "1000" || got.Price.Cmp(base.Price) != 0 ||
		got.Discount.String() != "100000" || got.MaxRate.Cmp(liquidlane.DiscountedRate(base.Price, got.Discount)) != 0 {
		t.Fatalf("capped inventory = %+v", got)
	}
	if got.DiscountID == nil || got.DiscountID.Hex() != testOfferID {
		t.Fatalf("discount id = %v", got.DiscountID)
	}
	if !got.ValidUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("valid until = %s", got.ValidUntil)
	}
}

func TestMatchInventoriesRejectsDiscountBelowCurrentAdapterMinimum(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	offer := testOffer(base, "1000", now.Add(time.Minute))
	offer.Discount = new(big.Int).Sub(base.AdapterMinDiscount, big.NewInt(1)).String()

	inventory, issues := MatchInventories(
		&List{Discounts: []ListItem{offer}},
		[]liquidlane.Inventory{base},
		MatchOptions{Now: now},
	)
	if len(inventory) != 0 || len(issues) != 1 {
		t.Fatalf("inventory=%+v issues=%+v", inventory, issues)
	}
}

// The listing carries no rate, so a route without the current oracle price cannot be priced.
func TestMatchInventoriesRequiresOraclePrice(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	base.Price = nil

	inventory, issues := MatchInventories(
		&List{Discounts: []ListItem{testOffer(base, "1000", now.Add(time.Minute))}},
		[]liquidlane.Inventory{base},
		MatchOptions{Now: now},
	)
	if len(inventory) != 0 || len(issues) != 1 {
		t.Fatalf("inventory=%+v issues=%+v", inventory, issues)
	}
}

func TestAdvertisedFillQuotesUseCurrentOracleAmountAndPolicy(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	listed := &List{Discounts: []ListItem{testOffer(base, "100", now.Add(time.Minute))}}
	listed.Discounts[0].Discount = "100000"
	physical := []liquidlane.FillQuote{{
		Inventory: testInventoryWithMinDiscount(
			liquidlane.DirectInventory(base.Route, big.NewInt(100), big.NewInt(2_000_000_000_000_000_000)),
			new(big.Int),
		),
		AmountIn: big.NewInt(10), GrossAmountOut: big.NewInt(20), MaxAmountOut: big.NewInt(20),
		MinDiscount: new(big.Int),
	}}

	quotes, issues := AdvertisedFillQuotes(listed, physical, MatchOptions{
		Now:         now,
		AllowsToken: func(token common.Address) bool { return token == base.TokenIn },
	})
	if len(issues) != 0 || len(quotes) != 1 || quotes[0].MaxAmountOut.Cmp(big.NewInt(18)) != 0 {
		t.Fatalf("quotes=%+v issues=%+v", quotes, issues)
	}
	blocked, _ := AdvertisedFillQuotes(listed, physical, MatchOptions{
		Now: now, AllowsToken: func(common.Address) bool { return false },
	})
	if len(blocked) != 0 {
		t.Fatalf("blocked quotes = %+v", blocked)
	}
}

func TestAdvertisedFillQuotesReserveFullPayout(t *testing.T) {
	for _, scenario := range []struct {
		name                           string
		decimals                       int
		input, gross, discount, payout int64
	}{
		{"6 decimals", 6, 300_000_000, 318_529_800, 200, 318_466_094},
		{"18 decimals", 18, 2_000_000_000_000_000_000, 2_000_000_000_000_000_002, 1, 1_999_998_000_000_000_001},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			now := time.Unix(1_800_000_000, 0)
			base := testPhysicalInventory()
			base.TokenInDecimals, base.TokenOutDecimals = scenario.decimals, scenario.decimals
			base.MaxRate, base.MaxAssets = big.NewInt(2e18), big.NewInt(scenario.payout)
			base.AdapterMinDiscount = new(big.Int)
			physical := liquidlane.FillQuote{
				Inventory: base, AmountIn: big.NewInt(scenario.input), GrossAmountOut: big.NewInt(scenario.gross),
				MinDiscount: new(big.Int),
			}
			offer := testOffer(base, base.MaxAssets.String(), now.Add(time.Minute))
			offer.Discount = big.NewInt(scenario.discount).String()
			listed := &List{Discounts: []ListItem{offer}}
			// Quotes are exact, so the order requires the whole payout.
			required := big.NewInt(scenario.payout)
			quotes, issues := AdvertisedFillQuotes(listed, []liquidlane.FillQuote{physical}, MatchOptions{Now: now})
			if len(issues) != 0 || len(quotes) != 1 || quotes[0].MaxAmountOut.Cmp(big.NewInt(scenario.payout)) != 0 {
				t.Fatalf("quotes=%+v issues=%+v; want payout %d", quotes, issues, scenario.payout)
			}
			signed := &Signed{
				DiscountID: *quotes[0].DiscountID, Adapter: base.Adapter,
				Terms:            SignedTerms{TokenToRedeem: base.TokenIn, Discount: big.NewInt(scenario.discount), Deadline: big.NewInt(offer.Deadline)},
				ProtocolDeadline: big.NewInt(offer.Deadline),
			}
			for _, tt := range []struct {
				capacity, pending int64
				wantFill          bool
			}{
				{scenario.payout, 0, true},
				{scenario.payout - 1, 0, false},
				{scenario.payout, 1, false},
			} {
				quote := quotes[0]
				quote.MaxAssets = big.NewInt(tt.capacity)
				solution, err := single.SolveFill(strategies.FillTask{
					TokenIn: base.TokenIn, TokenOut: base.TokenOut, AmountIn: physical.AmountIn,
					Quotes: []liquidlane.FillQuote{quote}, ValidAfter: now,
					Reservations: liquidlane.CapacityReservations{liquidlane.RouteCapacityID(base.Route): big.NewInt(tt.pending)},
				})
				if err != nil || (solution != nil) != tt.wantFill {
					t.Fatalf("capacity=%d pending=%d: solution=%+v err=%v; want fill %v", tt.capacity, tt.pending, solution, err, tt.wantFill)
				}
				if solution == nil {
					continue
				}
				routes := solution.Finalize(required)
				if len(routes) != 1 || routes[0].ReservedAmountOut.Cmp(big.NewInt(scenario.payout)) != 0 || routes[0].MinAmountOut.Cmp(required) != 0 {
					t.Fatalf("routes=%+v; want minimum %s and reservation %d", routes, required, scenario.payout)
				}
				// The unchanged signed discount must resolve inside the plan: a reservation below the
				// payout aborts with "exceeds the selected capacity reservation".
				if _, err := ValidateSigned(signed, Selection{
					DiscountID: signed.DiscountID, Adapter: base.Adapter, TokenIn: base.TokenIn,
					MinAmountOut: routes[0].MinAmountOut, MaxAmountOut: routes[0].ReservedAmountOut,
				}, physical, now); err != nil {
					t.Fatalf("unchanged signed payout must fit the plan: %v", err)
				}
			}
		})
	}
}

func TestAdvertisedFillQuotesCheckSignedDiscountAgainstCurrentMinimum(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	physical := []liquidlane.FillQuote{{
		Inventory: testInventoryWithMinDiscount(
			liquidlane.DirectInventory(base.Route, big.NewInt(100), big.NewInt(900)),
			big.NewInt(100_000),
		),
		AmountIn: big.NewInt(10), GrossAmountOut: big.NewInt(10), MaxAmountOut: big.NewInt(9),
		MinDiscount: big.NewInt(100_000),
	}}

	tests := []struct {
		name       string
		discount   string
		wantPayout int64 // zero means rejected
	}{
		{name: "discount at current minimum", discount: "100000", wantPayout: 9},
		// The adapter reverts a discount below its current minimum.
		{name: "discount below current minimum", discount: "99999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := testOffer(base, "100", now.Add(time.Minute))
			offer.Discount = tt.discount
			quotes, issues := AdvertisedFillQuotes(
				&List{Discounts: []ListItem{offer}}, physical, MatchOptions{Now: now},
			)
			if tt.wantPayout == 0 {
				if len(quotes) != 0 || len(issues) != 1 {
					t.Fatalf("quotes=%+v issues=%+v", quotes, issues)
				}
				return
			}
			if len(issues) != 0 || len(quotes) != 1 || quotes[0].MaxAmountOut.Cmp(big.NewInt(tt.wantPayout)) != 0 {
				t.Fatalf("quotes=%+v issues=%+v; want payout %d", quotes, issues, tt.wantPayout)
			}
		})
	}
}

func FuzzAdvertisedFillQuotesStayInsideCurrentFacts(f *testing.F) {
	f.Add(uint32(1_000), uint32(900), uint32(100_000), uint32(1_000), uint64(1_000_000_000_000_000_000))
	f.Fuzz(func(
		t *testing.T,
		rawAmountIn, rawGross, rawDiscount, rawMaxAssets uint32,
		rawMaxRate uint64,
	) {
		amountIn := int64(rawAmountIn%1_000_000 + 1)
		gross := int64(rawGross%1_000_000 + 1)
		discount := int64(rawDiscount % uint32(liquidlane.DiscountPrecision+1))
		maxAssets := int64(rawMaxAssets%1_000_000 + 1)
		maxRate := new(big.Int).SetUint64(rawMaxRate%2_000_000_000_000_000_000 + 1)
		now := time.Unix(1_800_000_000, 0)
		base := testPhysicalInventory()
		base.MaxAssets = big.NewInt(maxAssets)
		base.MaxRate = maxRate
		base.AdapterMinDiscount = new(big.Int)
		physical := []liquidlane.FillQuote{{
			Inventory:      base,
			AmountIn:       big.NewInt(amountIn),
			GrossAmountOut: big.NewInt(gross),
			MaxAmountOut:   big.NewInt(gross),
			MinDiscount:    new(big.Int),
		}}
		offer := testOffer(base, big.NewInt(maxAssets).String(), now.Add(time.Minute))
		offer.Discount = big.NewInt(discount).String()

		quotes, issues := AdvertisedFillQuotes(
			&List{Discounts: []ListItem{offer}}, physical, MatchOptions{Now: now},
		)
		if len(issues) != 0 || len(quotes) == 0 {
			return
		}
		quote := quotes[0]
		payout := liquidlane.AmountOutAfterDiscount(big.NewInt(gross), big.NewInt(discount))
		if quote.MaxAmountOut.Sign() <= 0 || quote.MaxAmountOut.Cmp(payout) != 0 {
			t.Fatalf("amountOut = %s, adapter payout = %s", quote.MaxAmountOut, payout)
		}
		if quote.MaxAssets.Sign() <= 0 || quote.MaxAssets.Cmp(big.NewInt(maxAssets)) > 0 {
			t.Fatalf("maxAssets = %s, physical = %d", quote.MaxAssets, maxAssets)
		}
		if want := liquidlane.RateForAmountOut(payout, big.NewInt(amountIn), 6, 6); quote.MaxRate.Cmp(want) != 0 {
			t.Fatalf("maxRate = %s, payout rate = %s", quote.MaxRate, want)
		}
	})
}

func testPhysicalInventory() liquidlane.Inventory {
	route := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		6,
		6,
	)
	inventory := liquidlane.DirectInventory(route, big.NewInt(1_000), big.NewInt(900_000_000_000_000_000))
	inventory.AdapterMinDiscount = big.NewInt(100_000)
	inventory.Price = big.NewInt(1_000_000_000_000_000_000) // getMaxRate 0.9e18 at the 10% minimum
	return inventory
}

func testInventoryWithMinDiscount(inventory liquidlane.Inventory, minDiscount *big.Int) liquidlane.Inventory {
	inventory.AdapterMinDiscount = liquidlane.CloneBig(minDiscount)
	return inventory
}

func testOffer(base liquidlane.Inventory, maxAssets string, deadline time.Time) ListItem {
	return ListItem{
		DiscountID: testOfferID,
		Adapter:    base.Adapter.Hex(), TokenToRedeem: base.TokenIn.Hex(), Collateral: base.TokenOut.Hex(),
		CollateralDecimals: base.TokenOutDecimals, Discount: "100000", Deadline: deadline.Unix(),
		MaxAssets: maxAssets,
	}
}
