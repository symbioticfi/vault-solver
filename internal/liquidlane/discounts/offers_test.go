package discounts

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const testOfferID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestMatchInventoriesScopesCapsAndKeepsAdvertisedNetRate(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	netRate := big.NewInt(800_000_000_000_000_000)
	listed := &List{Discounts: []ListItem{
		testOffer(base, "2000", netRate.String(), now.Add(time.Minute)),
		{
			DiscountID: testOfferID,
			Adapter:    common.HexToAddress("0xdead").Hex(), TokenToRedeem: base.TokenIn.Hex(),
			Collateral: base.TokenOut.Hex(), CollateralDecimals: base.TokenOutDecimals,
			Discount: "100000", Deadline: now.Add(time.Minute).Unix(), MaxRate: netRate.String(), MaxAssets: "2000",
		},
	}}

	inventory, issues := MatchInventories(listed, []liquidlane.Inventory{base}, MatchOptions{Now: now})
	if len(issues) != 0 || len(inventory) != 1 {
		t.Fatalf("inventory=%+v issues=%+v", inventory, issues)
	}
	if inventory[0].MaxAssets.String() != "1000" || inventory[0].MaxRate.Cmp(netRate) != 0 {
		t.Fatalf("capped inventory = %+v", inventory[0])
	}
	if inventory[0].DiscountID == nil || inventory[0].DiscountID.Hex() != testOfferID {
		t.Fatalf("discount id = %v", inventory[0].DiscountID)
	}
	if !inventory[0].ValidUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("valid until = %s", inventory[0].ValidUntil)
	}
}

func TestMatchInventoriesRejectsDiscountBelowCurrentAdapterMinimum(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	offer := testOffer(base, "1000", base.MaxRate.String(), now.Add(time.Minute))
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

func TestAdvertisedFillQuotesUseCurrentOracleAmountAndPolicy(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	listed := &List{Discounts: []ListItem{
		testOffer(base, "100", "2000000000000000000", now.Add(time.Minute)),
	}}
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

func TestAdvertisedFillQuotesRejectStaleAdapterEconomics(t *testing.T) {
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
		name     string
		discount string
		maxRate  string
	}{
		{name: "rate above current maximum", discount: "100000", maxRate: "901"},
		{name: "discount below current minimum", discount: "99999", maxRate: "900"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := testOffer(base, "100", tt.maxRate, now.Add(time.Minute))
			offer.Discount = tt.discount
			quotes, issues := AdvertisedFillQuotes(
				&List{Discounts: []ListItem{offer}}, physical, MatchOptions{Now: now},
			)
			if len(quotes) != 0 || len(issues) != 1 {
				t.Fatalf("quotes=%+v issues=%+v", quotes, issues)
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
		offer := testOffer(base, big.NewInt(maxAssets).String(), maxRate.String(), now.Add(time.Minute))
		offer.Discount = big.NewInt(discount).String()

		quotes, issues := AdvertisedFillQuotes(
			&List{Discounts: []ListItem{offer}}, physical, MatchOptions{Now: now},
		)
		if len(issues) != 0 || len(quotes) == 0 {
			return
		}
		quote := quotes[0]
		if quote.MaxAmountOut.Sign() <= 0 || quote.MaxAmountOut.Cmp(big.NewInt(gross)) > 0 {
			t.Fatalf("amountOut = %s, gross = %d", quote.MaxAmountOut, gross)
		}
		if quote.MaxAssets.Sign() <= 0 || quote.MaxAssets.Cmp(big.NewInt(maxAssets)) > 0 {
			t.Fatalf("maxAssets = %s, physical = %d", quote.MaxAssets, maxAssets)
		}
		if quote.MaxRate.Cmp(base.MaxRate) > 0 {
			t.Fatalf("maxRate = %s, physical = %s", quote.MaxRate, base.MaxRate)
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
	return inventory
}

func testInventoryWithMinDiscount(inventory liquidlane.Inventory, minDiscount *big.Int) liquidlane.Inventory {
	inventory.AdapterMinDiscount = liquidlane.CloneBig(minDiscount)
	return inventory
}

func testOffer(base liquidlane.Inventory, maxAssets, maxRate string, deadline time.Time) ListItem {
	return ListItem{
		DiscountID: testOfferID,
		Adapter:    base.Adapter.Hex(), TokenToRedeem: base.TokenIn.Hex(), Collateral: base.TokenOut.Hex(),
		CollateralDecimals: base.TokenOutDecimals, Discount: "100000", Deadline: deadline.Unix(),
		MaxRate: maxRate, MaxAssets: maxAssets,
	}
}
