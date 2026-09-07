package defaultstrategy

import (
	"context"
	"encoding/binary"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning/strategytest"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

func TestDecideQuotesRequiresSolverExpiry(t *testing.T) {
	strategy := testStrategy(t, Config{})
	if _, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		ChainTime: time.Unix(1_800_000_000, 0),
	}); err == nil {
		t.Fatal("expected missing quote expiry error")
	}
}

func TestDefaultExecutionBufferIsOneBlock(t *testing.T) {
	strategy := testStrategy(t, Config{})
	if strategy.policy.ExecutionBuffer != 12*time.Second {
		t.Fatalf("execution buffer = %s", strategy.policy.ExecutionBuffer)
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

// One physical route isolates amount floors, configured ranges and reservation ordering.
func TestDecideQuotesInventoryLimits(t *testing.T) {
	for _, test := range []struct {
		name                                   string
		config                                 Config
		assets, reserved, minAmount, maxAmount int64
		ranges                                 int
		lifetime                               time.Duration
		wantRate, rateCeiling                  string
	}{
		{name: "price buffer", config: Config{PriceBufferBps: 100, MinAmount: "1000"}, assets: 990_000_000,
			minAmount: 1000, maxAmount: 990_000_000, lifetime: 5 * time.Minute, rateCeiling: "0.98"},
		{name: "break even minimum", config: Config{MinAmount: "1"}, assets: 100,
			minAmount: 1, maxAmount: 100, lifetime: time.Minute, wantRate: "1"},
		{name: "configured range count", config: Config{MinAmount: "100", RangeCount: 4}, assets: 1000,
			minAmount: 100, maxAmount: 1000, ranges: 4, lifetime: time.Minute},
		{name: "below minimum", config: Config{MinAmount: "1000001"}, assets: 1_000_000, lifetime: 90 * time.Second},
		{name: "inventory reserve", config: Config{InventoryReserveBps: 1000, MinAmount: "2"}, assets: 1000,
			minAmount: 2, maxAmount: 900, lifetime: 90 * time.Second},
		{name: "reserve before pending", config: Config{InventoryReserveBps: 1000, MinAmount: "2"}, assets: 1000, reserved: 800,
			minAmount: 2, maxAmount: 100, lifetime: 90 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(1_800_000_000, 0)
			input := types.QuoteInput{
				Solver: common.Address{1}, ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(test.lifetime), MaxFeePerGas: new(big.Int),
				Inventory: []liquidlane.Inventory{{
					Route: liquidlane.Route{ID: "route-1", CapacityID: "capacity-1", Adapter: common.Address{2},
						TokenIn: common.Address{3}, TokenOut: common.Address{4}, TokenInDecimals: 6, TokenOutDecimals: 6},
					MaxAssets: big.NewInt(test.assets), MaxRate: big.NewInt(1_000_000_000_000_000_000),
				}},
			}
			if test.reserved > 0 {
				input.Reservations = liquidlane.CapacityReservations{"capacity-1": big.NewInt(test.reserved)}
			}
			out, err := testStrategy(t, test.config).DecideQuotes(t.Context(), input)
			testcheck.NoError(t, err)
			if test.maxAmount == 0 {
				if len(out.Quotes) != 0 {
					t.Fatalf("below-minimum inventory quoted: %+v", out.Quotes)
				}
				return
			}
			if len(out.Quotes) != 1 || len(out.Quotes[0].Ranges) == 0 {
				t.Fatalf("quotes = %+v", out.Quotes)
			}
			quote := out.Quotes[0]
			ranges := quote.Ranges
			if quote.Expiry != input.QuoteExpiresAt.Unix() || ranges[0].MinAmount.Cmp(big.NewInt(test.minAmount)) != 0 || ranges[len(ranges)-1].MaxAmount.Cmp(big.NewInt(test.maxAmount)) != 0 {
				t.Fatalf("quote = %+v, want [%d,%d] until %s", quote, test.minAmount, test.maxAmount, input.QuoteExpiresAt)
			}
			if test.ranges != 0 && len(ranges) != test.ranges {
				t.Fatalf("ranges = %d, want %d", len(ranges), test.ranges)
			}
			for i := 1; i < len(ranges); i++ {
				if want := new(big.Int).Add(ranges[i-1].MaxAmount, big.NewInt(1)); ranges[i].MinAmount.Cmp(want) != 0 {
					t.Fatalf("range[%d].min = %s, want %s", i, ranges[i].MinAmount, want)
				}
			}
			if test.wantRate != "" && ranges[0].Quote != test.wantRate {
				t.Fatalf("rate = %s, want %s", ranges[0].Quote, test.wantRate)
			}
			if test.rateCeiling != "" {
				rate, ok := new(big.Rat).SetString(ranges[0].Quote)
				ceiling, _ := new(big.Rat).SetString(test.rateCeiling)
				if !ok || rate.Sign() <= 0 || rate.Cmp(ceiling) > 0 {
					t.Fatalf("rate = %s, want positive and <= %s", ranges[0].Quote, test.rateCeiling)
				}
			}
		})
	}
}

func TestDecideQuotesChargesGasAfterBuildingRange(t *testing.T) {
	cfg := Config{MinAmount: "1000"}
	strategy := testStrategy(t, cfg)

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Solver:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ChainTime:      time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_090, 0),
		MaxFeePerGas:   big.NewInt(100),
		GasPrices:      testGasPrices(common.HexToAddress("0x4444444444444444444444444444444444444444"), 1_000_000_000_000),
		Inventory: []liquidlane.Inventory{{
			Route: liquidlane.Route{
				ID:      "route-1",
				Adapter: common.HexToAddress("0x2222222222222222222222222222222222222222"),
				TokenIn: common.HexToAddress("0x3333333333333333333333333333333333333333"), TokenOut: common.HexToAddress("0x4444444444444444444444444444444444444444"),
				TokenInDecimals:  6,
				TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(20_000),
			MaxRate:   big.NewInt(1_000_000_000_000_000_000),
		}},
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
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
			got := liquidlane.MaxRateForAmountOut(
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
	strategy := &Strategy{policy: planning.ExecutionPolicy{MinAmount: big.NewInt(3)}}
	candidates := []liquidlane.QuoteCandidate{{
		ID:           "route-1",
		Route:        liquidlane.Route{ID: "route-1"},
		Rate:         big.NewInt(500_000_000_000_000_000),
		MaxAmountIn:  big.NewInt(4),
		MaxAmountOut: big.NewInt(2),
	}}
	pricing, err := planning.NewGasPricing(
		big.NewInt(0), candidates[0].Route.TokenOut, nil, nil, 0, planning.GasEnvelope{},
	)
	testcheck.NoError(t, err, "NewGasPricing: %v")
	quoter := newRangeQuoter(strategy, candidates, 1, pricing)
	quoteRange, err := quoter.quote(big.NewInt(3), big.NewInt(4), quoter.floor(big.NewInt(4)))
	testcheck.NoError(t, err, "priceQuoteRange: %v")
	if quoteRange == nil ||
		quoteRange.MinAmount.Cmp(big.NewInt(3)) != 0 ||
		quoteRange.MaxAmount.Cmp(big.NewInt(4)) != 0 ||
		quoteRange.Quote != "0.5" {
		t.Fatalf("range = %+v, want [3,4] at 0.5", quoteRange)
	}
}

func TestQuoteBreakpointsRespectIntegerIntervals(t *testing.T) {
	for _, tc := range []struct {
		name             string
		minimum, maximum int64
		count            int
		want             []int64
	}{
		{name: "backwards", minimum: 5, maximum: 4, count: 8},
		{name: "no intervals", minimum: 1, maximum: 10, count: 0},
		{name: "one point", minimum: 4, maximum: 4, count: 8, want: []int64{4}},
		{name: "adjacent endpoints", minimum: 4, maximum: 5, count: 8, want: []int64{5}},
		{name: "exhaust integer interior", minimum: 3, maximum: 7, count: 20, want: []int64{4, 5, 6, 7}},
		{name: "equal relative spans", minimum: 1, maximum: 10_000, count: 4, want: []int64{10, 100, 1000, 10_000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteBreakpoints(big.NewInt(tc.maximum), big.NewInt(tc.minimum), tc.count)
			if len(got) != len(tc.want) {
				t.Fatalf("breakpoints = %v, want %v", got, tc.want)
			}
			for i, want := range tc.want {
				if got[i].Cmp(big.NewInt(want)) != 0 {
					t.Fatalf("breakpoints = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestDecideQuotesTrimsAtGasBreakEven(t *testing.T) {
	strategy := testStrategy(t, Config{MinAmount: "1", RangeCount: 8})
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
			pricing, pricingErr := planning.NewGasPricing(
				maxFeePerGas, tokenOut, gasPrices, nil, 0, types.LiquidLaneGasEnvelope(),
			)
			testcheck.NoError(t, pricingErr, "NewGasPricing: %v")
			gasCost := pricing.MaxCost(1, 0)
			actualGasCost := pricing.Cost([]planning.GasLeg{{Route: route, AmountOut: maximum}})
			if actualGasCost.Cmp(gasCost) != 0 {
				t.Fatalf("actual gas cost = %s, max gas cost = %s", actualGasCost, gasCost)
			}

			// At gasCost+1 the floor rate is positive but still rounds to zero output.
			wantMin := new(big.Int).Add(gasCost, big.NewInt(2))
			floorOutput := func(amount *big.Int) *big.Int {
				floorRate := newRangeQuoter(strategy, candidates, 1, pricing).floor(maximum).at(amount)
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
				GasPrices: gasPrices, MaxFeePerGas: maxFeePerGas, ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
			})
			testcheck.NoError(t, decideErr, "DecideQuotes: %v")
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
	cfg := Config{MinAmount: "900"}
	strategy := testStrategy(t, cfg)

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
		GasSnapshot: gasSnapshot, GasPrices: testGasPrices(tokenOut, 1_000_000), MaxFeePerGas: big.NewInt(1_000_000_000), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
	if len(out.Quotes) != 1 || len(out.Quotes[0].Ranges) == 0 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	pricing, err := planning.NewGasPricing(
		big.NewInt(1_000_000_000), tokenOut, testGasPrices(tokenOut, 1_000_000), gasSnapshot, 0,
		types.LiquidLaneGasEnvelope(),
	)
	testcheck.NoError(t, err, "pricing: %v")
	for _, quoteRange := range out.Quotes[0].Ranges {
		rate, ok := new(big.Rat).SetString(quoteRange.Quote)
		if !ok {
			t.Fatalf("invalid quote rate %q", quoteRange.Quote)
		}
		for amount := quoteRange.MinAmount.Int64(); amount <= quoteRange.MaxAmount.Int64(); amount++ {
			actual := new(big.Int).Sub(big.NewInt(amount), pricing.Cost([]planning.GasLeg{{
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

func TestDecideQuotesUsesAtMostThreePhysicalRoutes(t *testing.T) {
	strategy := testStrategy(t, Config{MinAmount: "2"})
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
		Inventory: inventory, MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
	ranges := out.Quotes[0].Ranges
	if got := ranges[len(ranges)-1].MaxAmount.String(); got != "300" {
		t.Fatalf("maxAmount = %s, want three-route capacity 300", got)
	}
}

func TestDecideQuotesRespectsRouteLimit(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	for _, tc := range []struct {
		name, wantMax string
		singleRoute   map[common.Address]bool
	}{
		{"independent routes aggregate", "1000", nil},
		{"permissioned token uses one route", "500", map[common.Address]bool{tokenIn: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strategy := testStrategy(t, Config{MinAmount: "2"})
			now := time.Unix(1_800_000_000, 0)
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
				Inventory: inventory, SingleRouteTokens: tc.singleRoute,
				MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
			})
			testcheck.NoError(t, err, "DecideQuotes: %v")
			if len(out.Quotes) != 1 {
				t.Fatalf("quotes = %+v", out.Quotes)
			}
			ranges := out.Quotes[0].Ranges
			if got := ranges[len(ranges)-1].MaxAmount.String(); got != tc.wantMax {
				t.Fatalf("maxAmount = %s, want %s", got, tc.wantMax)
			}
		})
	}
}

func TestDecideQuotesNeverOverquotesBlendedRouteRange(t *testing.T) {
	strategy := testStrategy(t, Config{MinAmount: "2"})
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
		Inventory: inventory, MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
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
	strategy := testStrategy(t, Config{MinAmount: "1", RangeCount: 1})
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
	pricing, err := planning.NewGasPricing(
		big.NewInt(0), tokenOut, nil, nil, 0, planning.GasEnvelope{},
	)
	testcheck.NoError(t, err, "NewGasPricing: %v")
	quoter := newRangeQuoter(strategy, candidates, 3, pricing)
	quoteRange, err := quoter.quote(big.NewInt(6), big.NewInt(9), quoter.floor(big.NewInt(9)))
	testcheck.NoError(t, err, "priceQuoteRange: %v")
	if quoteRange == nil {
		t.Fatal("priceQuoteRange returned no range")
	}
	rate, ok := new(big.Rat).SetString(quoteRange.Quote)
	if !ok {
		t.Fatalf("invalid quote rate %q", quoteRange.Quote)
	}
	for amount := quoteRange.MinAmount.Int64(); amount <= quoteRange.MaxAmount.Int64(); amount++ {
		actual, solveErr := planning.NewQuotePool(candidates).Solve(planning.QuoteTask{
			ExactInput: big.NewInt(amount), MaxRoutes: 3,
			MinInput: strategy.policy.MinAmount, InputPolicy: planning.RejectUncoveredInput,
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
	strategy := testStrategy(t, Config{MinAmount: "100", RangeCount: 1})
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
	pricing, err := planning.NewGasPricing(
		big.NewInt(0), tokenOut, nil, nil, 0, planning.GasEnvelope{},
	)
	testcheck.NoError(t, err, "NewGasPricing: %v")
	ranges, _, err := strategy.buildQuoteRanges(candidates, 64, pricing)
	testcheck.NoError(t, err, "buildQuoteRanges: %v")
	if len(ranges) != 1 || ranges[0].MaxAmount.Cmp(big.NewInt(6_400)) != 0 {
		t.Fatalf("ranges = %+v, want one range through 6400", ranges)
	}
}

func TestDecideQuotesUsesPrivateAlternativeBeforeDirectFallback(t *testing.T) {
	strategy := testStrategy(t, Config{MinAmount: "100"})
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
		Inventory: inventory, MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
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
	strategy := testStrategy(t, Config{MinAmount: "100"})
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
	testcheck.NoError(t, err, "DecideQuotes: %v")
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v, want direct quote", out.Quotes)
	}
	if out.Quotes[0].Expiry != chainTime.Add(time.Minute).Unix() {
		t.Fatalf("expiry = %d, want %d", out.Quotes[0].Expiry, chainTime.Add(time.Minute).Unix())
	}
}

func TestPriceBufferCoversQuoteToFillAndExecutionWindows(t *testing.T) {
	strategy := testStrategy(t, Config{PriceBufferBps: 100, MinAmount: "10000"})
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
	testcheck.NoError(t, err, "DecideQuotes: %v")
	if len(quotes.Quotes) != 1 {
		t.Fatalf("quote = %+v", quotes.Quotes)
	}
	quoteRate, ok := new(big.Rat).SetString(quotes.Quotes[0].Ranges[0].Quote)
	if !ok || quoteRate.Sign() <= 0 || quoteRate.Cmp(big.NewRat(98, 100)) > 0 {
		t.Fatalf("quote rate = %q, want positive rate no greater than 0.98", quotes.Quotes[0].Ranges[0].Quote)
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10_000), OutputAmount: big.NewInt(9_800),
			ChainTime: now, MaxFeePerGas: big.NewInt(0),
			Quotes: []liquidlane.FillQuote{{
				Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(20_000)},
				AmountIn:  big.NewInt(10_000), MaxAmountOut: big.NewInt(9_900),
			}},
		},
	})
	testcheck.NoError(t, err, "DecideFill: %v")
	if plan == nil {
		t.Fatal("expected fill after one price-buffer adverse move")
	}
}

func TestDecideQuotesUsesPrivateDiscountWithoutDirectCandidateAndClipsExpiry(t *testing.T) {
	strategy := testStrategy(t, Config{MinAmount: "1000"})
	now := time.Unix(1_800_000_000, 0)
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1",
		TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenOut: common.HexToAddress("0x2222222222222222222222222222222222222222"),
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	discount := liquidlane.DiscountInventory(
		route, big.NewInt(900), big.NewInt(800_000_000_000_000_000), discountID, now.Add(time.Minute),
	)

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory:    []liquidlane.Inventory{discount},
		MaxFeePerGas: big.NewInt(0), ChainTime: now, QuoteExpiresAt: now.Add(90 * time.Second),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
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
	strategy := testStrategy(t, Config{})
	now := time.Unix(1_800_000_000, 0)
	route := liquidlane.Route{
		ID: "route-1", TokenIn: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenOut: common.HexToAddress("0x2222222222222222222222222222222222222222"),
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
		Inventory: []liquidlane.Inventory{direct, discount}, MaxFeePerGas: big.NewInt(0), ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %+v", out.Quotes)
	}
}

func TestDecideFillSingleRoute(t *testing.T) {
	for _, tc := range []struct {
		name              string
		config            Config
		now               time.Time
		expires, deadline uint32
		quotedOutput      int64
		wantPlan          bool
	}{
		{"profitable", Config{MinAmount: "1000"}, time.Unix(1_800_000_000, 0), 0, 0, 1_000_000, true},
		{"no required profit margin", Config{}, time.Time{}, 0, 0, 999_000, true},
		{"expired", Config{}, time.Unix(1_700_000_000, 0), 1_700_000_000, 1_700_000_100, 1_000_000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strategy := testStrategy(t, tc.config)
			tokenIn, tokenOut, adapter := common.HexToAddress("0x1"), common.HexToAddress("0x2"), common.HexToAddress("0x3")
			plan, err := strategy.DecideFill(t.Context(), types.FillInput{
				FillInput: planning.FillInput{
					TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(990_000),
					ChainTime: tc.now, MaxFeePerGas: big.NewInt(0),
					Quotes: []liquidlane.FillQuote{{
						Inventory: liquidlane.Inventory{
							Route:     liquidlane.Route{ID: "route-1", Adapter: adapter, TokenIn: tokenIn, TokenOut: tokenOut},
							MaxAssets: big.NewInt(2_000_000),
						},
						AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(tc.quotedOutput),
					}},
				},

				Expires: tc.expires, FillDeadline: tc.deadline,
			})
			testcheck.NoError(t, err, "DecideFill: %v")
			if (plan != nil) != tc.wantPlan {
				t.Fatalf("plan = %+v, want plan %t", plan, tc.wantPlan)
			}
			if plan != nil && (len(plan.Routes) != 1 || plan.Routes[0].Adapter != adapter ||
				plan.Routes[0].ExpectedAmountOut.Cmp(big.NewInt(tc.quotedOutput)) != 0) {
				t.Fatalf("routes = %+v, want adapter %s and output %d", plan.Routes, adapter, tc.quotedOutput)
			}
		})
	}
}

func TestDecideFillPermissionedTokenNeverAggregatesRoutes(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := testStrategy(t, Config{})
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
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900),
			RequireSingleRoute: true,
			ChainTime:          time.Unix(1_800_000_000, 0), MaxFeePerGas: big.NewInt(0),
			Quotes: quotes,
		},
	})
	testcheck.NoError(t, err, "DecideFill: %v")
	if plan != nil {
		t.Fatalf("permissioned token must not aggregate routes, got %+v", plan)
	}
}

func TestDecideFillChargesPrivateExecutionGasAfterGreedySelection(t *testing.T) {
	cfg := Config{}
	strategy := testStrategy(t, cfg)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	vault := common.HexToAddress("0x4444444444444444444444444444444444444444")
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	route := liquidlane.Route{ID: "route-1", Adapter: adapter, Vault: vault, TokenIn: tokenIn, TokenOut: tokenOut}

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900_000),
			ChainTime: time.Unix(1_800_000_000, 0), MaxFeePerGas: big.NewInt(1),
			GasPrices: testGasPrices(tokenOut, 1_000_000_000_000_000_000),
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
		},
	})
	testcheck.NoError(t, err, "DecideFill: %v")
	if plan == nil || len(plan.Routes) != 1 || plan.Routes[0].DiscountID == nil {
		t.Fatalf("plan = %+v, want higher-rate private route", plan)
	}
	if plan.Routes[0].MinAmountOut.Cmp(big.NewInt(900_000)) <= 0 {
		t.Fatalf("minAmountOut = %s, want order output plus complete-plan gas", plan.Routes[0].MinAmountOut)
	}
}

func TestDecideFillPrivateCapacityIncludesUpwardPriceBuffer(t *testing.T) {
	strategy := testStrategy(t, Config{PriceBufferBps: 100})
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
				FillInput: planning.FillInput{
					TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10_000), OutputAmount: big.NewInt(9_500),
					ChainTime: now, MaxFeePerGas: big.NewInt(0),
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
				},
			})
			testcheck.NoError(t, fillErr, "DecideFill: %v")
			if (plan != nil) != tt.wantFill {
				t.Fatalf("plan = %+v, wantFill = %v", plan, tt.wantFill)
			}
			if tt.wantFill && plan.Routes[0].ReservedAmountOut.String() != "9999" {
				t.Fatalf("private reservation = %s, want 9999", plan.Routes[0].ReservedAmountOut)
			}
		})
	}
}

func TestDecideFillRequiresExecutionDeadlineBuffer(t *testing.T) {
	strategy := testStrategy(t, Config{ExecutionDeadlineBuffer: "30s"})
	now := time.Unix(1_800_000_000, 0)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900),
			ChainTime: now, MaxFeePerGas: big.NewInt(0),
			Quotes: profitableFillQuotes(tokenIn, tokenOut),
		},

		Expires: uint32(now.Add(30 * time.Second).Unix()), FillDeadline: uint32(now.Add(time.Minute).Unix()),
	})
	testcheck.NoError(t, err, "DecideFill: %v")
	if plan != nil {
		t.Fatalf("expected near-expiry order to be skipped, got %+v", plan)
	}
}

func TestDecideQuotesPublishesFullSharedCapacityForEachPair(t *testing.T) {
	strategy := testStrategy(t, Config{MinAmount: "2"})
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
				Inventory:    inventory,
				Reservations: tt.reservations,
				MaxFeePerGas: big.NewInt(0), ChainTime: time.Unix(1_800_000_000, 0),
				QuoteExpiresAt: time.Unix(1_800_000_090, 0),
			})
			testcheck.NoError(t, err, "DecideQuotes: %v")
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
	strategy := testStrategy(t, Config{MinAmount: "2"})
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := make([]liquidlane.Inventory, 2)
	for index := range inventory {
		inventory[index] = liquidlane.Inventory{
			Route: liquidlane.Route{
				ID:         liquidlane.RouteID("route-" + strconv.Itoa(index+1)),
				CapacityID: "capacity-1",
				Adapter:    common.BytesToAddress([]byte{byte(index + 1)}),
				TokenIn:    tokenIn, TokenOut: tokenOut,
				TokenInDecimals: 6, TokenOutDecimals: 6,
			},
			MaxAssets: big.NewInt(1_000), MaxRate: big.NewInt(1_000_000_000_000_000_000),
		}
	}

	out, err := strategy.DecideQuotes(context.Background(), types.QuoteInput{
		Inventory:    inventory,
		MaxFeePerGas: big.NewInt(0), ChainTime: time.Unix(1_800_000_000, 0),
		QuoteExpiresAt: time.Unix(1_800_000_090, 0),
	})
	testcheck.NoError(t, err, "DecideQuotes: %v")
	if len(out.Quotes) != 1 {
		t.Fatalf("quotes = %d", len(out.Quotes))
	}
	if got := out.Quotes[0].Ranges[len(out.Quotes[0].Ranges)-1].MaxAmount.String(); got != "1000" {
		t.Fatalf("maxAmount = %s, want 1000", got)
	}
}

func TestDecideFillSeparatesBufferedTargetFromEconomicFloor(t *testing.T) {
	strategy := testStrategy(t, Config{PriceBufferBps: 100})
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10_000), OutputAmount: big.NewInt(9_600),
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
		},
	})
	testcheck.NoError(t, err, "DecideFill: %v")
	if plan == nil || len(plan.Routes) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Routes[0].ExpectedAmountOut.String() != "9900" ||
		plan.Routes[0].MinAmountOut.String() != "9601" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDecideFillRejectsDutchAuctionContext(t *testing.T) {
	strategy := testStrategy(t, Config{})
	for _, outputContext := range [][]byte{{dutchAuctionContextType}, {exclusiveDutchAuctionContextType}} {
		plan, decideErr := strategy.DecideFill(context.Background(), types.FillInput{
			FillInput: planning.FillInput{
				AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(990_000),
			},

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
	strategy := testStrategy(t, Config{})
	for _, outputContext := range [][]byte{
		{limitOrderContextType, 0x01},
		{exclusiveLimitOrderContextType},
		{0x02},
	} {
		plan, decideErr := strategy.DecideFill(context.Background(), types.FillInput{
			FillInput: planning.FillInput{
				AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(990_000),
			},

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
	strategy := testStrategy(t, Config{})
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	solver := common.HexToAddress("0x5555555555555555555555555555555555555555")
	otherSolver := common.HexToAddress("0x6666666666666666666666666666666666666666")

	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(990_000),
			ChainTime: time.Unix(1_800_000_000, 0), MaxFeePerGas: big.NewInt(0),
			Quotes: profitableFillQuotes(tokenIn, tokenOut),
		},

		Solver: solver,

		OutputContext: exclusiveLimitContext(otherSolver),
	})
	testcheck.NoError(t, err, "DecideFill: %v")
	if plan != nil {
		t.Fatalf("expected skip during another solver's exclusive window, got %+v", plan)
	}

	plan, err = strategy.DecideFill(context.Background(), types.FillInput{
		FillInput: planning.FillInput{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(990_000),
			ChainTime: time.Unix(1_800_000_011, 0), MaxFeePerGas: big.NewInt(0),
			Quotes: profitableFillQuotes(tokenIn, tokenOut),
		},

		Solver: solver,

		OutputContext: exclusiveLimitContext(otherSolver),
	})
	testcheck.NoError(t, err, "DecideFill after window: %v")
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
			testcheck.NoError(t, err, "parseOutputContext: %v")
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

func testStrategy(t *testing.T, cfg Config) *Strategy {
	t.Helper()
	strategy, err := New(cfg)
	testcheck.NoError(t, err, "New: %v")
	return strategy
}

func testGasPrices(token common.Address, amount int64) *liquidlanegas.PriceSnapshot {
	return liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{token: big.NewInt(amount)})
}

func TestSharedFillContracts(t *testing.T) {
	strategy, err := New(Config{})
	testcheck.NoError(t, err)
	strategytest.CheckFill(t, func(input planning.FillInput) ([]planning.FillRoute, error) {
		plan, err := strategy.DecideFill(t.Context(), types.FillInput{FillInput: input})
		if plan == nil {
			return nil, err
		}
		if len(plan.Routes) == 0 {
			t.Fatal("non-nil fill plan has no routes")
		}
		return plan.Routes, err
	})
}
