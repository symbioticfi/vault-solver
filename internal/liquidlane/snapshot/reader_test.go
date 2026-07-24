package snapshot

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

type fakeLiquidReader struct {
	routes     []liquidlane.Route
	inventory  []liquidlane.Inventory
	quotes     []liquidlane.FillQuote
	authorized []liquidlane.Route
	gas        *liquidlanegas.Snapshot
}

func (f *fakeLiquidReader) ResolveRoutes(context.Context, []common.Address) ([]liquidlane.Route, error) {
	return f.routes, nil
}

func (f *fakeLiquidReader) ReadInventory(context.Context, []liquidlane.Route) ([]liquidlane.Inventory, error) {
	return f.inventory, nil
}

func (f *fakeLiquidReader) FilterAuthorized(
	_ context.Context,
	inventory []liquidlane.Inventory,
	_ common.Address,
) ([]liquidlane.Inventory, error) {
	allowed := make(map[liquidlane.RouteID]bool, len(f.authorized))
	for _, route := range f.authorized {
		allowed[route.ID] = true
	}
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if allowed[item.ID] {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeLiquidReader) ReadFillQuotes(
	context.Context,
	[]liquidlane.Route,
	common.Address,
	*big.Int,
) ([]liquidlane.FillQuote, error) {
	return f.quotes, nil
}

func (f *fakeLiquidReader) FilterAuthorizedRoutes(
	context.Context,
	[]liquidlane.Route,
	common.Address,
) ([]liquidlane.Route, error) {
	return f.authorized, nil
}

func (f *fakeLiquidReader) ReadGasSnapshot(context.Context, []liquidlane.Route) (*liquidlanegas.Snapshot, error) {
	return f.gas, nil
}

type fakeGasReader struct {
	tokens []liquidlanegas.Token
	prices *liquidlanegas.PriceSnapshot
}

func (f *fakeGasReader) ValidateTokens(tokens []liquidlanegas.Token) error {
	f.tokens = tokens
	return nil
}

func (f *fakeGasReader) Read(
	_ context.Context,
	tokens []liquidlanegas.Token,
	_ time.Time,
) (*liquidlanegas.PriceSnapshot, error) {
	f.tokens = tokens
	return f.prices, nil
}

func TestReaderBuildsQuoteAndFillSnapshots(t *testing.T) {
	t.Parallel()
	routeA := liquidlane.Route{ID: "a", TokenOut: common.HexToAddress("0xa"), TokenOutDecimals: 6}
	routeB := liquidlane.Route{ID: "b", TokenOut: common.HexToAddress("0xb"), TokenOutDecimals: 18}
	liquid := &fakeLiquidReader{
		routes: []liquidlane.Route{routeA, routeB}, authorized: []liquidlane.Route{routeB},
		inventory: []liquidlane.Inventory{{Route: routeA}, {Route: routeB}},
		quotes: []liquidlane.FillQuote{
			{Inventory: liquidlane.Inventory{Route: routeA}},
			{Inventory: liquidlane.Inventory{Route: routeB}},
		},
		gas: &liquidlanegas.Snapshot{},
	}
	gas := &fakeGasReader{prices: &liquidlanegas.PriceSnapshot{}}
	reader := newReader(liquid, gas)

	quote, err := reader.Quote(t.Context(), liquid.routes, common.Address{}, time.Now())
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(quote.Direct) != 1 || quote.Direct[0].ID != routeB.ID || len(quote.Physical) != 2 ||
		quote.GasSnapshot != liquid.gas || quote.GasPrices != gas.prices {
		t.Fatalf("quote snapshot = %#v", quote)
	}

	fill, err := reader.Fill(t.Context(), liquid.routes, common.Address{}, common.Address{}, big.NewInt(1), time.Now())
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if len(fill.Direct) != 1 || fill.Direct[0].ID != routeB.ID || len(fill.Physical) != 2 {
		t.Fatalf("fill snapshot = %#v", fill)
	}
	if len(gas.tokens) != 2 || gas.tokens[0].Address != routeA.TokenOut || gas.tokens[1].Decimals != 18 {
		t.Fatalf("gas tokens = %#v", gas.tokens)
	}
}
