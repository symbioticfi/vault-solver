package discounts

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const (
	testOfferID  = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testOfferIDB = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testOfferIDC = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

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

func TestOfferIssueOrderingAndRatePrecedence(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	economic := testOffer(base, "1000", new(big.Int).Add(base.MaxRate, big.NewInt(1)).String(), now.Add(time.Minute))
	economic.DiscountID = testOfferIDB
	economic.Discount = new(big.Int).Sub(base.AdapterMinDiscount, big.NewInt(1)).String()
	malformed := testOffer(base, "not-a-decimal", base.MaxRate.String(), now.Add(time.Minute))
	malformed.DiscountID = testOfferIDC
	listed := &List{Discounts: []ListItem{economic, malformed}}

	t.Run("inventories", func(t *testing.T) {
		inventory, issues := MatchInventories(listed, []liquidlane.Inventory{base}, MatchOptions{Now: now})
		if len(inventory) != 0 {
			t.Fatalf("inventory = %+v, want none", inventory)
		}
		assertOfferIssues(t, issues, []expectedOfferIssue{
			{discountID: testOfferIDC, err: `maxAssets: invalid positive decimal "not-a-decimal"`},
			{discountID: testOfferIDB, err: "advertised discount rate exceeds current adapter max rate"},
		})
	})

	t.Run("fill quotes", func(t *testing.T) {
		quotes, issues := AdvertisedFillQuotes(
			listed,
			[]liquidlane.FillQuote{testPhysicalFillQuote(base)},
			MatchOptions{Now: now},
		)
		if len(quotes) != 0 {
			t.Fatalf("quotes = %+v, want none", quotes)
		}
		assertOfferIssues(t, issues, []expectedOfferIssue{
			{discountID: testOfferIDC, err: `maxAssets: invalid positive decimal "not-a-decimal"`},
			{discountID: testOfferIDB, err: "advertised discount rate exceeds current adapter max rate"},
		})
	})
}

func TestMatchInventoriesDuplicateDiscountIDUsesFirstValidOffer(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	valid := testOffer(base, "111", base.MaxRate.String(), now.Add(2*time.Minute))
	invalid := testOffer(base, "222", new(big.Int).Add(base.MaxRate, big.NewInt(1)).String(), now.Add(time.Minute))

	t.Run("invalid first does not suppress later valid duplicate", func(t *testing.T) {
		inventory, issues := MatchInventories(
			&List{Discounts: []ListItem{invalid, valid}},
			[]liquidlane.Inventory{base},
			MatchOptions{Now: now},
		)
		if len(inventory) != 1 || inventory[0].MaxAssets.String() != "111" ||
			!inventory[0].ValidUntil.Equal(now.Add(2*time.Minute)) {
			t.Fatalf("inventory = %+v, want later valid duplicate", inventory)
		}
		assertOfferIssues(t, issues, []expectedOfferIssue{{
			discountID: testOfferID,
			err:        "advertised discount rate exceeds current adapter max rate",
		}})
	})

	t.Run("valid first suppresses later invalid duplicate", func(t *testing.T) {
		inventory, issues := MatchInventories(
			&List{Discounts: []ListItem{valid, invalid}},
			[]liquidlane.Inventory{base},
			MatchOptions{Now: now},
		)
		if len(issues) != 0 || len(inventory) != 1 || inventory[0].MaxAssets.String() != "111" ||
			!inventory[0].ValidUntil.Equal(now.Add(2*time.Minute)) {
			t.Fatalf("inventory=%+v issues=%+v, want only first valid duplicate", inventory, issues)
		}
	})
}

func TestAdvertisedFillQuotesDuplicateDiscountIDUsesFirstValidOffer(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	valid := testOffer(base, "111", base.MaxRate.String(), now.Add(2*time.Minute))
	invalid := testOffer(base, "222", new(big.Int).Add(base.MaxRate, big.NewInt(1)).String(), now.Add(time.Minute))
	physical := []liquidlane.FillQuote{testPhysicalFillQuote(base)}

	t.Run("invalid first does not suppress later valid duplicate", func(t *testing.T) {
		quotes, issues := AdvertisedFillQuotes(
			&List{Discounts: []ListItem{invalid, valid}}, physical, MatchOptions{Now: now},
		)
		if len(quotes) != 1 || quotes[0].MaxAssets.String() != "111" ||
			!quotes[0].ValidUntil.Equal(now.Add(2*time.Minute)) {
			t.Fatalf("quotes = %+v, want later valid duplicate", quotes)
		}
		assertOfferIssues(t, issues, []expectedOfferIssue{{
			discountID: testOfferID,
			err:        "advertised discount rate exceeds current adapter max rate",
		}})
	})

	t.Run("valid first suppresses later invalid duplicate", func(t *testing.T) {
		quotes, issues := AdvertisedFillQuotes(
			&List{Discounts: []ListItem{valid, invalid}}, physical, MatchOptions{Now: now},
		)
		if len(issues) != 0 || len(quotes) != 1 || quotes[0].MaxAssets.String() != "111" ||
			!quotes[0].ValidUntil.Equal(now.Add(2*time.Minute)) {
			t.Fatalf("quotes=%+v issues=%+v, want only first valid duplicate", quotes, issues)
		}
	})
}

func TestMatchInventoriesSilentlySkipsMismatches(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()

	tests := []struct {
		name    string
		mutate  func(*ListItem)
		options MatchOptions
	}{
		{
			name: "route",
			mutate: func(offer *ListItem) {
				offer.Adapter = common.HexToAddress("0x5555555555555555555555555555555555555555").Hex()
			},
			options: MatchOptions{Now: now},
		},
		{
			name: "collateral decimals",
			mutate: func(offer *ListItem) {
				offer.CollateralDecimals++
			},
			options: MatchOptions{Now: now},
		},
		{
			name:    "token policy",
			mutate:  func(*ListItem) {},
			options: MatchOptions{Now: now, AllowsToken: func(common.Address) bool { return false }},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := testOffer(base, "1000", base.MaxRate.String(), now.Add(time.Minute))
			tt.mutate(&offer)
			inventory, issues := MatchInventories(
				&List{Discounts: []ListItem{offer}}, []liquidlane.Inventory{base}, tt.options,
			)
			if len(inventory) != 0 || len(issues) != 0 {
				t.Fatalf("inventory=%+v issues=%+v, want silent skip", inventory, issues)
			}
		})
	}
}

func TestMatchInventoriesUsesLastPhysicalDuplicateRoute(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	first := testPhysicalInventory()
	first.MaxAssets = big.NewInt(111)
	last := testPhysicalInventory()
	last.MaxAssets = big.NewInt(222)
	offer := testOffer(last, "1000", last.MaxRate.String(), now.Add(time.Minute))

	inventory, issues := MatchInventories(
		&List{Discounts: []ListItem{offer}},
		[]liquidlane.Inventory{first, last},
		MatchOptions{Now: now},
	)
	if len(issues) != 0 || len(inventory) != 1 || inventory[0].MaxAssets.String() != "222" {
		t.Fatalf("inventory=%+v issues=%+v, want cap from last physical duplicate", inventory, issues)
	}
}

func TestAdvertisedFillQuotesSilentlySkipsMismatches(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()

	tests := []struct {
		name    string
		mutate  func(*ListItem)
		options MatchOptions
	}{
		{
			name: "route",
			mutate: func(offer *ListItem) {
				offer.Adapter = common.HexToAddress("0x5555555555555555555555555555555555555555").Hex()
			},
			options: MatchOptions{Now: now},
		},
		{
			name: "collateral decimals",
			mutate: func(offer *ListItem) {
				offer.CollateralDecimals++
			},
			options: MatchOptions{Now: now},
		},
		{
			name:    "token policy",
			mutate:  func(*ListItem) {},
			options: MatchOptions{Now: now, AllowsToken: func(common.Address) bool { return false }},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := testOffer(base, "1000", base.MaxRate.String(), now.Add(time.Minute))
			tt.mutate(&offer)
			quotes, issues := AdvertisedFillQuotes(
				&List{Discounts: []ListItem{offer}},
				[]liquidlane.FillQuote{testPhysicalFillQuote(base)},
				tt.options,
			)
			if len(quotes) != 0 || len(issues) != 0 {
				t.Fatalf("quotes=%+v issues=%+v, want silent skip", quotes, issues)
			}
		})
	}
}

func TestAdvertisedFillQuotesUsesLastPhysicalDuplicateRoute(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	first := testPhysicalFillQuote(base)
	first.MaxAssets = big.NewInt(111)
	first.AmountIn = big.NewInt(10)
	first.GrossAmountOut = big.NewInt(10)
	last := testPhysicalFillQuote(base)
	last.MaxAssets = big.NewInt(222)
	last.AmountIn = big.NewInt(20)
	last.GrossAmountOut = big.NewInt(20)
	offer := testOffer(base, "1000", base.MaxRate.String(), now.Add(time.Minute))

	quotes, issues := AdvertisedFillQuotes(
		&List{Discounts: []ListItem{offer}},
		[]liquidlane.FillQuote{first, last},
		MatchOptions{Now: now},
	)
	if len(issues) != 0 || len(quotes) != 1 || quotes[0].MaxAssets.String() != "222" ||
		quotes[0].AmountIn.String() != "20" || quotes[0].GrossAmountOut.String() != "20" {
		t.Fatalf("quotes=%+v issues=%+v, want facts from last physical duplicate", quotes, issues)
	}
}

func TestMatchInventoriesFailClosedOnInvalidPhysicalEconomics(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tt := range physicalEconomicsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			base := testPhysicalInventory()
			offer := testOffer(base, "1000", base.MaxRate.String(), now.Add(time.Minute))
			mutateInventoryEconomics(&base, tt.mutation)
			inventory, issues := MatchInventories(
				&List{Discounts: []ListItem{offer}}, []liquidlane.Inventory{base}, MatchOptions{Now: now},
			)
			if len(inventory) != tt.wantCount {
				t.Fatalf("inventory = %+v, want count %d", inventory, tt.wantCount)
			}
			assertOfferIssues(t, issues, tt.wantIssues)
		})
	}
}

func TestAdvertisedFillQuotesFailClosedOnInvalidPhysicalEconomics(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tt := range physicalEconomicsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			inventory := testPhysicalInventory()
			offer := testOffer(inventory, "1000", inventory.MaxRate.String(), now.Add(time.Minute))
			base := testPhysicalFillQuote(inventory)
			mutateFillQuoteEconomics(&base, tt.mutation)
			quotes, issues := AdvertisedFillQuotes(
				&List{Discounts: []ListItem{offer}}, []liquidlane.FillQuote{base}, MatchOptions{Now: now},
			)
			if len(quotes) != tt.wantCount {
				t.Fatalf("quotes = %+v, want count %d", quotes, tt.wantCount)
			}
			assertOfferIssues(t, issues, tt.wantIssues)
		})
	}
}

func TestMatchInventoriesClipsMaxAssetsAtOfferAndPhysicalCaps(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	tests := []struct {
		name      string
		maxAssets string
		want      string
	}{
		{name: "offer limited", maxAssets: "400", want: "400"},
		{name: "physical limited", maxAssets: "2000", want: "1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := testOffer(base, tt.maxAssets, base.MaxRate.String(), now.Add(time.Minute))
			inventory, issues := MatchInventories(
				&List{Discounts: []ListItem{offer}}, []liquidlane.Inventory{base}, MatchOptions{Now: now},
			)
			if len(issues) != 0 || len(inventory) != 1 || inventory[0].MaxAssets.String() != tt.want {
				t.Fatalf("inventory=%+v issues=%+v, want max assets %s", inventory, issues, tt.want)
			}
		})
	}
}

func TestAdvertisedFillQuotesClipMaxAssetsAtOfferAndPhysicalCaps(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	physical := []liquidlane.FillQuote{testPhysicalFillQuote(base)}
	tests := []struct {
		name      string
		maxAssets string
		want      string
	}{
		{name: "offer limited", maxAssets: "400", want: "400"},
		{name: "physical limited", maxAssets: "2000", want: "1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := testOffer(base, tt.maxAssets, base.MaxRate.String(), now.Add(time.Minute))
			quotes, issues := AdvertisedFillQuotes(
				&List{Discounts: []ListItem{offer}}, physical, MatchOptions{Now: now},
			)
			if len(issues) != 0 || len(quotes) != 1 || quotes[0].MaxAssets.String() != tt.want {
				t.Fatalf("quotes=%+v issues=%+v, want max assets %s", quotes, issues, tt.want)
			}
		})
	}
}

func TestAdvertisedFillQuotesPinsNestedRoundingBoundary(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	route := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		18,
		18,
	)
	advertisedRate := mustTestBig(t, "1034566856666676655")
	base := testInventoryWithMinDiscount(
		liquidlane.DirectInventory(route, mustTestBig(t, "2000000000000000"), advertisedRate),
		new(big.Int),
	)
	physical := []liquidlane.FillQuote{{
		Inventory:      base,
		AmountIn:       mustTestBig(t, "1000000000000000"),
		GrossAmountOut: mustTestBig(t, "1034567891234567"),
		MaxAmountOut:   mustTestBig(t, "1034567891234567"),
		MinDiscount:    new(big.Int),
	}}
	offer := testOffer(base, "2000000000000000", advertisedRate.String(), now.Add(time.Minute))
	offer.Discount = "1"

	quotes, issues := AdvertisedFillQuotes(
		&List{Discounts: []ListItem{offer}}, physical, MatchOptions{Now: now},
	)
	if len(issues) != 0 || len(quotes) != 1 {
		t.Fatalf("quotes=%+v issues=%+v", quotes, issues)
	}
	if got := quotes[0].MaxRate.String(); got != "1034566856666675000" {
		t.Fatalf("MaxRate = %s, want 1034566856666675000", got)
	}
	if got := quotes[0].MaxAmountOut.String(); got != "1034566856666675" {
		t.Fatalf("MaxAmountOut = %s, want 1034566856666675", got)
	}
	rawAdvertisedOut := liquidlane.AmountOutForRate(physical[0].AmountIn, advertisedRate, 18, 18)
	if difference := new(big.Int).Sub(rawAdvertisedOut, quotes[0].MaxAmountOut); difference.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("raw advertised output %s - nested-rounded output %s = %s, want 1", rawAdvertisedOut, quotes[0].MaxAmountOut, difference)
	}
}

func TestMatchInventoriesDoesNotAliasPhysicalBigInts(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	offer := testOffer(base, "800", "800000000000000000", now.Add(time.Minute))

	inventory, issues := MatchInventories(
		&List{Discounts: []ListItem{offer}}, []liquidlane.Inventory{base}, MatchOptions{Now: now},
	)
	if len(issues) != 0 || len(inventory) != 1 {
		t.Fatalf("inventory=%+v issues=%+v", inventory, issues)
	}
	source := []*big.Int{base.MaxAssets, base.MaxRate, base.AdapterMinDiscount}
	returned := []*big.Int{inventory[0].MaxAssets, inventory[0].MaxRate, inventory[0].AdapterMinDiscount}
	assertNoBigIntAliases(t, source, returned)

	base.MaxAssets.SetInt64(1)
	base.MaxRate.SetInt64(2)
	base.AdapterMinDiscount.SetInt64(3)
	assertBigIntValues(t, returned, []string{"800", "800000000000000000", "100000"})

	for _, value := range returned {
		value.SetInt64(9)
	}
	assertBigIntValues(t, source, []string{"1", "2", "3"})
}

func TestAdvertisedFillQuotesDoNotAliasPhysicalBigInts(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	inventory := testPhysicalInventory()
	base := testPhysicalFillQuote(inventory)
	offer := testOffer(inventory, "800", "800000000000000000", now.Add(time.Minute))

	quotes, issues := AdvertisedFillQuotes(
		&List{Discounts: []ListItem{offer}}, []liquidlane.FillQuote{base}, MatchOptions{Now: now},
	)
	if len(issues) != 0 || len(quotes) != 1 {
		t.Fatalf("quotes=%+v issues=%+v", quotes, issues)
	}
	source := []*big.Int{
		base.MaxAssets,
		base.MaxRate,
		base.AdapterMinDiscount,
		base.AmountIn,
		base.GrossAmountOut,
		base.MaxAmountOut,
		base.MinDiscount,
	}
	returned := []*big.Int{
		quotes[0].MaxAssets,
		quotes[0].MaxRate,
		quotes[0].AdapterMinDiscount,
		quotes[0].AmountIn,
		quotes[0].GrossAmountOut,
		quotes[0].MaxAmountOut,
		quotes[0].MinDiscount,
	}
	assertNoBigIntAliases(t, source, returned)

	for index, value := range source {
		value.SetInt64(int64(index + 1))
	}
	assertBigIntValues(t, returned, []string{
		"800", "800000000000000000", "100000", "1000", "1000", "800", "100000",
	})

	for _, value := range returned {
		value.SetInt64(9)
	}
	assertBigIntValues(t, source, []string{"1", "2", "3", "4", "5", "6", "7"})
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

type physicalEconomicsMutation int

const (
	maxRateNil physicalEconomicsMutation = iota
	maxRateZero
	maxRateNegative
	minimumDiscountNil
	minimumDiscountZero
	minimumDiscountNegative
	maxAssetsNil
	maxAssetsZero
	maxAssetsNegative
)

type physicalEconomicsTestCase struct {
	name       string
	mutation   physicalEconomicsMutation
	wantCount  int
	wantIssues []expectedOfferIssue
}

func physicalEconomicsTestCases() []physicalEconomicsTestCase {
	rateIssue := []expectedOfferIssue{{
		discountID: testOfferID,
		err:        "advertised discount rate exceeds current adapter max rate",
	}}
	minimumIssue := []expectedOfferIssue{{
		discountID: testOfferID,
		err:        "advertised discount is below current adapter minimum",
	}}
	return []physicalEconomicsTestCase{
		{name: "nil max rate", mutation: maxRateNil, wantIssues: rateIssue},
		{name: "zero max rate", mutation: maxRateZero, wantIssues: rateIssue},
		{name: "negative max rate", mutation: maxRateNegative, wantIssues: rateIssue},
		{name: "nil minimum discount", mutation: minimumDiscountNil, wantIssues: minimumIssue},
		{name: "zero minimum discount is valid", mutation: minimumDiscountZero, wantCount: 1},
		{name: "negative minimum discount", mutation: minimumDiscountNegative, wantIssues: minimumIssue},
		{name: "nil max assets", mutation: maxAssetsNil},
		{name: "zero max assets", mutation: maxAssetsZero},
		{name: "negative max assets", mutation: maxAssetsNegative},
	}
}

func mutateInventoryEconomics(base *liquidlane.Inventory, mutation physicalEconomicsMutation) {
	switch mutation {
	case maxRateNil:
		base.MaxRate = nil
	case maxRateZero:
		base.MaxRate = new(big.Int)
	case maxRateNegative:
		base.MaxRate = big.NewInt(-1)
	case minimumDiscountNil:
		base.AdapterMinDiscount = nil
	case minimumDiscountZero:
		base.AdapterMinDiscount = new(big.Int)
	case minimumDiscountNegative:
		base.AdapterMinDiscount = big.NewInt(-1)
	case maxAssetsNil:
		base.MaxAssets = nil
	case maxAssetsZero:
		base.MaxAssets = new(big.Int)
	case maxAssetsNegative:
		base.MaxAssets = big.NewInt(-1)
	}
}

func mutateFillQuoteEconomics(base *liquidlane.FillQuote, mutation physicalEconomicsMutation) {
	switch mutation {
	case maxRateNil:
		base.MaxRate = nil
	case maxRateZero:
		base.MaxRate = new(big.Int)
	case maxRateNegative:
		base.MaxRate = big.NewInt(-1)
	case minimumDiscountNil:
		base.MinDiscount = nil
	case minimumDiscountZero:
		base.MinDiscount = new(big.Int)
	case minimumDiscountNegative:
		base.MinDiscount = big.NewInt(-1)
	case maxAssetsNil:
		base.MaxAssets = nil
	case maxAssetsZero:
		base.MaxAssets = new(big.Int)
	case maxAssetsNegative:
		base.MaxAssets = big.NewInt(-1)
	}
}

type expectedOfferIssue struct {
	discountID string
	err        string
}

func assertOfferIssues(t *testing.T, got []OfferIssue, want []expectedOfferIssue) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("issues = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index].DiscountID != want[index].discountID || got[index].Err.Error() != want[index].err {
			t.Fatalf(
				"issue[%d] = {%q, %q}, want {%q, %q}",
				index,
				got[index].DiscountID,
				got[index].Err,
				want[index].discountID,
				want[index].err,
			)
		}
	}
}

func assertNoBigIntAliases(t *testing.T, source, returned []*big.Int) {
	t.Helper()
	for sourceIndex, sourceValue := range source {
		for returnedIndex, returnedValue := range returned {
			if sourceValue == returnedValue {
				t.Fatalf("source[%d] aliases returned[%d] at %p", sourceIndex, returnedIndex, sourceValue)
			}
		}
	}
}

func assertBigIntValues(t *testing.T, got []*big.Int, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d values, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].String() != want[index] {
			t.Fatalf("value[%d] = %s, want %s", index, got[index], want[index])
		}
	}
}

func mustTestBig(t *testing.T, raw string) *big.Int {
	t.Helper()
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		t.Fatalf("invalid test integer %q", raw)
	}
	return value
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

func testPhysicalFillQuote(base liquidlane.Inventory) liquidlane.FillQuote {
	return liquidlane.FillQuote{
		Inventory:      base,
		AmountIn:       big.NewInt(1_000),
		GrossAmountOut: big.NewInt(1_000),
		MaxAmountOut:   big.NewInt(900),
		MinDiscount:    big.NewInt(100_000),
	}
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
