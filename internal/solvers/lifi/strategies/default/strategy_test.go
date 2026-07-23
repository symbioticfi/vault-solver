package defaultstrategy

import (
	"context"
	"encoding/binary"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func TestDecideQuotesRequiresSolverExpiry(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err = strategy.DecideQuotes(context.Background(), types.QuoteInput{
		ChainTime: time.Unix(1_800_000_000, 0),
	}); err == nil {
		t.Fatal("expected missing quote expiry error")
	}
}

func TestDefaultExecutionBufferIsOneBlock(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if strategy.executionBuffer != 12*time.Second {
		t.Fatalf("execution buffer = %s", strategy.executionBuffer)
	}
}

func TestDefaultQuoteRangeCount(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if strategy.rangeCount != defaultRangeCount {
		t.Fatalf("range count = %d, want %d", strategy.rangeCount, defaultRangeCount)
	}
}

func TestQuoteRangeCountValidation(t *testing.T) {
	for _, value := range []int{-1, types.MaxQuoteRanges + 1} {
		if _, err := New(Config{RangeCount: value}); err == nil {
			t.Fatalf("rangeCount %d: expected error", value)
		}
	}
}

func TestDecideQuotesAppliesBuffersAndCapacity(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{PriceBufferBps: 100, MinAmount: "1000"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Solver:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ChainTime:      time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_300, 0),
		MaxFeePerGas:   big.NewInt(0),
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID:               "route-1",
				Adapter:          common.HexToAddress("0x2222222222222222222222222222222222222222"),
				TokenIn:          common.HexToAddress("0x3333333333333333333333333333333333333333"),
				TokenOut:         common.HexToAddress("0x4444444444444444444444444444444444444444"),
				TokenInDecimals:  6,
				TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(990_000_000),
			MaxRate:   big.NewInt(1_000_000_000_000_000_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes len = %d", len(out.Quotes))
	}
	q := out.Quotes[0]
	if q.Expiry != 1_800_000_300 {
		t.Fatalf("expiry = %d", q.Expiry)
	}
	if got, ok := new(big.Rat).SetString(q.Ranges[0].Quote); !ok ||
		got.Sign() <= 0 || got.Cmp(big.NewRat(98, 100)) > 0 {
		t.Fatalf("quote = %q, want positive rate no greater than buffered 0.98", q.Ranges[0].Quote)
	}
	if got := q.Ranges[0].MinAmount.String(); got != "1000" {
		t.Fatalf("minAmount = %s", got)
	}
	if got := q.Ranges[len(q.Ranges)-1].MaxAmount.String(); got != "990000000" {
		t.Fatalf("maxAmount = %s", got)
	}
}

func TestDecideQuotesChargesGasAfterBuildingRange(t *testing.T) {
	cfg := testStrategyConfig(Config{MinAmount: "1000"})
	strategy, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Solver:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ChainTime:      time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_090, 0),
		MaxFeePerGas:   big.NewInt(100),
		GasPrices:      testGasPrices(common.HexToAddress("0x4444444444444444444444444444444444444444"), 1_000_000_000_000),
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID:               "route-1",
				Adapter:          common.HexToAddress("0x2222222222222222222222222222222222222222"),
				TokenIn:          common.HexToAddress("0x3333333333333333333333333333333333333333"),
				TokenOut:         common.HexToAddress("0x4444444444444444444444444444444444444444"),
				TokenInDecimals:  6,
				TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(20_000),
			MaxRate:   big.NewInt(1_000_000_000_000_000_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes len = %d", len(out.Quotes))
	}
	ranges := out.Quotes[0].Ranges
	if len(ranges) <= 1 {
		t.Fatalf("expected dynamic ranges, got %d", len(ranges))
	}
	if got := ranges[0].MinAmount.String(); got != "1000" {
		t.Fatalf("range[0].min = %s", got)
	}
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "20000" {
		t.Fatalf("last range max = %s", got)
	}
	for _, quoteRange := range ranges {
		rate, ok := new(big.Rat).SetString(quoteRange.Quote)
		if !ok || rate.Sign() <= 0 || rate.Cmp(big.NewRat(1, 1)) >= 0 {
			t.Fatalf("quote should deduct complete-plan gas: %#v", ranges)
		}
	}
}

func TestDecideQuotesAllowsBreakEvenMinimum(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "1"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID:      "route-1",
				TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenOut: common.HexToAddress("0x2222222222222222222222222222222222222222"),
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(100), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}},
		MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 || out.Quotes[0].Ranges[0].Quote != "1" {
		t.Fatalf("quotes = %+v, want break-even range", out.Quotes)
	}
}

func TestDecideQuotesRaisesMinimumAboveGasBreakEven(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "1"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID:      "route-1",
				TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(10_000_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}},
		GasPrices: testGasPrices(tokenOut, 1_000_000_000_000_000_000), MaxFeePerGas: big.NewInt(1),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 || len(out.Quotes[0].Ranges) == 0 {
		t.Fatalf("quotes = %+v, want gas-aware range", out.Quotes)
	}
	if out.Quotes[0].Ranges[0].MinAmount.Cmp(big.NewInt(1)) <= 0 {
		t.Fatalf("minAmount = %s, want amount above gas break-even", out.Quotes[0].Ranges[0].MinAmount)
	}
}

func TestDecideQuotesBoundsGasTransitionInsideRange(t *testing.T) {
	cfg := testStrategyConfig(Config{MinAmount: "900"})
	strategy, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x4444444444444444444444444444444444444444")
	adapter := common.HexToAddress("0x2222222222222222222222222222222222222222")
	vault := common.HexToAddress("0x3333333333333333333333333333333333333333")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter, Vault: vault,
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	gasSnapshot := &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{tokenIn: big.NewInt(1_000)}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vault: {FreeAssets: big.NewInt(10_000), Withdrawable: big.NewInt(10_000)},
		},
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{{
			Route: route, MaxAssets: big.NewInt(2_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}},
		GasSnapshot: gasSnapshot, GasPrices: testGasPrices(tokenOut, 1_000_000), MaxFeePerGas: big.NewInt(1_000_000_000),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 || len(out.Quotes[0].Ranges) == 0 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	pricing, err := newGasPricing(
		big.NewInt(1_000_000_000), tokenOut, testGasPrices(tokenOut, 1_000_000), gasSnapshot, 0,
	)
	if err != nil {
		t.Fatalf("pricing: %v", err)
	}
	for _, quoteRange := range out.Quotes[0].Ranges {
		rate, ok := new(big.Rat).SetString(quoteRange.Quote)
		if !ok {
			t.Fatalf("invalid quote rate %q", quoteRange.Quote)
		}
		for amount := quoteRange.MinAmount.Int64(); amount <= quoteRange.MaxAmount.Int64(); amount++ {
			actual := new(big.Int).Sub(big.NewInt(amount), pricing.cost([]gasLeg{{
				route: route, amountOut: big.NewInt(amount),
			}}))
			quoted := new(big.Int).Mul(big.NewInt(amount), rate.Num())
			quoted.Div(quoted, rate.Denom())
			if quoted.Cmp(actual) > 0 {
				t.Fatalf("amount %d quoted %s above executable %s in range %+v", amount, quoted, actual, quoteRange)
			}
		}
	}
}

func TestDecideQuotesUsesConfiguredRangeCount(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "100", RangeCount: 4}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID: "route-1", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}},
		MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	ranges := out.Quotes[0].Ranges
	if len(ranges) != 4 {
		t.Fatalf("ranges = %+v, want four configured ranges", ranges)
	}
	for i := range ranges {
		if i > 0 {
			wantMin := new(big.Int).Add(ranges[i-1].MaxAmount, big.NewInt(1))
			if ranges[i].MinAmount.Cmp(wantMin) != 0 {
				t.Fatalf("range[%d].min = %s, want %s", i, ranges[i].MinAmount, wantMin)
			}
		}
	}
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "1000" {
		t.Fatalf("last maxAmount = %s, want 1000", got)
	}
}

func TestDecideQuotesUsesAtMostThreePhysicalRoutes(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := make([]liquidlane.Inventory, 4)
	for i := range inventory {
		inventory[i] = liquidlane.Inventory{
			Route: liquidlane.Route{
				ID:         liquidlane.RouteID("route-" + strconv.Itoa(i+1)),
				CapacityID: liquidlane.CapacityID("capacity-" + strconv.Itoa(i+1)),
				Adapter:    common.BytesToAddress([]byte{byte(i + 1)}),
				TokenIn:    tokenIn, TokenOut: tokenOut, TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(100), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: inventory, MaxFeePerGas: big.NewInt(0),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	ranges := out.Quotes[0].Ranges
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "300" {
		t.Fatalf("maxAmount = %s, want three-route capacity 300", got)
	}
}

func TestDecideQuotesAggregatesIndependentRoutesIntoOnePairCurve(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := []liquidlane.Inventory{
		{
			Route: liquidlane.Route{
				ID: "route-1", CapacityID: "capacity-1", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(500), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
		{
			Route: liquidlane.Route{
				ID: "route-2", CapacityID: "capacity-2", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(500), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory:    inventory,
		MaxFeePerGas: big.NewInt(0),
		ChainTime:    now, ServerTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	ranges := out.Quotes[0].Ranges
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "1000" {
		t.Fatalf("aggregate maxAmount = %s, want 1000", got)
	}
}

func TestDecideQuotesPermissionedTokenUsesOneRoute(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy, err := New(testStrategyConfig(Config{MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	inventory := []liquidlane.Inventory{
		{
			Route: liquidlane.Route{
				ID: "route-1", CapacityID: "capacity-1", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(500), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
		{
			Route: liquidlane.Route{
				ID: "route-2", CapacityID: "capacity-2", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(500), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: inventory, SingleRouteTokens: map[common.Address]bool{tokenIn: true},
		MaxFeePerGas: big.NewInt(0),
		ChainTime:    now, ServerTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	ranges := out.Quotes[0].Ranges
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "500" {
		t.Fatalf("permissioned maxAmount = %s, want 500", got)
	}
}

func TestDecideQuotesNeverOverquotesBlendedRouteRange(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := []liquidlane.Inventory{
		{
			Route: liquidlane.Route{
				ID: "route-fast", CapacityID: "capacity-fast", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(200), MaxRate: big.NewInt(2_000_000_000_000_000_000),
		},
		{
			Route: liquidlane.Route{
				ID: "route-slow", CapacityID: "capacity-slow", TokenIn: tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(100), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: inventory, MaxFeePerGas: big.NewInt(0),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	for _, quoteRange := range out.Quotes[0].Ranges {
		rate, ok := new(big.Rat).SetString(quoteRange.Quote)
		if !ok {
			t.Fatalf("invalid quote rate %q", quoteRange.Quote)
		}
		for amount := quoteRange.MinAmount.Int64(); amount <= quoteRange.MaxAmount.Int64(); amount++ {
			fastInput := min(amount, int64(100))
			actualOut := 2*fastInput + max(amount-fastInput, int64(0))
			quoted := new(big.Int).Mul(big.NewInt(amount), rate.Num())
			quoted.Div(quoted, rate.Denom())
			if quoted.Cmp(big.NewInt(actualOut)) > 0 {
				t.Fatalf("amount %d quoted %s above executable %d in range %+v", amount, quoted, actualOut, quoteRange)
			}
		}
	}
}

func TestDecideQuotesUsesPrivateAlternativeBeforeDirectFallback(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "100"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", TokenIn: tokenIn, TokenOut: tokenOut,
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	inventory := []liquidlane.Inventory{
		liquidlane.DirectInventory(route, big.NewInt(1_000), big.NewInt(900_000_000_000_000_000)),
		liquidlane.DiscountInventory(
			route, big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000),
			discountID, now.Add(time.Minute),
		),
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: inventory, MaxFeePerGas: big.NewInt(0),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 || out.Quotes[0].Expiry != now.Add(48*time.Second).Unix() {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	usedPrivateRate := false
	for _, quoteRange := range out.Quotes[0].Ranges {
		rate, ok := new(big.Rat).SetString(quoteRange.Quote)
		if ok && rate.Cmp(big.NewRat(9, 10)) > 0 {
			usedPrivateRate = true
			break
		}
	}
	if !usedPrivateRate {
		t.Fatalf("private alternative did not improve any range: %+v", out.Quotes[0].Ranges)
	}
}

func TestPriceBufferCoversQuoteToFillAndExecutionWindows(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{PriceBufferBps: 100, MinAmount: "10000"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	route := liquidlane.Route{
		ID: "route-1", Adapter: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	quotes, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{{
			Route: route, MaxAssets: big.NewInt(20_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}},
		MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(quotes.Quotes) != 1 {
		t.Fatalf("quote = %+v", quotes.Quotes)
	}
	quoteRate, ok := new(big.Rat).SetString(quotes.Quotes[0].Ranges[0].Quote)
	if !ok || quoteRate.Sign() <= 0 || quoteRate.Cmp(big.NewRat(98, 100)) > 0 {
		t.Fatalf("quote rate = %q, want positive rate no greater than 0.98", quotes.Quotes[0].Ranges[0].Quote)
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(10_000), OutputAmount: big.NewInt(9_800), ChainTime: now,
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(20_000)},
			AmountIn:  big.NewInt(10_000), MaxAmountOut: big.NewInt(9_900),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil {
		t.Fatal("expected fill after one price-buffer adverse move")
	}
}

func TestDecideFillBuildsMultiRoutePlan(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	quotes := make([]liquidlane.FillQuote, 0, 2)
	for i, routeID := range []liquidlane.RouteID{"route-1", "route-2"} {
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{
					ID: routeID, CapacityID: liquidlane.CapacityID("capacity-" + strconv.Itoa(i+1)),
					Adapter: common.BytesToAddress([]byte{byte(i + 1)}), TokenIn: tokenIn, TokenOut: tokenOut,
				},
				MaxAssets: big.NewInt(500),
			},
			AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(1_000),
		})
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900), ChainTime: time.Unix(1_800_000_000, 0),
		MaxFeePerGas: big.NewInt(0), Quotes: quotes,
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil || len(plan.Routes) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	amountIn := new(big.Int)
	minimumOut := new(big.Int)
	for _, route := range plan.Routes {
		amountIn.Add(amountIn, route.AmountIn)
		minimumOut.Add(minimumOut, route.MinAmountOut)
	}
	if amountIn.String() != "1000" || minimumOut.String() != "900" {
		t.Fatalf("amountIn=%s minimumOut=%s", amountIn, minimumOut)
	}
}

func TestDecideFillDoesNotDoubleSpendSharedVaultCapacity(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	quotes := make([]liquidlane.FillQuote, 0, 2)
	for i, routeID := range []liquidlane.RouteID{"route-1", "route-2"} {
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{
					ID: routeID, CapacityID: "shared-capacity",
					Adapter: common.BytesToAddress([]byte{byte(i + 1)}), TokenIn: tokenIn, TokenOut: tokenOut,
				},
				MaxAssets: big.NewInt(600),
			},
			AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(1_000),
		})
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900), ChainTime: time.Unix(1_800_000_000, 0),
		MaxFeePerGas: big.NewInt(0), Quotes: quotes,
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan != nil {
		t.Fatalf("shared capacity was double counted: %+v", plan)
	}
}

func TestDecideQuotesUsesPrivateDiscountWithoutDirectCandidateAndClipsExpiry(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "1000"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1",
		TokenIn:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		TokenOut:        common.HexToAddress("0x2222222222222222222222222222222222222222"),
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	discount := liquidlane.DiscountInventory(
		route, big.NewInt(900), big.NewInt(800_000_000_000_000_000), discountID, now.Add(time.Minute),
	)

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory:    []liquidlane.Inventory{discount},
		MaxFeePerGas: big.NewInt(0), ChainTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	quote := out.Quotes[0]
	last := quote.Ranges[len(quote.Ranges)-1]
	lastRate, ok := new(big.Rat).SetString(last.Quote)
	if quote.Expiry != now.Add(48*time.Second).Unix() || last.MaxAmount.String() != "1125" ||
		!ok || lastRate.Sign() <= 0 || lastRate.Cmp(big.NewRat(8, 10)) > 0 {
		t.Fatalf("discount quote = %+v", quote)
	}
}

func TestDecideQuotesPublishesOnePairForDirectAndDiscount(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	route := liquidlane.Route{
		ID: "route-1", TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		TokenOut:        common.HexToAddress("0x2222222222222222222222222222222222222222"),
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	direct := liquidlane.DirectInventory(
		route, big.NewInt(1_000), big.NewInt(900_000_000_000_000_000),
	)
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	discount := liquidlane.DiscountInventory(
		route, big.NewInt(1_000), big.NewInt(800_000_000_000_000_000), discountID, now.Add(time.Minute),
	)

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{direct, discount}, MaxFeePerGas: big.NewInt(0),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
}

func TestDecideQuotesSkipsBelowMinAmount(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "1000001"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Solver:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ChainTime:      time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_090, 0),
		MaxFeePerGas:   big.NewInt(0),
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID:               "route-1",
				TokenIn:          common.HexToAddress("0x3333333333333333333333333333333333333333"),
				TokenOut:         common.HexToAddress("0x4444444444444444444444444444444444444444"),
				TokenInDecimals:  6,
				TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(1_000_000),
			MaxRate:   big.NewInt(1_000_000_000_000_000_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 0 {
		t.Fatalf("quotes len = %d", len(out.Quotes))
	}
}

func TestDecideFillSelectsProfitableRoute(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "1000"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn:      tokenIn,
		TokenOut:     tokenOut,
		AmountIn:     big.NewInt(1_000_000),
		OutputAmount: big.NewInt(990_000),
		ChainTime:    time.Unix(1_800_000_000, 0),
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{
				Route:     liquidlane.Route{ID: "route-1", Adapter: adapter, TokenIn: tokenIn, TokenOut: tokenOut},
				MaxAssets: big.NewInt(2_000_000),
			},
			AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil {
		t.Fatal("expected fill plan")
	}
	if len(plan.Routes) != 1 || plan.Routes[0].Adapter != adapter {
		t.Fatalf("routes = %+v", plan.Routes)
	}
	if plan.Routes[0].ExpectedAmountOut.String() != "1000000" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDecideFillPermissionedTokenNeverAggregatesRoutes(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	quotes := []liquidlane.FillQuote{
		{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{
					ID: "route-1", CapacityID: "capacity-1",
					TokenIn: tokenIn, TokenOut: tokenOut,
				},
				MaxAssets: big.NewInt(500),
			},
			AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(1_000),
		},
		{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{
					ID: "route-2", CapacityID: "capacity-2",
					TokenIn: tokenIn, TokenOut: tokenOut,
				},
				MaxAssets: big.NewInt(500),
			},
			AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(1_000),
		},
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900),
		RequireSingleRoute: true,
		ChainTime:          time.Unix(1_800_000_000, 0), MaxFeePerGas: big.NewInt(0), Quotes: quotes,
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan != nil {
		t.Fatalf("permissioned token must not aggregate routes, got %+v", plan)
	}
}

func TestDecideFillSelectsBestRouteInsteadOfConfigOrder(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	firstAdapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	bestAdapter := common.HexToAddress("0x4444444444444444444444444444444444444444")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(990_000),
		ChainTime:    time.Unix(1_800_000_000, 0),
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{
			{
				Inventory: liquidlane.Inventory{
					Route:     liquidlane.Route{ID: "route-1", Adapter: firstAdapter, TokenIn: tokenIn, TokenOut: tokenOut},
					MaxAssets: big.NewInt(2_000_000),
				},
				AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
			},
			{
				Inventory: liquidlane.Inventory{
					Route:     liquidlane.Route{ID: "route-2", Adapter: bestAdapter, TokenIn: tokenIn, TokenOut: tokenOut},
					MaxAssets: big.NewInt(2_000_000),
				},
				AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_100_000),
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil || len(plan.Routes) != 1 || plan.Routes[0].Adapter != bestAdapter {
		t.Fatalf("plan = %+v, want adapter %s", plan, bestAdapter)
	}
}

func TestDecideFillCommitsSelectedPrivateDiscount(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	route := liquidlane.Route{ID: "route-1", Adapter: adapter, TokenIn: tokenIn, TokenOut: tokenOut}

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(850), ChainTime: time.Unix(1_800_000_000, 0),
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{
			{Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(1_000)}, AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(900)},
			{
				Inventory: liquidlane.Inventory{
					Route: route, MaxAssets: big.NewInt(1_000),
					DiscountID: &discountID,
				},
				AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(950), MinDiscount: big.NewInt(100_000),
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil || len(plan.Routes) != 1 || plan.Routes[0].DiscountID == nil ||
		*plan.Routes[0].DiscountID != discountID {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDecideFillChargesPrivateExecutionGasAfterGreedySelection(t *testing.T) {
	cfg := testStrategyConfig(Config{})
	strategy, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	vault := common.HexToAddress("0x4444444444444444444444444444444444444444")
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	route := liquidlane.Route{ID: "route-1", Adapter: adapter, Vault: vault, TokenIn: tokenIn, TokenOut: tokenOut}

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900_000), ChainTime: time.Unix(1_800_000_000, 0),
		MaxFeePerGas: big.NewInt(1),
		GasPrices:    testGasPrices(tokenOut, 1_000_000_000_000_000_000),
		GasSnapshot: &liquidlanegas.Snapshot{
			Adapters: map[common.Address]*liquidlanegas.AdapterState{
				adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{tokenIn: big.NewInt(3_000_000)}},
			},
			Vaults: map[common.Address]*liquidlanegas.VaultState{
				vault: {FreeAssets: new(big.Int), Withdrawable: new(big.Int)},
			},
		},
		Quotes: []liquidlane.FillQuote{
			{
				Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(3_000_000)},
				AmountIn:  big.NewInt(1_000), MaxAmountOut: big.NewInt(1_800_000),
			},
			{
				Inventory: liquidlane.Inventory{
					Route: route, MaxAssets: big.NewInt(3_000_000),
					DiscountID: &discountID,
				},
				AmountIn: big.NewInt(1_000), MaxAmountOut: big.NewInt(1_800_050),
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil || len(plan.Routes) != 1 || plan.Routes[0].DiscountID == nil {
		t.Fatalf("plan = %+v, want higher-rate private route", plan)
	}
	if plan.Routes[0].MinAmountOut.Cmp(big.NewInt(900_000)) <= 0 {
		t.Fatalf("minAmountOut = %s, want order output plus complete-plan gas", plan.Routes[0].MinAmountOut)
	}
}

func TestDecideFillPrivateCapacityIncludesUpwardPriceBuffer(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{PriceBufferBps: 100}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	for _, tt := range []struct {
		name         string
		maxAmountOut int64
		wantFill     bool
	}{
		{name: "buffer fits", maxAmountOut: 9_900, wantFill: true},
		{name: "buffer exceeds capacity", maxAmountOut: 9_901, wantFill: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan, fillErr := strategy.DecideFill(context.Background(), types.FillInput{
				TokenIn: tokenIn, TokenOut: tokenOut,
				AmountIn: big.NewInt(10_000), OutputAmount: big.NewInt(9_500), ChainTime: now,
				MaxFeePerGas: big.NewInt(0),
				Quotes: []liquidlane.FillQuote{{
					Inventory: liquidlane.Inventory{
						Route: liquidlane.Route{
							ID: "route-1", Adapter: common.HexToAddress("0x3333333333333333333333333333333333333333"),
							TokenIn: tokenIn, TokenOut: tokenOut,
						},
						MaxAssets:  big.NewInt(10_000),
						DiscountID: &discountID, ValidUntil: now.Add(time.Minute),
					},
					AmountIn:     big.NewInt(10_000),
					MaxAmountOut: big.NewInt(tt.maxAmountOut),
					MinDiscount:  big.NewInt(100_000),
				}},
			})
			if fillErr != nil {
				t.Fatalf("DecideFill: %v", fillErr)
			}
			if (plan != nil) != tt.wantFill {
				t.Fatalf("plan = %+v, wantFill = %v", plan, tt.wantFill)
			}
			if tt.wantFill && plan.Routes[0].ReservedAmountOut.String() != "9999" {
				t.Fatalf("private reservation = %s, want 9999", plan.Routes[0].ReservedAmountOut)
			}
		})
	}
}

func TestDecideFillSubtractsPendingCapacityReservations(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1",
		Adapter:  common.HexToAddress("0x3333333333333333333333333333333333333333"),
		TokenIn:  tokenIn,
		TokenOut: tokenOut,
	}

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn:      tokenIn,
		TokenOut:     tokenOut,
		AmountIn:     big.NewInt(100),
		OutputAmount: big.NewInt(90),
		MaxFeePerGas: big.NewInt(0),
		Reservations: map[liquidlane.CapacityID]*big.Int{"capacity-1": big.NewInt(60)},
		Quotes: []liquidlane.FillQuote{{
			Inventory:    liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(100)},
			AmountIn:     big.NewInt(100),
			MaxAmountOut: big.NewInt(100),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan != nil {
		t.Fatalf("plan = %+v, want pending reservation to leave insufficient capacity", plan)
	}
}

func TestDecideFillRequiresExecutionDeadlineBuffer(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{ExecutionDeadlineBuffer: "30s"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900), ChainTime: now,
		Expires: uint32(now.Add(30 * time.Second).Unix()), FillDeadline: uint32(now.Add(time.Minute).Unix()),
		MaxFeePerGas: big.NewInt(0), Quotes: profitableFillQuotes(tokenIn, tokenOut),
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan != nil {
		t.Fatalf("expected near-expiry order to be skipped, got %+v", plan)
	}
}

func TestDecideQuotesKeepsInventoryReserve(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{InventoryReserveBps: 1_000, MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	routeItem := liquidlane.Inventory{
		Route: liquidlane.Route{
			ID: "route-1", TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"),
			TokenOut:        common.HexToAddress("0x2222222222222222222222222222222222222222"),
			TokenInDecimals: 6, TokenOutDecimals: 6,
		},
		MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{routeItem}, MaxFeePerGas: big.NewInt(0),
		ChainTime: time.Unix(1_800_000_000, 0), QuoteExpiresAt: time.Unix(1_800_000_090, 0),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %d", len(out.Quotes))
	}
	ranges := out.Quotes[0].Ranges
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "900" {
		t.Fatalf("reserved maxAmount = %s, want 900", got)
	}
}

func TestDecideQuotesAppliesReserveBeforeInFlightReservations(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{InventoryReserveBps: 1_000, MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	routeItem := liquidlane.Inventory{
		Route: liquidlane.Route{
			ID: "route-1", CapacityID: "capacity-1",
			TokenIn:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
			TokenOut:        common.HexToAddress("0x2222222222222222222222222222222222222222"),
			TokenInDecimals: 6, TokenOutDecimals: 6,
		},
		MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{routeItem}, MaxFeePerGas: big.NewInt(0),
		Reservations:   map[liquidlane.CapacityID]*big.Int{"capacity-1": big.NewInt(800)},
		ChainTime:      time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_090, 0),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %d", len(out.Quotes))
	}
	if got := out.Quotes[0].Ranges[len(out.Quotes[0].Ranges)-1].MaxAmount.String(); got != "100" {
		t.Fatalf("maxAmount = %s, want reserve-first capacity 100", got)
	}
}

func TestDecideQuotesSharesOneCapacityDomainAcrossRoutes(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := []liquidlane.Inventory{
		{
			Route: liquidlane.Route{
				ID: "route-1", CapacityID: "capacity-1",
				TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
		{
			Route: liquidlane.Route{
				ID: "route-2", CapacityID: "capacity-1",
				TokenIn: common.HexToAddress("0x3333333333333333333333333333333333333333"), TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		},
	}
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory:      inventory,
		MaxFeePerGas:   big.NewInt(0),
		ChainTime:      time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_090, 0),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 2 {
		t.Fatalf("quotes = %d", len(out.Quotes))
	}
	total := new(big.Int)
	for _, quote := range out.Quotes {
		total.Add(total, quote.Ranges[len(quote.Ranges)-1].MaxAmount)
	}
	if total.String() != "1000" {
		t.Fatalf("total quoted capacity = %s, want 1000", total)
	}
}

func TestDecideFillSeparatesBufferedTargetFromEconomicFloor(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{PriceBufferBps: 100}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut,
		AmountIn: big.NewInt(10_000), OutputAmount: big.NewInt(9_600),
		MaxFeePerGas: big.NewInt(100),
		GasPrices:    testGasPrices(tokenOut, 1),
		ChainTime:    time.Unix(1_800_000_000, 0),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{
					ID: "route-1", CapacityID: "capacity-1",
					Adapter: common.HexToAddress("0x3333333333333333333333333333333333333333"),
					TokenIn: tokenIn, TokenOut: tokenOut,
				},
				MaxAssets: big.NewInt(20_000),
			},
			AmountIn: big.NewInt(10_000), MaxAmountOut: big.NewInt(10_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil || len(plan.Routes) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Routes[0].ExpectedAmountOut.String() != "9900" ||
		plan.Routes[0].MinAmountOut.String() != "9601" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDecideFillRejectsDutchAuctionContext(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, outputContext := range [][]byte{{dutchAuctionContextType}, {exclusiveDutchAuctionContextType}} {
		plan, decideErr := strategy.DecideFill(context.Background(), types.FillInput{
			AmountIn:      big.NewInt(1_000_000),
			OutputAmount:  big.NewInt(990_000),
			OutputContext: outputContext,
		})
		if decideErr == nil || !strings.Contains(decideErr.Error(), "Dutch auctions are not supported") {
			t.Fatalf("DecideFill(context=%x) error = %v", outputContext, decideErr)
		}
		if plan != nil {
			t.Fatalf("DecideFill(context=%x) plan = %+v", outputContext, plan)
		}
	}
}

func TestDecideFillRespectsExclusiveWindow(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	solver := common.HexToAddress("0x5555555555555555555555555555555555555555")
	otherSolver := common.HexToAddress("0x6666666666666666666666666666666666666666")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		Solver:        solver,
		TokenIn:       tokenIn,
		TokenOut:      tokenOut,
		AmountIn:      big.NewInt(1_000_000),
		OutputAmount:  big.NewInt(990_000),
		OutputContext: exclusiveLimitContext(otherSolver),
		ChainTime:     time.Unix(1_800_000_000, 0),
		MaxFeePerGas:  big.NewInt(0),
		Quotes:        profitableFillQuotes(tokenIn, tokenOut),
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan != nil {
		t.Fatalf("expected skip during another solver's exclusive window, got %+v", plan)
	}

	plan, err = strategy.DecideFill(context.Background(), types.FillInput{
		Solver:        solver,
		TokenIn:       tokenIn,
		TokenOut:      tokenOut,
		AmountIn:      big.NewInt(1_000_000),
		OutputAmount:  big.NewInt(990_000),
		OutputContext: exclusiveLimitContext(otherSolver),
		ChainTime:     time.Unix(1_800_000_011, 0),
		MaxFeePerGas:  big.NewInt(0),
		Quotes:        profitableFillQuotes(tokenIn, tokenOut),
	})
	if err != nil {
		t.Fatalf("DecideFill after window: %v", err)
	}
	if plan == nil {
		t.Fatal("expected fill after exclusive window")
	}
}

func TestOutputPricingSupportsOutputSettlerSimpleContexts(t *testing.T) {
	solver := common.HexToAddress("0x5555555555555555555555555555555555555555")
	base := big.NewInt(990_000)
	now := time.Unix(1_800_000_005, 0)

	tests := map[string]struct {
		context []byte
		want    string
		fill    bool
		wantErr bool
	}{
		"empty limit": {
			context: nil,
			want:    "990000",
			fill:    true,
		},
		"typed limit": {
			context: []byte{limitOrderContextType},
			want:    "990000",
			fill:    true,
		},
		"dutch": {
			context: []byte{dutchAuctionContextType},
			wantErr: true,
		},
		"exclusive limit for solver": {
			context: exclusiveLimitContext(solver),
			want:    "990000",
			fill:    true,
		},
		"exclusive limit for another solver": {
			context: exclusiveLimitContext(common.HexToAddress("0x6666666666666666666666666666666666666666")),
			fill:    false,
		},
		"exclusive dutch": {
			context: []byte{exclusiveDutchAuctionContextType},
			wantErr: true,
		},
		"invalid type": {
			context: []byte{0x02},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			pricing, err := parseOutputContext(base, tt.context)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOutputContext: %v", err)
			}
			got, ok := pricing.fill(solver, now, big.NewInt(1_000_000))
			if ok != tt.fill {
				t.Fatalf("fill = %v", ok)
			}
			if !ok {
				return
			}
			if got.String() != tt.want {
				t.Fatalf("amount = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDistributeMinimumsPreservesTotalAndRouteBounds(t *testing.T) {
	tests := []struct {
		name    string
		targets []*big.Int
		total   *big.Int
		want    []string
	}{
		{name: "equal", targets: []*big.Int{big.NewInt(500), big.NewInt(500)}, total: big.NewInt(900), want: []string{"450", "450"}},
		{name: "uneven", targets: []*big.Int{big.NewInt(200), big.NewInt(800)}, total: big.NewInt(503), want: []string{"100", "403"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distributeMinimums(tt.targets, tt.total)
			if len(got) != len(tt.want) {
				t.Fatalf("minimums = %v", got)
			}
			total := new(big.Int)
			for i := range got {
				if got[i].String() != tt.want[i] || got[i].Sign() <= 0 || got[i].Cmp(tt.targets[i]) > 0 {
					t.Fatalf("minimum[%d] = %s, want %s", i, got[i], tt.want[i])
				}
				total.Add(total, got[i])
			}
			if total.Cmp(tt.total) != 0 {
				t.Fatalf("sum = %s, want %s", total, tt.total)
			}
		})
	}
}

func TestDecideFillDoesNotRequireProfitMargin(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn:      tokenIn,
		TokenOut:     tokenOut,
		AmountIn:     big.NewInt(1_000_000),
		OutputAmount: big.NewInt(990_000),
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{
				Route:     liquidlane.Route{ID: "route-1", Adapter: common.HexToAddress("0x3333333333333333333333333333333333333333"), TokenIn: tokenIn, TokenOut: tokenOut},
				MaxAssets: big.NewInt(2_000_000),
			},
			AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(999_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil {
		t.Fatal("expected fill without an explicit profit margin")
	}
}

func TestDecideFillSkipsExpiredOrder(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn:      tokenIn,
		TokenOut:     tokenOut,
		AmountIn:     big.NewInt(1_000_000),
		OutputAmount: big.NewInt(990_000),
		Expires:      1_700_000_000,
		FillDeadline: 1_700_000_100,
		ChainTime:    time.Unix(1_700_000_000, 0),
		MaxFeePerGas: big.NewInt(0),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{
				Route:     liquidlane.Route{ID: "route-1", Adapter: common.HexToAddress("0x3333333333333333333333333333333333333333"), TokenIn: tokenIn, TokenOut: tokenOut},
				MaxAssets: big.NewInt(2_000_000),
			},
			AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan != nil {
		t.Fatalf("expected expired skip, got %#v", plan)
	}
}

func exclusiveLimitContext(exclusiveFor common.Address) []byte {
	out := make([]byte, 37)
	out[0] = exclusiveLimitOrderContextType
	solverID := solverIdentifier(exclusiveFor)
	copy(out[1:33], solverID[:])
	binary.BigEndian.PutUint32(out[33:37], 1_800_000_010)
	return out
}

func profitableFillQuotes(tokenIn, tokenOut common.Address) []liquidlane.FillQuote {
	return []liquidlane.FillQuote{{
		Inventory: liquidlane.Inventory{
			Route: liquidlane.Route{
				ID: "route-1", Adapter: common.HexToAddress("0x3333333333333333333333333333333333333333"),
				TokenIn: tokenIn, TokenOut: tokenOut,
			},
			MaxAssets: big.NewInt(2_000_000),
		},
		AmountIn:     big.NewInt(1_000_000),
		MaxAmountOut: big.NewInt(1_000_000),
	}}
}

func testStrategyConfig(cfg Config) Config {
	return cfg
}

func testGasPrices(token common.Address, amount int64) *liquidlanegas.PriceSnapshot {
	return liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{token: big.NewInt(amount)})
}

func TestFixedPointDecimal(t *testing.T) {
	tests := map[string]string{
		"0":                      "0",
		"990000000000000000":     "0.99",
		"1000000000000000000":    "1",
		"1234500000000000000":    "1.2345",
		"1000000000000000000000": "1000",
	}
	for raw, want := range tests {
		n, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			t.Fatalf("bad test int %s", raw)
		}
		if got := fixedPointDecimal(n, 18); got != want {
			t.Fatalf("fixedPointDecimal(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestQuoteLadderKeepsBestDirectAndPrivate(t *testing.T) {
	route := liquidlane.Route{ID: "route-1"}
	bestDiscount := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	worseDiscount := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	ladder := buildQuoteLadder([]quoteCandidate{
		{
			Inventory: liquidlane.Inventory{Route: route, MaxRate: big.NewInt(90)},
			maxInput:  big.NewInt(100),
		},
		{
			Inventory: liquidlane.Inventory{Route: route, MaxRate: big.NewInt(100), DiscountID: &bestDiscount},
			maxInput:  big.NewInt(100),
		},
		{
			Inventory: liquidlane.Inventory{Route: route, MaxRate: big.NewInt(95), DiscountID: &worseDiscount},
			maxInput:  big.NewInt(1_000),
		},
	})
	if len(ladder) != 1 || len(ladder[0].alternatives) != 2 {
		t.Fatalf("ladder = %+v", ladder)
	}
	foundDirect, foundBestPrivate := false, false
	for _, candidate := range ladder[0].alternatives {
		foundDirect = foundDirect || candidate.DiscountID == nil
		foundBestPrivate = foundBestPrivate ||
			(candidate.DiscountID != nil && *candidate.DiscountID == bestDiscount)
	}
	if !foundDirect || !foundBestPrivate {
		t.Fatalf("alternatives = %+v", ladder[0].alternatives)
	}
}

func TestSolveExactInputQuoteUsesAlternativeThatCoversWholeLeg(t *testing.T) {
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	routeA := liquidlane.Route{ID: "route-a"}
	routeB := liquidlane.Route{ID: "route-b"}
	ladder := buildQuoteLadder([]quoteCandidate{
		{Inventory: liquidlane.Inventory{Route: routeA, MaxRate: big.NewInt(500_000_000_000_000_000)}, maxInput: big.NewInt(1_000)},
		{Inventory: liquidlane.Inventory{Route: routeA, MaxRate: big.NewInt(2_000_000_000_000_000_000), DiscountID: &discountID}, maxInput: big.NewInt(1)},
		{Inventory: liquidlane.Inventory{Route: routeB, MaxRate: big.NewInt(1_000_000_000_000_000_000)}, maxInput: big.NewInt(1_000)},
	})

	pricing := gasPricing{feePerGas: new(big.Int), tokenOutPerNative: new(big.Int)}
	quote, ok := solveExactInputQuote(ladder, big.NewInt(1_000), pricing)
	if !ok || len(quote.candidates) != 1 {
		t.Fatalf("quote = %+v, ok = %v", quote, ok)
	}
	if quote.candidates[0].ID != routeB.ID || quote.candidates[0].DiscountID != nil {
		t.Fatalf("candidates = %+v", quote.candidates)
	}
	if quote.grossAmountOut.Cmp(big.NewInt(1_000)) != 0 {
		t.Fatalf("output = %s, want 1000", quote.grossAmountOut)
	}

	small, ok := solveExactInputQuote(ladder, big.NewInt(1), pricing)
	if !ok || len(small.candidates) != 1 || small.candidates[0].DiscountID == nil {
		t.Fatalf("small quote = %+v, ok = %v", small, ok)
	}
}

func TestQuoteRangesCoverEveryInteriorAmountAndCandidate(t *testing.T) {
	strategy := &Strategy{minAmount: big.NewInt(1), rangeCount: 8}
	pricing := gasPricing{feePerGas: new(big.Int), tokenOutPerNative: new(big.Int)}
	rateUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(rateScaleDigits), nil)

	for scenario := 1; scenario <= 200; scenario++ {
		candidates := make([]quoteCandidate, 0, 6)
		for routeIndex := 0; routeIndex < 3; routeIndex++ {
			route := liquidlane.Route{ID: liquidlane.RouteID("route-" + strconv.Itoa(routeIndex))}
			directInput := big.NewInt(int64(1 + (scenario*(routeIndex+3))%15))
			directRate := new(big.Int).Mul(
				rateUnit, big.NewInt(int64(1+(scenario*(routeIndex+5))%7)),
			)
			candidates = append(candidates, quoteCandidate{
				Inventory: liquidlane.Inventory{Route: route, MaxRate: directRate},
				maxInput:  directInput,
			})

			discountID := common.BigToHash(big.NewInt(int64(scenario*10 + routeIndex + 1)))
			privateInput := big.NewInt(int64(1 + (scenario*(routeIndex+7))%15))
			privateRate := new(big.Int).Mul(
				rateUnit, big.NewInt(int64(1+(scenario*(routeIndex+11))%9)),
			)
			candidates = append(candidates, quoteCandidate{
				Inventory: liquidlane.Inventory{
					Route: route, MaxRate: privateRate, DiscountID: &discountID,
				},
				maxInput: privateInput,
			})
		}

		ladder := buildQuoteLadder(candidates)
		ranges, sampledCandidates := strategy.buildQuoteRanges(ladder, pricing)
		for _, quoteRange := range ranges {
			rate, parsed := new(big.Rat).SetString(quoteRange.Quote)
			if !parsed {
				t.Fatalf("scenario %d: invalid rate %q", scenario, quoteRange.Quote)
			}
			for amount := quoteRange.MinAmount.Int64(); amount <= quoteRange.MaxAmount.Int64(); amount++ {
				solution, solved := solveExactInputQuote(ladder, big.NewInt(amount), pricing)
				if !solved {
					t.Fatalf("scenario %d amount %d: published without a solution", scenario, amount)
				}
				quoted := new(big.Int).Mul(big.NewInt(amount), rate.Num())
				quoted.Div(quoted, rate.Denom())
				if quoted.Cmp(solution.grossAmountOut) > 0 {
					t.Fatalf(
						"scenario %d amount %d: quoted %s above output %s in range %+v",
						scenario, amount, quoted, solution.grossAmountOut, quoteRange,
					)
				}
				for _, candidate := range solution.candidates {
					if _, found := sampledCandidates[candidate.id()]; !found {
						t.Fatalf(
							"scenario %d amount %d: interior candidate %s missing from expiry set",
							scenario, amount, candidate.id(),
						)
					}
				}
			}
		}
	}
}
