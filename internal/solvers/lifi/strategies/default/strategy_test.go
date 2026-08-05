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
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
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
	if strategy.rangeCount != 8 {
		t.Fatalf("range count = %d", strategy.rangeCount)
	}
}

func TestRangeCountValidation(t *testing.T) {
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

func TestMaximumNonOverquotingRate(t *testing.T) {
	tests := []struct {
		name                    string
		amountIn, amountOut     int64
		inDecimals, outDecimals int
		want                    int64
	}{
		{name: "fractional boundary", amountIn: 3, amountOut: 1, want: 666_666_666_666_666_666},
		{name: "exact boundary", amountIn: 4, amountOut: 1, want: 499_999_999_999_999_999},
		{
			name: "decimal conversion", amountIn: 3, amountOut: 1,
			inDecimals: 6, outDecimals: 18, want: 666_666,
		},
		{
			name: "unrepresentable output", amountIn: 2_000_000_000_000_000_000, amountOut: 1,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amountIn := big.NewInt(tt.amountIn)
			amountOut := big.NewInt(tt.amountOut)
			got := maximumNonOverquotingRate(
				amountOut, amountIn, tt.inDecimals, tt.outDecimals,
			)
			if got.Cmp(big.NewInt(tt.want)) != 0 {
				t.Fatalf("rate = %s, want %d", got, tt.want)
			}
			if output := liquidlane.AmountOutForRate(
				amountIn, got, tt.inDecimals, tt.outDecimals,
			); output.Cmp(amountOut) > 0 {
				t.Fatalf("rate %s produces %s, above %s", got, output, amountOut)
			}
			next := new(big.Int).Add(got, big.NewInt(1))
			if output := liquidlane.AmountOutForRate(
				amountIn, next, tt.inDecimals, tt.outDecimals,
			); output.Cmp(amountOut) <= 0 {
				t.Fatalf("next rate %s is still safe", next)
			}
		})
	}
}

func TestPriceQuoteRangeKeepsPositiveMinimumOutput(t *testing.T) {
	strategy := &Strategy{minAmount: big.NewInt(3)}
	candidates := []liquidlane.QuoteCandidate{{
		ID:           "route-1",
		Route:        liquidlane.Route{ID: "route-1"},
		Rate:         big.NewInt(500_000_000_000_000_000),
		MaxAmountIn:  big.NewInt(4),
		MaxAmountOut: big.NewInt(2),
	}}
	pricing, err := liquidstrategies.NewGasPricing(
		big.NewInt(0), candidates[0].Route.TokenOut, nil, nil, 0, liquidstrategies.GasEnvelope{},
	)
	if err != nil {
		t.Fatalf("NewGasPricing: %v", err)
	}
	quoteRange, err := strategy.priceQuoteRange(
		candidates, 1, big.NewInt(3), big.NewInt(4), pricing.MaxCost(1, 0), 1, pricing,
	)
	if err != nil {
		t.Fatalf("priceQuoteRange: %v", err)
	}
	if quoteRange == nil ||
		quoteRange.MinAmount.Cmp(big.NewInt(3)) != 0 ||
		quoteRange.MaxAmount.Cmp(big.NewInt(4)) != 0 ||
		quoteRange.Quote != "0.5" {
		t.Fatalf("range = %+v, want [3,4] at 0.5", quoteRange)
	}
}

func TestDecideQuotesTrimsAtGasBreakEven(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "1", RangeCount: 8}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	route := liquidlane.Route{
		ID: "route-1", TokenIn: tokenIn, TokenOut: tokenOut,
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	maximum := big.NewInt(10_000_000)
	rate := big.NewInt(1_000_000_000_000_000_000)
	gasPrices := testGasPrices(tokenOut, 1_000_000_000_000_000_000)
	candidates := []liquidlane.QuoteCandidate{{
		ID: "route-1", Route: route, Rate: rate,
		MaxAmountIn: maximum, MaxAmountOut: maximum,
	}}
	breakpoints := quoteBreakpoints(maximum, big.NewInt(1), 8)
	if len(breakpoints) != 8 {
		t.Fatalf("breakpoints = %v, want eight", breakpoints)
	}

	tests := []struct {
		name          string
		maxFeePerGas  int64
		crossingRange int
	}{
		{name: "penultimate range", maxFeePerGas: 1, crossingRange: 6},
		{name: "final range", maxFeePerGas: 2, crossingRange: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxFeePerGas := big.NewInt(tt.maxFeePerGas)
			pricing, pricingErr := liquidstrategies.NewGasPricing(
				maxFeePerGas, tokenOut, gasPrices, nil, 0, types.LiquidLaneGasEnvelope(),
			)
			if pricingErr != nil {
				t.Fatalf("NewGasPricing: %v", pricingErr)
			}
			gasCost := pricing.MaxCost(1, 0)
			actualGasCost := pricing.Cost([]liquidstrategies.GasLeg{{Route: route, AmountOut: maximum}})
			if actualGasCost.Cmp(gasCost) != 0 {
				t.Fatalf("actual gas cost = %s, max gas cost = %s", actualGasCost, gasCost)
			}

			// At gasCost+1 the floor rate is positive but still rounds to zero output.
			wantMin := new(big.Int).Add(gasCost, big.NewInt(2))
			floorOutput := func(amount *big.Int) *big.Int {
				floorRate := candidateFloorRate(candidates, amount, maximum, gasCost, 1, 0, 6, 6)
				return liquidlane.AmountOutForRate(amount, floorRate, 6, 6)
			}
			if unsafeInput := new(big.Int).Sub(wantMin, big.NewInt(1)); floorOutput(unsafeInput).Sign() != 0 {
				t.Fatalf("amount %s has positive floor output", unsafeInput)
			}
			if floorOutput(wantMin).Sign() <= 0 {
				t.Fatalf("amount %s has no positive floor output", wantMin)
			}

			rangeLower := big.NewInt(1)
			if tt.crossingRange > 0 {
				rangeLower = new(big.Int).Add(breakpoints[tt.crossingRange-1], big.NewInt(1))
			}
			rangeUpper := breakpoints[tt.crossingRange]
			if rangeLower.Cmp(wantMin) >= 0 || rangeUpper.Cmp(wantMin) < 0 {
				t.Fatalf("range [%s,%s] does not cross safe minimum %s", rangeLower, rangeUpper, wantMin)
			}

			out, decideErr := strategy.DecideQuotes(context.Background(), types.QuoteInput{
				Inventory: []liquidlane.Inventory{{
					Route:     route,
					MaxAssets: maximum, MaxRate: rate,
				}},
				GasPrices: gasPrices, MaxFeePerGas: maxFeePerGas,
				ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
			})
			if decideErr != nil {
				t.Fatalf("DecideQuotes: %v", decideErr)
			}
			if len(out.Quotes) != 1 || len(out.Quotes[0].Ranges) == 0 {
				t.Fatalf("quotes = %+v, want ranges above gas break-even", out.Quotes)
			}
			ranges := out.Quotes[0].Ranges
			if ranges[0].MinAmount.Cmp(wantMin) != 0 {
				t.Fatalf("first minAmount = %s, want %s", ranges[0].MinAmount, wantMin)
			}
			if ranges[len(ranges)-1].MaxAmount.Cmp(maximum) != 0 {
				t.Fatalf("last maxAmount = %s, want %s", ranges[len(ranges)-1].MaxAmount, maximum)
			}
		})
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
	pricing, err := liquidstrategies.NewGasPricing(
		big.NewInt(1_000_000_000), tokenOut, testGasPrices(tokenOut, 1_000_000), gasSnapshot, 0,
		types.LiquidLaneGasEnvelope(),
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
			actual := new(big.Int).Sub(big.NewInt(amount), pricing.Cost([]liquidstrategies.GasLeg{{
				Route: route, AmountOut: big.NewInt(amount),
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

func TestPriceQuoteRangeNeverOverquotesInteriorRouteTransition(t *testing.T) {
	strategy, err := New(Config{MinAmount: "1", RangeCount: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	rateUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	candidate := func(
		id liquidlane.CandidateID,
		routeID liquidlane.RouteID,
		rate int64,
		maxInput int64,
		discountID *common.Hash,
	) liquidlane.QuoteCandidate {
		scaledRate := new(big.Int).Mul(rateUnit, big.NewInt(rate))
		return liquidlane.QuoteCandidate{
			ID: id,
			Route: liquidlane.Route{
				ID: routeID, CapacityID: liquidlane.CapacityID(routeID),
				TokenIn: tokenIn, TokenOut: tokenOut,
			},
			Rate: scaledRate, MaxAmountIn: big.NewInt(maxInput),
			MaxAmountOut: big.NewInt(rate * maxInput), DiscountID: discountID,
		}
	}
	discountA := common.HexToHash("0x01")
	discountB := common.HexToHash("0x02")
	candidates := []liquidlane.QuoteCandidate{
		candidate("a-private", "a", 2, 1, &discountA),
		candidate("a-direct", "a", 1, 5, nil),
		candidate("b-private", "b", 2, 4, &discountB),
		candidate("b-direct", "b", 1, 5, nil),
		candidate("c-direct", "c", 2, 2, nil),
	}
	pricing, err := liquidstrategies.NewGasPricing(
		big.NewInt(0), tokenOut, nil, nil, 0, liquidstrategies.GasEnvelope{},
	)
	if err != nil {
		t.Fatalf("NewGasPricing: %v", err)
	}
	quoteRange, err := strategy.priceQuoteRange(
		candidates, 3, big.NewInt(6), big.NewInt(9), pricing.MaxCost(3, 2), 3, pricing,
	)
	if err != nil {
		t.Fatalf("priceQuoteRange: %v", err)
	}
	if quoteRange == nil {
		t.Fatal("priceQuoteRange returned no range")
	}
	rate, ok := new(big.Rat).SetString(quoteRange.Quote)
	if !ok {
		t.Fatalf("invalid quote rate %q", quoteRange.Quote)
	}
	for amount := quoteRange.MinAmount.Int64(); amount <= quoteRange.MaxAmount.Int64(); amount++ {
		actual, solveErr := liquidgreedy.SolveQuote(liquidgreedy.QuoteTask{
			ExactInput: big.NewInt(amount), Candidates: candidates, MaxRoutes: 3,
			MinInput: strategy.minAmount, InputPolicy: liquidgreedy.RejectUncoveredInput,
		})
		if solveErr != nil || actual == nil {
			t.Fatalf("SolveQuote(%d) = %+v, %v", amount, actual, solveErr)
		}
		quoted := new(big.Int).Mul(big.NewInt(amount), rate.Num())
		quoted.Div(quoted, rate.Denom())
		if quoted.Cmp(actual.AmountOut) > 0 {
			t.Fatalf(
				"amount %d quoted %s above executable %s in range %+v",
				amount, quoted, actual.AmountOut, quoteRange,
			)
		}
	}
}

func TestBuildQuoteRangesScalesToManyRoutes(t *testing.T) {
	strategy, err := New(Config{MinAmount: "100", RangeCount: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	candidates := make([]liquidlane.QuoteCandidate, 128)
	for index := range candidates {
		routeID := liquidlane.RouteID("route-" + strconv.Itoa(index))
		candidates[index] = liquidlane.QuoteCandidate{
			ID: liquidlane.CandidateID(routeID),
			Route: liquidlane.Route{
				ID: routeID, CapacityID: liquidlane.CapacityID(routeID),
				TokenIn: tokenIn, TokenOut: tokenOut,
			},
			Rate:         big.NewInt(2_000_000_000_000_000_000),
			MaxAmountIn:  big.NewInt(100),
			MaxAmountOut: big.NewInt(200),
		}
	}
	pricing, err := liquidstrategies.NewGasPricing(
		big.NewInt(0), tokenOut, nil, nil, 0, liquidstrategies.GasEnvelope{},
	)
	if err != nil {
		t.Fatalf("NewGasPricing: %v", err)
	}
	ranges, _, err := strategy.buildQuoteRanges(candidates, 64, pricing)
	if err != nil {
		t.Fatalf("buildQuoteRanges: %v", err)
	}
	if len(ranges) != 1 || ranges[0].MaxAmount.Cmp(big.NewInt(6_400)) != 0 {
		t.Fatalf("ranges = %+v, want one range through 6400", ranges)
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

func TestDecideQuotesFiltersPrivateAlternativeExpiredByServerClock(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "100"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	chainTime := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", TokenIn: tokenIn, TokenOut: tokenOut,
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory: []liquidlane.Inventory{
			liquidlane.DirectInventory(route, big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000)),
			liquidlane.DiscountInventory(
				route, big.NewInt(1_000), big.NewInt(500_000_000_000_000_000),
				discountID, chainTime.Add(20*time.Second),
			),
		},
		MaxFeePerGas: big.NewInt(0), ChainTime: chainTime, ServerTime: chainTime.Add(9 * time.Second),
		QuoteExpiresAt: chainTime.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v, want direct quote", out.Quotes)
	}
	if out.Quotes[0].Expiry != chainTime.Add(time.Minute).Unix() {
		t.Fatalf("expiry = %d, want %d", out.Quotes[0].Expiry, chainTime.Add(time.Minute).Unix())
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
		Reservations: liquidlane.CapacityReservations{"capacity-1": big.NewInt(60)},
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
		Inventory:      []liquidlane.Inventory{routeItem},
		Reservations:   liquidlane.CapacityReservations{"capacity-1": big.NewInt(800)},
		MaxFeePerGas:   big.NewInt(0),
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

func TestDecideQuotesPublishesFullSharedCapacityForEachPair(t *testing.T) {
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
	tests := []struct {
		name         string
		reservations liquidlane.CapacityReservations
		want         string
	}{
		{name: "available", want: "1000"},
		{
			name:         "reserved",
			reservations: liquidlane.CapacityReservations{"capacity-1": big.NewInt(400)},
			want:         "600",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
				Inventory:      inventory,
				Reservations:   tt.reservations,
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
			for _, quote := range out.Quotes {
				if got := quote.Ranges[len(quote.Ranges)-1].MaxAmount.String(); got != tt.want {
					t.Fatalf("pair %s/%s maxAmount = %s, want %s",
						quote.FromAsset.Hex(), quote.ToAsset.Hex(), got, tt.want)
				}
			}
		})
	}
}

func TestDecideQuotesDoesNotDoubleCountSharedCapacityWithinPair(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{MinAmount: "2"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := make([]liquidlane.Inventory, 2)
	for index := range inventory {
		inventory[index] = liquidlane.Inventory{
			Route: liquidlane.Route{
				ID:              liquidlane.RouteID("route-" + strconv.Itoa(index+1)),
				CapacityID:      "capacity-1",
				Adapter:         common.BytesToAddress([]byte{byte(index + 1)}),
				TokenIn:         tokenIn,
				TokenOut:        tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}
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
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %d", len(out.Quotes))
	}
	if got := out.Quotes[0].Ranges[len(out.Quotes[0].Ranges)-1].MaxAmount.String(); got != "1000" {
		t.Fatalf("maxAmount = %s, want 1000", got)
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
		if !types.IsPermanentFillDecisionError(decideErr) {
			t.Fatalf("DecideFill(context=%x) error is not permanent", outputContext)
		}
		if plan != nil {
			t.Fatalf("DecideFill(context=%x) plan = %+v", outputContext, plan)
		}
	}
}

func TestDecideFillMarksMalformedOutputContextPermanent(t *testing.T) {
	strategy, err := New(testStrategyConfig(Config{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, outputContext := range [][]byte{
		{limitOrderContextType, 0x01},
		{exclusiveLimitOrderContextType},
		{0x02},
	} {
		plan, decideErr := strategy.DecideFill(context.Background(), types.FillInput{
			AmountIn:      big.NewInt(1_000_000),
			OutputAmount:  big.NewInt(990_000),
			OutputContext: outputContext,
		})
		if decideErr == nil || !types.IsPermanentFillDecisionError(decideErr) {
			t.Fatalf("DecideFill(context=%x) error = %v, want permanent", outputContext, decideErr)
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
