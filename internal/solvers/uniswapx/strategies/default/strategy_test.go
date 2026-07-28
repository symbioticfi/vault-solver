package defaultstrategy

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
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

func TestDecideQuoteDoesNotSplitCapacityWithUnrelatedPairs(t *testing.T) {
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
	if err != nil || quote == nil || quote.AmountOut.String() != "100" {
		t.Fatalf("quote = %+v, err %v", quote, err)
	}

	input.AmountOut = input.AmountIn
	input.AmountIn = nil
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote == nil || quote.AmountIn.String() != "100" {
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
	strategy, err := New(Config{})
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
	input.Inventory[1].ValidUntil = input.QuoteExpiresAt
	quote, err = strategy.DecideQuote(context.Background(), input)
	if err != nil || quote == nil || quote.AmountOut.String() != "100" {
		t.Fatalf("expired private fallback = %+v, err %v", quote, err)
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
