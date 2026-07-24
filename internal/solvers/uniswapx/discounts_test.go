package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

const testDiscountID = "0x1111111111111111111111111111111111111111111111111111111111111111"

type fakeDiscountProvider struct {
	list     *liquiddiscounts.List
	resolved *liquiddiscounts.Resolved
	listErr  error

	listCalls int
}

func (f *fakeDiscountProvider) ListDiscounts(context.Context) (*liquiddiscounts.List, error) {
	f.listCalls++
	return f.list, f.listErr
}

func (f *fakeDiscountProvider) Resolve(context.Context, string) (*liquiddiscounts.Resolved, error) {
	return f.resolved, nil
}

func TestDiscountInventoriesUseConfiguredPhysicalRoute(t *testing.T) {
	now := time.Unix(1_000, 0)
	route := testDiscountRoute()
	policy, _ := tokenpolicy.New(tokenpolicy.All, nil)
	listed := &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
		testDiscountOffer(route, now.Add(time.Minute), "80", "100"),
	}}
	solver := &Solver{
		cfg:       &Config{Discounts: &DiscountConfig{HTTPTimeout: time.Second}, TokenPolicy: policy},
		discounts: &fakeDiscountProvider{list: listed}, log: logr.Discard(),
	}
	physical := liquidlane.DirectInventory(route, big.NewInt(100), big.NewInt(100))
	physical.AdapterMinDiscount = new(big.Int)
	inventory := solver.discountInventories(listed, []liquidlane.Inventory{physical}, now)
	if len(inventory) != 1 || inventory[0].DiscountID == nil || inventory[0].MaxAssets.Cmp(big.NewInt(80)) != 0 {
		t.Fatalf("discount inventory = %+v", inventory)
	}
}

func TestDiscountFillQuotesUseCurrentOracleAmount(t *testing.T) {
	now := time.Unix(1_000, 0)
	route := testDiscountRoute()
	policy, _ := tokenpolicy.New(tokenpolicy.All, nil)
	offer := testDiscountOffer(route, now.Add(time.Minute), "100", "2000000000000000000")
	offer.Discount = "100000"
	listed := &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{offer}}
	solver := &Solver{
		cfg:       &Config{Discounts: &DiscountConfig{HTTPTimeout: time.Second}, TokenPolicy: policy},
		discounts: &fakeDiscountProvider{list: listed},
		log:       logr.Discard(),
	}
	quotes := solver.discountFillQuotes(listed, []liquidlane.FillQuote{{
		Inventory: testInventoryWithMinDiscount(
			route, big.NewInt(100), big.NewInt(2_000_000_000_000_000_000), new(big.Int),
		),
		AmountIn:       big.NewInt(10),
		GrossAmountOut: big.NewInt(20),
		MaxAmountOut:   big.NewInt(20),
		MinDiscount:    new(big.Int),
	}}, now)
	if len(quotes) != 1 || quotes[0].MaxAmountOut.Cmp(big.NewInt(18)) != 0 || quotes[0].DiscountID == nil {
		t.Fatalf("discount quotes = %+v", quotes)
	}
}

func TestExternalModeNeverListsDiscounts(t *testing.T) {
	route := testDiscountRoute()
	provider := &fakeDiscountProvider{listErr: errors.New("must not be called")}
	solver := &Solver{
		cfg: &Config{
			SolverMode: solverModeExternal,
			Adapters:   []common.Address{route.Adapter},
		},
		discounts: provider,
		log:       logr.Discard(),
	}

	quoteRoutes, quoted, err := solver.quoteRoutesWithDiscounts(t.Context(), []liquidlane.Route{route}, time.Now())
	if err != nil || quoted != nil || len(quoteRoutes) != 1 {
		t.Fatalf("external quote routes/list/error = %+v/%+v/%v", quoteRoutes, quoted, err)
	}
	fillRoutes, filled, err := solver.fillRoutesWithDiscounts(
		t.Context(),
		[]liquidlane.Route{route},
		route.TokenIn,
		route.TokenOut,
		time.Now(),
	)
	if err != nil || filled != nil || len(fillRoutes) != 1 || provider.listCalls != 0 {
		t.Fatalf(
			"external fill routes/list/calls/error = %+v/%+v/%d/%v",
			fillRoutes,
			filled,
			provider.listCalls,
			err,
		)
	}
}

func TestResolveDiscountRevalidatesSelectedTerms(t *testing.T) {
	now := time.Unix(1_000, 0)
	route := testDiscountRoute()
	physical := []liquidlane.FillQuote{{
		Inventory: testInventoryWithMinDiscount(
			route, big.NewInt(100), big.NewInt(1_000_000_000_000_000_000), new(big.Int),
		),
		AmountIn: big.NewInt(100), GrossAmountOut: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		MinDiscount: new(big.Int),
	}}
	selected := liquiddiscounts.Selection{
		DiscountID:   common.HexToHash(testDiscountID),
		Adapter:      route.Adapter,
		TokenIn:      route.TokenIn,
		TokenOut:     route.TokenOut,
		AmountIn:     big.NewInt(100),
		MinAmountOut: big.NewInt(90),
	}
	validResolved := func() *liquiddiscounts.Resolved {
		deadline := now.Add(time.Minute).Unix()
		return &liquiddiscounts.Resolved{
			DiscountID: testDiscountID,
			Discount: liquiddiscounts.Terms{
				Adapter: route.Adapter.Hex(), TokenToRedeem: route.TokenIn.Hex(), Discount: "0",
				Signer:   common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
				Protocol: common.HexToAddress("0x6666666666666666666666666666666666666666").Hex(),
				Nonce:    "0x1", Deadline: deadline,
			},
			SignerSignature: "0x01", ProtocolDeadline: deadline, ProtocolSignature: "0x02",
		}
	}

	for name, mutate := range map[string]func(*liquiddiscounts.Resolved, *liquiddiscounts.Selection){
		"discount id": func(resolved *liquiddiscounts.Resolved, _ *liquiddiscounts.Selection) {
			resolved.DiscountID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"adapter": func(resolved *liquiddiscounts.Resolved, _ *liquiddiscounts.Selection) {
			resolved.Discount.Adapter = common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Hex()
		},
		"token": func(resolved *liquiddiscounts.Resolved, _ *liquiddiscounts.Selection) {
			resolved.Discount.TokenToRedeem = common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb").Hex()
		},
		"discount deadline": func(resolved *liquiddiscounts.Resolved, _ *liquiddiscounts.Selection) {
			resolved.Discount.Deadline = now.Add(15 * time.Second).Unix()
		},
		"protocol deadline": func(resolved *liquiddiscounts.Resolved, _ *liquiddiscounts.Selection) {
			resolved.ProtocolDeadline = now.Add(15 * time.Second).Unix()
		},
		"minimum output": func(_ *liquiddiscounts.Resolved, selection *liquiddiscounts.Selection) {
			selection.MinAmountOut = big.NewInt(101)
		},
		"adapter minimum discount": func(resolved *liquiddiscounts.Resolved, _ *liquiddiscounts.Selection) {
			resolved.Discount.Discount = "0"
			physical[0].MinDiscount = big.NewInt(1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			physical[0].MinDiscount = new(big.Int)
			resolved := validResolved()
			selection := selected
			mutate(resolved, &selection)
			solver := &Solver{
				cfg: &Config{Discounts: &DiscountConfig{
					HTTPTimeout: time.Second, MinimumValidity: 15 * time.Second,
				}},
				discounts: &fakeDiscountProvider{resolved: resolved}, log: logr.Discard(),
			}
			if _, err := solver.resolveDiscount(t.Context(), selection, physical, now); err == nil {
				t.Fatal("expected resolved discount rejection")
			}
		})
	}

	physical[0].MinDiscount = new(big.Int)
	solver := &Solver{
		cfg: &Config{Discounts: &DiscountConfig{
			HTTPTimeout: time.Second, MinimumValidity: 15 * time.Second,
		}},
		discounts: &fakeDiscountProvider{resolved: validResolved()}, log: logr.Discard(),
	}
	if _, err := solver.resolveDiscount(t.Context(), selected, physical, now); err != nil {
		t.Fatalf("valid resolved discount: %v", err)
	}
}

func testDiscountRoute() liquidlane.Route {
	return liquidlane.NewRoute(
		1,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		18,
		18,
	)
}

func testDiscountOffer(
	route liquidlane.Route,
	deadline time.Time,
	maxAssets string,
	maxRate string,
) liquiddiscounts.ListItem {
	return liquiddiscounts.ListItem{
		DiscountID: testDiscountID, Adapter: route.Adapter.Hex(), TokenToRedeem: route.TokenIn.Hex(),
		Collateral: route.TokenOut.Hex(), CollateralDecimals: route.TokenOutDecimals,
		Discount: "0", Signer: common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
		Deadline: deadline.Unix(), MaxRate: maxRate, MaxAssets: maxAssets,
	}
}

func testInventoryWithMinDiscount(
	route liquidlane.Route,
	maxAssets, maxRate, minDiscount *big.Int,
) liquidlane.Inventory {
	inventory := liquidlane.DirectInventory(route, maxAssets, maxRate)
	inventory.AdapterMinDiscount = liquidlane.CloneBig(minDiscount)
	return inventory
}
