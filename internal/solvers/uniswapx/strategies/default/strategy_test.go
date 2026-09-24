package defaultstrategy

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

var quoteRateScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

func TestDefaultExecutionBufferIsOneBlock(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if strategy.executionBuffer != 12*time.Second {
		t.Fatalf("execution buffer = %s", strategy.executionBuffer)
	}
}

func TestDecideQuoteRequiresOneRequestedAmountAndFreshState(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	for _, input := range []types.QuoteInput{
		{ChainTime: now, QuoteExpiresAt: now},
		{ChainTime: now, QuoteExpiresAt: now.Add(time.Minute)},
		{ChainTime: now, QuoteExpiresAt: now.Add(time.Minute), AmountIn: big.NewInt(1), AmountOut: big.NewInt(1)},
	} {
		if _, quoteErr := strategy.DecideQuote(context.Background(), input); quoteErr == nil {
			t.Fatalf("input %+v: error = nil", input)
		}
	}
}

func TestDecideQuoteReturnsOneExactInputAmountWithBuffer(t *testing.T) {
	strategy, err := New(Config{PriceBufferBps: 100})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(1_000, 1_000, quoteRateScale)
	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if quote == nil || quote.AmountIn.String() != "1000" || quote.AmountOut.String() != "980" {
		t.Fatalf("quote = %+v", quote)
	}
}

func TestDecideQuoteSubtractsCompleteFillGas(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(1_000_000, 2_000_000, new(big.Int).Mul(big.NewInt(2), quoteRateScale))
	route := input.Inventory[0].Route
	input.MaxFeePerGas = big.NewInt(1)
	input.GasPrices = testGasPrices(route.TokenOut, 1_000_000_000_000_000_000)
	input.GasSnapshot = acquireGasSnapshot(route, 2_000_000)

	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if quote == nil || quote.AmountOut.String() != "1450000" {
		t.Fatalf("quote = %+v, want output 1450000", quote)
	}
}

func TestDecideQuoteExactOutputFindsInputIncludingGas(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(1, 2_000_000, new(big.Int).Mul(big.NewInt(2), quoteRateScale))
	input.AmountIn = nil
	input.AmountOut = big.NewInt(900_000)
	route := input.Inventory[0].Route
	input.MaxFeePerGas = big.NewInt(1)
	input.GasPrices = testGasPrices(route.TokenOut, 1_000_000_000_000_000_000)
	input.GasSnapshot = acquireGasSnapshot(route, 2_000_000)

	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if quote == nil || quote.AmountIn.String() != "725000" || quote.AmountOut.String() != "900000" {
		t.Fatalf("quote = %+v", quote)
	}
}

func TestDecideQuoteAggregatesRoutesOnlyWhenAllowed(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(900, 500, quoteRateScale)
	second := input.Inventory[0]
	second.ID = "route-2"
	second.CapacityID = "capacity-2"
	second.Adapter = common.HexToAddress("0x00000000000000000000000000000000000000a2")
	input.Inventory = append(input.Inventory, second)

	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil || quote == nil || quote.AmountOut.String() != "900" {
		t.Fatalf("multi-route quote = %+v, err %v", quote, err)
	}
	input.RequireSingleRoute = true
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote != nil {
		t.Fatalf("single-route quote = %+v, err %v", quote, err)
	}
}

func TestSingleQuoteUsesFullSharedCapacity(t *testing.T) {
	strategy, err := New(Config{Name: types.SingleName})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		exactOutput     bool
		otherPair       bool
		pending         int64
		wantIn, wantOut int64
	}{
		{"best exact input", false, false, 0, 40, 80},
		{"best exact output", true, false, 0, 40, 80},
		{"unrelated pair", false, true, 0, 40, 80},
		{"better source lacks volume", false, false, 30, 40, 40},
		{"all sources lack volume", true, false, 30, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := directQuoteInput(40, 100, new(big.Int).Mul(big.NewInt(2), quoteRateScale))
			other := input.Inventory[0]
			other.Route = testRoute("route-2", "capacity-1", 2, input.TokenIn, input.TokenOut)
			other.MaxRate = quoteRateScale
			if test.otherPair {
				other.TokenIn = common.HexToAddress("0x03")
			}
			input.Inventory = append(input.Inventory, other)
			input.Reservations = liquidlane.CapacityReservations{"capacity-1": big.NewInt(test.pending)}
			if test.exactOutput {
				input.AmountIn, input.AmountOut = nil, big.NewInt(80)
			}
			quote, quoteErr := strategy.DecideQuote(t.Context(), input)
			if quoteErr != nil || (quote == nil) != (test.wantIn == 0) {
				t.Fatalf("quote=%+v, err=%v", quote, quoteErr)
			}
			if quote != nil && (quote.AmountIn.Int64() != test.wantIn || quote.AmountOut.Int64() != test.wantOut || quote.CandidateID == "") {
				t.Fatalf("quote=%+v, want %d -> %d", quote, test.wantIn, test.wantOut)
			}
		})
	}
}

func TestLocalQuoteAndFillAgreeOnReservedPhysicalCapacity(t *testing.T) {
	for _, name := range []string{types.DefaultName, types.SingleName} {
		for _, buffer := range []int{0, 100} {
			t.Run(name+"/"+big.NewInt(int64(buffer)).String(), func(t *testing.T) {
				strategy, err := New(Config{Name: name, PriceBufferBps: buffer, InventoryReserveBps: buffer})
				if err != nil {
					t.Fatal(err)
				}
				amount := int64(20_000)
				input := directQuoteInput(amount, 40_000, new(big.Int).Mul(big.NewInt(2), quoteRateScale))
				id := common.HexToHash("0x01")
				input.Inventory[0].DiscountID = &id
				other := input.Inventory[0]
				other.Route = testRoute("route-2", "capacity-1", 2, input.TokenIn, input.TokenOut)
				other.MaxAssets, other.MaxRate, other.DiscountID = big.NewInt(100_000), quoteRateScale, nil
				input.Inventory = append(input.Inventory, other)
				input.Reservations = liquidlane.CapacityReservations{"capacity-1": big.NewInt(30_000)}
				quote, err := strategy.DecideQuote(t.Context(), input)
				if err != nil || quote == nil {
					t.Fatalf("quote=%+v err=%v", quote, err)
				}
				if buffer == 0 {
					want := int64(25_000)
					if name == types.SingleName {
						want = amount
					}
					if quote.AmountOut.Int64() != want {
						t.Fatalf("output=%s, want %d after source reservation", quote.AmountOut, want)
					}
				}
				narrow := directFillQuote(input.Inventory[0].Route, amount, 40_000, 2*amount)
				narrow.DiscountID = &id
				fillInput := types.FillInput{
					TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: quote.AmountIn, OutputAmount: quote.AmountOut,
					ChainTime: input.ChainTime, MaxFeePerGas: new(big.Int), Reservations: input.Reservations,
					Quotes:         []liquidlane.FillQuote{narrow, directFillQuote(other.Route, amount, 100_000, amount)},
					CapacityLimits: map[liquidlane.CapacityID]*big.Int{"capacity-1": big.NewInt(100_000)},
				}
				// A large domain budget must not let the narrow source spend its pending capacity again.
				blocked := fillInput
				blocked.Quotes = []liquidlane.FillQuote{narrow}
				blocked.OutputAmount = big.NewInt(2 * amount)
				if plan, err := strategy.DecideFill(t.Context(), blocked); err != nil || plan != nil {
					t.Fatalf("narrow source spent pending capacity: plan=%+v err=%v", plan, err)
				}
				plan, err := strategy.DecideFill(t.Context(), fillInput)
				if err != nil || plan == nil {
					t.Fatalf("quote %s -> %s cannot be filled: %v", quote.AmountIn, quote.AmountOut, err)
				}
				_, err = strategies.ValidateFillRoutes(strategies.FillValidation{
					TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: quote.AmountIn, RequiredAmountOut: quote.AmountOut,
					MaxRoutes: types.MaxRoutes, Quotes: fillInput.Quotes, Reservations: input.Reservations, CapacityLimits: fillInput.CapacityLimits,
				}, plan.Routes)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestDecideQuoteUsesCurrentCapacityAndReservations(t *testing.T) {
	strategy, err := New(Config{InventoryReserveBps: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(50, 100, quoteRateScale)
	input.Reservations = liquidlane.CapacityReservations{"capacity-1": big.NewInt(50)}
	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil || quote != nil {
		t.Fatalf("reserved quote = %+v, err %v", quote, err)
	}
	input.Reservations = nil
	input.AmountIn = big.NewInt(90)
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote == nil || quote.AmountOut.String() != "90" {
		t.Fatalf("reserve boundary quote = %+v, err %v", quote, err)
	}
	input.AmountIn = big.NewInt(91)
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote != nil {
		t.Fatalf("above reserve quote = %+v, err %v", quote, err)
	}
}

func TestDecideQuoteSplitsSharedCapacityAcrossPairs(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(100, 100, quoteRateScale)
	for index, tokenIn := range []common.Address{
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
	} {
		other := input.Inventory[0]
		other.ID = liquidlane.RouteID("other-" + tokenIn.Hex())
		other.Adapter = common.BytesToAddress([]byte{byte(index + 2)})
		other.TokenIn = tokenIn
		input.Inventory = append(input.Inventory, other)
	}

	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil || quote != nil {
		t.Fatalf("quote = %+v, err %v", quote, err)
	}

	input.AmountIn = big.NewInt(33)
	quote, err = strategy.DecideQuote(t.Context(), input)
	if err != nil || quote == nil || quote.AmountOut.Int64() != 33 {
		t.Fatalf("within-share quote = %+v, err %v", quote, err)
	}
	input.AmountIn = big.NewInt(100)
	input.AmountOut = input.AmountIn
	input.AmountIn = nil
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote != nil {
		t.Fatalf("exact-output quote = %+v, err %v", quote, err)
	}
}

func TestDecideQuoteKeepsMatchingRoutesWithinSharedCapacity(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	input := directQuoteInput(100, 100, quoteRateScale)
	second := input.Inventory[0]
	second.ID = "route-2"
	second.Adapter = common.HexToAddress("0x00000000000000000000000000000000000000a2")
	input.Inventory = append(input.Inventory, second)

	quote, err := strategy.DecideQuote(context.Background(), input)
	if err != nil || quote == nil || quote.AmountOut.String() != "100" {
		t.Fatalf("quote = %+v, err %v", quote, err)
	}
	input.AmountIn = big.NewInt(101)
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote != nil {
		t.Fatalf("over-capacity quote = %+v, err %v", quote, err)
	}
}

func TestDecideQuoteChoosesFreshPrivateAlternative(t *testing.T) {
	for _, name := range []string{types.DefaultName, types.SingleName} {
		t.Run(name, func(t *testing.T) {
			strategy, err := New(Config{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			input := directQuoteInput(100, 200, quoteRateScale)
			discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			private := input.Inventory[0]
			private.DiscountID = &discountID
			private.MaxRate = new(big.Int).Mul(big.NewInt(2), quoteRateScale)
			private.ValidUntil = input.QuoteExpiresAt.Add(time.Minute)
			input.Inventory = append(input.Inventory, private)

			quote, err := strategy.DecideQuote(context.Background(), input)
			if err != nil || quote == nil || quote.AmountOut.String() != "200" {
				t.Fatalf("private quote = %+v, err %v", quote, err)
			}
			input.Inventory[1].ValidUntil = input.QuoteExpiresAt.Add(time.Second)
			quote, err = strategy.DecideQuote(context.Background(), input)
			if err != nil || quote == nil || quote.AmountOut.String() != "100" {
				t.Fatalf("expired private fallback = %+v, err %v", quote, err)
			}
		})
	}
}
func TestDecideFillBuildsCurrentMultiRoutePlan(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	tokenIn, tokenOut := testPair()
	quotes := []liquidlane.FillQuote{
		directFillQuote(testRoute("route-1", "capacity-1", 1, tokenIn, tokenOut), 1_000, 500, 1_000),
		directFillQuote(testRoute("route-2", "capacity-2", 2, tokenIn, tokenOut), 1_000, 500, 1_000),
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900),
		ChainTime: time.Unix(1_800_000_000, 0), MaxFeePerGas: new(big.Int), Quotes: quotes,
	})
	if err != nil || plan == nil || len(plan.Routes) != 2 {
		t.Fatalf("plan = %+v, err %v", plan, err)
	}
	totalIn := new(big.Int)
	totalMinOut := new(big.Int)
	for _, route := range plan.Routes {
		totalIn.Add(totalIn, route.AmountIn)
		totalMinOut.Add(totalMinOut, route.MinAmountOut)
	}
	if totalIn.String() != "1000" || totalMinOut.String() != "900" {
		t.Fatalf("totals = %s/%s", totalIn, totalMinOut)
	}
}

func TestDecideFillDoesNotDoubleSpendSharedCapacity(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	tokenIn, tokenOut := testPair()
	quotes := []liquidlane.FillQuote{
		directFillQuote(testRoute("route-1", "shared", 1, tokenIn, tokenOut), 1_000, 600, 1_000),
		directFillQuote(testRoute("route-2", "shared", 2, tokenIn, tokenOut), 1_000, 600, 1_000),
	}
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900),
		MaxFeePerGas: new(big.Int), Quotes: quotes,
	})
	if err != nil || plan != nil {
		t.Fatalf("plan = %+v, err %v", plan, err)
	}
}

func TestDecideFillSelectsBestCurrentRoute(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	tokenIn, tokenOut := testPair()
	first := testRoute("route-1", "capacity-1", 1, tokenIn, tokenOut)
	best := testRoute("route-2", "capacity-2", 2, tokenIn, tokenOut)
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(900),
		MaxFeePerGas: new(big.Int), Quotes: []liquidlane.FillQuote{
			directFillQuote(first, 1_000, 2_000, 1_000),
			directFillQuote(best, 1_000, 2_000, 1_100),
		},
	})
	if err != nil || plan == nil || len(plan.Routes) != 1 || plan.Routes[0].Adapter != best.Adapter {
		t.Fatalf("plan = %+v, err %v", plan, err)
	}
}

func TestDecideFillHonorsPendingReservationAndDeadline(t *testing.T) {
	strategy, err := New(Config{ExecutionDeadlineBuffer: "30s"})
	if err != nil {
		t.Fatal(err)
	}
	tokenIn, tokenOut := testPair()
	now := time.Unix(1_800_000_000, 0)
	quote := directFillQuote(testRoute("route-1", "capacity-1", 1, tokenIn, tokenOut), 100, 100, 100)
	base := types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(90),
		ChainTime: now, MaxFeePerGas: new(big.Int), Quotes: []liquidlane.FillQuote{quote},
		Reservations: liquidlane.CapacityReservations{"capacity-1": big.NewInt(60)},
	}
	plan, err := strategy.DecideFill(context.Background(), base)
	if err != nil || plan != nil {
		t.Fatalf("reserved plan = %+v, err %v", plan, err)
	}
	base.Reservations = nil
	base.Deadline = uint32(now.Add(30 * time.Second).Unix())
	plan, err = strategy.DecideFill(context.Background(), base)
	if err != nil || plan != nil {
		t.Fatalf("near-deadline plan = %+v, err %v", plan, err)
	}
}

func TestDecideFillCommitsSelectedPrivateDiscount(t *testing.T) {
	strategy, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	tokenIn, tokenOut := testPair()
	route := testRoute("route-1", "capacity-1", 1, tokenIn, tokenOut)
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	direct := directFillQuote(route, 1_000, 1_000, 900)
	private := directFillQuote(route, 1_000, 1_000, 950)
	private.DiscountID = &discountID
	private.MinDiscount = big.NewInt(100_000)
	plan, err := strategy.DecideFill(context.Background(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000), OutputAmount: big.NewInt(850),
		MaxFeePerGas: new(big.Int), Quotes: []liquidlane.FillQuote{direct, private},
	})
	if err != nil || plan == nil || plan.Routes[0].DiscountID == nil || *plan.Routes[0].DiscountID != discountID {
		t.Fatalf("plan = %+v, err %v", plan, err)
	}
}

func directQuoteInput(amountIn, maxAssets int64, rate *big.Int) types.QuoteInput {
	tokenIn, tokenOut := testPair()
	now := time.Unix(1_800_000_000, 0)
	return types.QuoteInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(amountIn),
		Inventory: []liquidlane.Inventory{liquidlane.DirectInventory(
			testRoute("route-1", "capacity-1", 1, tokenIn, tokenOut), big.NewInt(maxAssets), rate,
		)},
		MaxFeePerGas: new(big.Int), ChainTime: now, QuoteExpiresAt: now.Add(time.Minute),
	}
}

func testPair() (tokenIn, tokenOut common.Address) {
	return common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222")
}

func testRoute(
	id liquidlane.RouteID,
	capacityID liquidlane.CapacityID,
	adapterByte byte,
	tokenIn, tokenOut common.Address,
) liquidlane.Route {
	return liquidlane.Route{
		ID: id, CapacityID: capacityID,
		Adapter: common.BytesToAddress([]byte{adapterByte}), Vault: common.BytesToAddress([]byte{adapterByte + 10}),
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 18, TokenOutDecimals: 18,
	}
}

func directFillQuote(route liquidlane.Route, amountIn, maxAssets, maxAmountOut int64) liquidlane.FillQuote {
	return liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(maxAssets)},
		AmountIn:  big.NewInt(amountIn), GrossAmountOut: big.NewInt(maxAmountOut), MaxAmountOut: big.NewInt(maxAmountOut),
	}
}

func acquireGasSnapshot(route liquidlane.Route, amount int64) *liquidlanegas.Snapshot {
	return &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			route.Adapter: {Vault: route.Vault, Acquire: map[common.Address]*big.Int{route.TokenIn: big.NewInt(amount)}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			route.Vault: {FreeAssets: new(big.Int), Withdrawable: new(big.Int)},
		},
	}
}

func testGasPrices(token common.Address, amount int64) *liquidlanegas.PriceSnapshot {
	return liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{token: big.NewInt(amount)})
}

func TestLocalStrategyAggregation(t *testing.T) {
	for _, name := range []string{types.DefaultName, types.SingleName} {
		t.Run(name, func(t *testing.T) {
			strategy, err := New(Config{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			input := directQuoteInput(1000, 600, quoteRateScale)
			other := input.Inventory[0]
			other.Route = testRoute("route-2", "capacity-2", 2, input.TokenIn, input.TokenOut)
			input.Inventory = append(input.Inventory, other)
			quote, err := strategy.DecideQuote(t.Context(), input)
			if err != nil || (quote != nil) != (name == types.DefaultName) {
				t.Fatalf("quote=%+v err=%v", quote, err)
			}
			if quote != nil && quote.CandidateID != "" {
				t.Fatal("aggregate quote identified one source")
			}
			plan, err := strategy.DecideFill(t.Context(), types.FillInput{
				TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
				OutputAmount: big.NewInt(900), MaxFeePerGas: new(big.Int),
				Quotes: []liquidlane.FillQuote{directFillQuote(input.Inventory[0].Route, 1000, 600, 1000), directFillQuote(other.Route, 1000, 600, 1000)},
			})
			if err != nil || (plan != nil) != (name == types.DefaultName) {
				t.Fatalf("fill=%+v err=%v", plan, err)
			}
			if plan != nil && len(plan.Routes) != 2 {
				t.Fatalf("routes=%+v", plan.Routes)
			}
		})
	}
	if strategy, err := New(Config{Name: "unsupported"}); err == nil || strategy != nil {
		t.Fatal("unknown local strategy accepted")
	}
}

func TestLocalStrategyBufferAndGasPolicy(t *testing.T) {
	for _, name := range []string{types.DefaultName, types.SingleName} {
		t.Run(name, func(t *testing.T) {
			s, err := New(Config{Name: name, PriceBufferBps: 100})
			if err != nil {
				t.Fatal(err)
			}
			input := directQuoteInput(1_000, 2_000, quoteRateScale)
			quote, err := s.DecideQuote(t.Context(), input)
			if err != nil || quote == nil || quote.AmountOut.Int64() != 980 {
				t.Fatalf("quote=%+v err=%v; want 2x quote buffer", quote, err)
			}
			for _, test := range []struct{ required, fee int64 }{{990, 0}, {991, 0}, {990, 1}} {
				plan, err := s.DecideFill(t.Context(), types.FillInput{
					TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
					OutputAmount: big.NewInt(test.required), MaxFeePerGas: big.NewInt(test.fee),
					GasPrices: testGasPrices(input.TokenOut, 1_000_000_000_000_000_000),
					Quotes:    []liquidlane.FillQuote{directFillQuote(input.Inventory[0].Route, 1_000, 2_000, 1_000)},
				})
				want := test.required == 990 && (test.fee == 0 || name == types.SingleName)
				if err != nil || (plan != nil) != want {
					t.Fatalf("required=%d fee=%d plan=%+v err=%v", test.required, test.fee, plan, err)
				}
				if plan != nil && plan.Routes[0].MinAmountOut.Int64() != test.required {
					t.Fatal("single fill added a gas repayment margin")
				}
			}
		})
	}
}
