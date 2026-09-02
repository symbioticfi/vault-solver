package lifi

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const testDiscountID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeDiscountClient struct {
	listed       *discounts.List
	resolved     *discounts.Resolved
	listCalls    int
	resolveCalls int
}

func (f *fakeDiscountClient) ListDiscounts(context.Context) (*discounts.List, error) {
	f.listCalls++
	return f.listed, nil
}

func (f *fakeDiscountClient) Resolve(context.Context, string) (*discounts.Resolved, error) {
	f.resolveCalls++
	return f.resolved, nil
}

func TestFillDiscountQuotesUsesFreshSignedTerms(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	directInventory := testDirectDiscountInventory()
	direct := liquidlane.FillQuote{
		Inventory: directInventory, AmountIn: big.NewInt(1_000),
		GrossAmountOut: big.NewInt(1_000), MaxAmountOut: big.NewInt(900), MinDiscount: big.NewInt(100_000),
	}
	fake := &fakeDiscountClient{
		listed: &discounts.List{Discounts: []discounts.ListItem{
			testDiscountListItem(directInventory, 1_000, now.Add(time.Minute)),
		}},
		resolved: testResolvedDiscount(directInventory, 100_000, now.Add(time.Minute)),
	}
	s := &Solver{discounts: fake, log: logr.Discard()}

	quotes, resolved := s.fillDiscountQuotes(context.Background(), []liquidlane.FillQuote{direct}, now)
	if len(quotes) != 1 || quotes[0].MaxAmountOut.String() != "900" ||
		!quotes[0].ValidUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("quotes = %+v", quotes)
	}
	id := common.HexToHash(testDiscountID)
	if resolved[id] == nil || fake.listCalls != 1 || fake.resolveCalls != 1 {
		t.Fatalf("resolved = %+v calls=%d/%d", resolved, fake.listCalls, fake.resolveCalls)
	}
}

func TestFillDiscountQuotesRejectsResolvedTermsBelowAdapterMinimum(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	directInventory := testDirectDiscountInventory()
	fake := &fakeDiscountClient{
		listed: &discounts.List{Discounts: []discounts.ListItem{
			testDiscountListItem(directInventory, 1_000, now.Add(time.Minute)),
		}},
		resolved: testResolvedDiscount(directInventory, 50_000, now.Add(time.Minute)),
	}
	s := &Solver{discounts: fake, log: logr.Discard()}
	direct := liquidlane.FillQuote{
		Inventory: directInventory, AmountIn: big.NewInt(1_000),
		GrossAmountOut: big.NewInt(1_000), MaxAmountOut: big.NewInt(900), MinDiscount: big.NewInt(100_000),
	}

	quotes, resolved := s.fillDiscountQuotes(context.Background(), []liquidlane.FillQuote{direct}, now)
	if len(quotes) != 0 || len(resolved) != 0 {
		t.Fatalf("unsafe discount survived: quotes=%+v resolved=%+v", quotes, resolved)
	}
}

func TestRefreshResolvedDiscountQuotesUsesFreshAdapterState(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	direct := testDirectDiscountInventory()
	id := common.HexToHash(testDiscountID)
	candidateInventory := liquidlane.DiscountInventory(
		direct.Route, big.NewInt(1_000), direct.MaxRate, id, now.Add(time.Minute),
	)
	candidateInventory.AdapterMinDiscount = big.NewInt(100_000)
	candidate := liquidlane.FillQuote{
		Inventory: candidateInventory,
		AmountIn:  big.NewInt(1_000), GrossAmountOut: big.NewInt(1_000), MaxAmountOut: big.NewInt(900),
		MinDiscount: big.NewInt(100_000),
	}
	fresh := liquidlane.FillQuote{
		Inventory: direct, AmountIn: big.NewInt(800), GrossAmountOut: big.NewInt(880),
		MaxAmountOut: big.NewInt(792), MinDiscount: big.NewInt(100_000),
	}
	fresh.MaxAssets = big.NewInt(700)
	signed, err := discounts.ParseSigned(testResolvedDiscount(direct, 100_000, now.Add(time.Minute)))
	if err != nil {
		t.Fatalf("ParseSigned: %v", err)
	}

	got, issues := discounts.RefreshFillQuotes(
		[]liquidlane.FillQuote{candidate}, map[common.Hash]*discounts.Signed{id: signed},
		[]liquidlane.FillQuote{fresh}, now,
	)
	if len(issues) != 0 || len(got) != 1 {
		t.Fatalf("quotes = %+v issues = %+v", got, issues)
	}
	if got[0].AmountIn.String() != "800" || got[0].GrossAmountOut.String() != "880" ||
		got[0].MaxAmountOut.String() != "792" || got[0].MaxAssets.String() != "700" {
		t.Fatalf("refreshed quote = %+v", got[0])
	}
}

func TestRefreshResolvedDiscountQuotesRejectsRateAboveFreshAdapterLimit(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	direct := testDirectDiscountInventory()
	id := common.HexToHash(testDiscountID)
	candidateInventory := liquidlane.DiscountInventory(
		direct.Route, big.NewInt(1_000), direct.MaxRate, id, now.Add(time.Minute),
	)
	candidateInventory.AdapterMinDiscount = big.NewInt(100_000)
	candidate := liquidlane.FillQuote{
		Inventory: candidateInventory,
		AmountIn:  big.NewInt(1_000), GrossAmountOut: big.NewInt(1_000), MaxAmountOut: big.NewInt(900),
		MinDiscount: big.NewInt(100_000),
	}
	fresh := candidate
	fresh.Inventory = direct
	fresh.MaxRate = big.NewInt(800_000_000_000_000_000)
	signed, err := discounts.ParseSigned(testResolvedDiscount(direct, 100_000, now.Add(time.Minute)))
	if err != nil {
		t.Fatalf("ParseSigned: %v", err)
	}

	got, _ := discounts.RefreshFillQuotes(
		[]liquidlane.FillQuote{candidate}, map[common.Hash]*discounts.Signed{id: signed},
		[]liquidlane.FillQuote{fresh}, now,
	)
	if len(got) != 0 {
		t.Fatalf("unsafe quote survived: %+v", got)
	}
}

func testDirectDiscountInventory() liquidlane.Inventory {
	routeItem := liquidlane.NewRoute(
		11155111,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		6,
		6,
	)
	inventory := liquidlane.DirectInventory(routeItem, big.NewInt(1_000), big.NewInt(900_000_000_000_000_000))
	inventory.AdapterMinDiscount = big.NewInt(100_000)
	return inventory
}

func testDiscountListItem(
	direct liquidlane.Inventory,
	maxAssets int64,
	deadline time.Time,
) discounts.ListItem {
	return discounts.ListItem{
		DiscountID: testDiscountID,
		Adapter:    direct.Adapter.Hex(), TokenToRedeem: direct.TokenIn.Hex(), Collateral: direct.TokenOut.Hex(),
		CollateralDecimals: direct.TokenOutDecimals, Deadline: deadline.Unix(),
		Discount: "100000",
		MaxRate:  direct.MaxRate.String(), MaxAssets: big.NewInt(maxAssets).String(),
	}
}

func testResolvedDiscount(
	direct liquidlane.Inventory,
	discount int64,
	deadline time.Time,
) *discounts.Resolved {
	return &discounts.Resolved{
		DiscountID: testDiscountID,
		Discount: discounts.Terms{
			Adapter: direct.Adapter.Hex(), TokenToRedeem: direct.TokenIn.Hex(), Discount: big.NewInt(discount).String(),
			Signer:   common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
			Protocol: common.HexToAddress("0x6666666666666666666666666666666666666666").Hex(),
			Nonce:    "1", Deadline: deadline.Unix(),
		},
		SignerSignature: "0x1234", ProtocolDeadline: deadline.Unix(), ProtocolSignature: "0x5678",
	}
}
