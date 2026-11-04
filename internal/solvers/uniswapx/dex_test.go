package uniswapx

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

// A non-linear AMM fixture makes a 1:1 bridge assumption and linear inverse rounding fail.
type testDexMarket struct {
	reserveIn, reserveOut *big.Int
	stale                 bool
	lastAmount            *big.Int
	lastExactOutput       bool
}

func (d *testDexMarket) validate(context.Context, common.Address) error { return nil }

func (d *testDexMarket) quote(_ context.Context, _ common.Address, amount *big.Int, exactOut bool) (dexQuote, error) {
	d.lastAmount, d.lastExactOutput = new(big.Int).Set(amount), exactOut
	var value *big.Int
	if exactOut {
		value = liquidlane.MulDivUp(amount, d.reserveIn, new(big.Int).Sub(d.reserveOut, amount))
	} else {
		value = new(big.Int).Div(new(big.Int).Mul(amount, d.reserveOut), new(big.Int).Add(d.reserveIn, amount))
	}
	at := time.Now().Truncate(time.Second)
	if d.stale {
		at = at.Add(-time.Minute)
	}
	return dexQuote{Amount: value, BlockTime: at}, nil
}

func dexPricingFixture(t *testing.T, outputDecimals int) (*Solver, types.QuoteInput, *testDexMarket) {
	t.Helper()
	input, bridge, output := common.HexToAddress("0x11"), common.HexToAddress("0x22"), common.HexToAddress("0x33")
	strategy, err := defaultstrategy.New(defaultstrategy.Config{Name: types.SingleName, ExecutionDeadlineBuffer: "1s"})
	if err != nil {
		t.Fatal(err)
	}
	s := newQuoteTestSolver(t, input, &quoteTestStrategy{})
	s.strategy = strategy
	s.cfg.Strategy.Name = types.SingleName
	s.cfg.Dex = &DexConfig{InputToken: input, BridgeToken: bridge, Outputs: [2]DexOutput{{Token: output, Fee: 100}},
		MaxBridgeAmount: big.NewInt(1_000_000_000), MinProfitBridge: big.NewInt(100_000), SlippageBps: 50,
		GasUnitsBudget: 500_000, PlanValidity: 30 * time.Second, RPCTimeout: 300 * time.Millisecond}
	route := liquidlane.NewRoute(1, common.HexToAddress("0x44"), common.HexToAddress("0x55"), input, bridge, 6, 6)
	price := big.NewInt(1_100_000_000_000_000_000)
	inventory := liquidlane.DirectInventory(route, big.NewInt(1_000_000_000), price)
	inventory.Price, inventory.AdapterMinDiscount = price, new(big.Int)
	inventory.Discount = new(big.Int)
	market := &testDexMarket{reserveIn: big.NewInt(1_000_000_000_000),
		reserveOut: new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(outputDecimals+6)), nil)}
	s.dex = market
	now := time.Now().Truncate(time.Second)
	q := types.QuoteInput{TokenIn: input, TokenOut: output, RequireSingleRoute: true,
		Inventory: []liquidlane.Inventory{inventory}, MaxFeePerGas: big.NewInt(1_000_000_000),
		GasPrices: liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{bridge: big.NewInt(2_000_000_000)}),
		ChainTime: now, QuoteExpiresAt: now.Add(30 * time.Second)}
	return s, q, market
}

func TestDexQuoteExactInputUsesBridgeBudgetAndFinalDecimals(t *testing.T) {
	for _, decimals := range []int{6, 18} {
		s, input, market := dexPricingFixture(t, decimals)
		input.AmountIn = big.NewInt(10_000_000) // PRIME -> 11 USDC, less 1 gas + 0.1 profit.
		quote, err := s.decideDexQuote(t.Context(), input)
		if err != nil || quote == nil {
			t.Fatalf("decimals=%d quote=%+v err=%v", decimals, quote, err)
		}
		budget := big.NewInt(9_900_000)
		want := new(big.Int).Div(new(big.Int).Mul(budget, market.reserveOut), new(big.Int).Add(market.reserveIn, budget))
		want.Mul(want, big.NewInt(9_950)).Div(want, big.NewInt(10_000))
		if market.lastExactOutput || market.lastAmount.Cmp(budget) != 0 || quote.AmountOut.Cmp(want) != 0 || quote.AmountIn.Cmp(input.AmountIn) != 0 {
			t.Fatalf("decimals=%d got=%s want=%s budget=%s", decimals, quote.AmountOut, want, market.lastAmount)
		}
	}
}

func TestDexQuoteExactOutputInvertsBothLegsWithCeil(t *testing.T) {
	for _, decimals := range []int{6, 18} {
		s, input, market := dexPricingFixture(t, decimals)
		input.AmountOut = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		quote, err := s.decideDexQuote(t.Context(), input)
		if err != nil || quote == nil {
			t.Fatalf("decimals=%d quote=%+v err=%v", decimals, quote, err)
		}
		cost := liquidlane.MulDivUp(input.AmountOut, market.reserveIn, new(big.Int).Sub(market.reserveOut, input.AmountOut))
		needed := new(big.Int).Add(dexSpendCap(cost, 50), big.NewInt(1_100_000))
		want := liquidlane.MinAmountInForDiscountedAmountOut(needed, input.Inventory[0].Price, new(big.Int), 6, 6)
		if !market.lastExactOutput || quote.AmountIn.Cmp(want) != 0 || quote.AmountOut.Cmp(input.AmountOut) != 0 {
			t.Fatalf("decimals=%d input=%s want=%s output=%s", decimals, quote.AmountIn, want, quote.AmountOut)
		}
	}
}

func TestDexQuoteFailsClosedForStalePriceAndInsufficientEconomics(t *testing.T) {
	for _, mode := range []string{"stale", "gas exceeds payout", "bridge cap", "missing gas price"} {
		t.Run(mode, func(t *testing.T) {
			s, input, market := dexPricingFixture(t, 6)
			input.AmountIn = big.NewInt(10_000_000)
			switch mode {
			case "stale":
				market.stale = true
			case "gas exceeds payout":
				input.AmountIn = big.NewInt(1)
			case "bridge cap":
				s.cfg.Dex.MaxBridgeAmount = big.NewInt(1)
			case "missing gas price":
				input.GasPrices = nil
			}
			quote, _ := s.decideDexQuote(t.Context(), input)
			if quote != nil {
				t.Fatalf("unfunded/stale quote=%+v", quote)
			}
		})
	}
}

func TestDexHTTPQuoteKeepsFinalPairWhileUsingBridgeInventory(t *testing.T) {
	s, input, _ := dexPricingFixture(t, 18)
	s.quoteState.Store(&quoteState{inventory: input.Inventory, expiresAt: input.QuoteExpiresAt,
		chainTime: input.ChainTime, maxFeePerGas: input.MaxFeePerGas, gasPrices: input.GasPrices,
		singleRouteFor: map[common.Address]bool{input.TokenIn: true}})
	request := validQuoteRequest(input.TokenIn, input.TokenOut)
	request.Amount = "10000000"
	response, err := s.evaluateQuote(t.Context(), request)
	if err != nil || response.AmountOut == "0" || !response.pairInScope || response.TokenOut != input.TokenOut.Hex() {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if s.sourceTokenOut(input.TokenIn, input.TokenOut) != s.cfg.Dex.BridgeToken {
		t.Fatal("fill discovery must select the bridge asset")
	}
}

func TestDexSpendCapRoundsUpWithoutMutatingInput(t *testing.T) {
	amount := big.NewInt(1)
	if got := dexSpendCap(amount, 1); got.Int64() != 2 || amount.Int64() != 1 {
		t.Fatalf("cap=%s input=%s", got, amount)
	}
}

type rejectingDexStartup struct{ dexQuoter }

func (*rejectingDexStartup) validate(context.Context, common.Address) error {
	return errors.New("immutable mismatch sentinel")
}

func TestDexStartupValidatesEvenWithoutDirectAdapters(t *testing.T) {
	s := &Solver{cfg: &Config{SolverMode: solverModeInternal, Dex: &DexConfig{}, Discounts: &DiscountConfig{}},
		reader: &startupChainReader{}, dex: &rejectingDexStartup{}, log: logr.Discard()}
	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "immutable mismatch sentinel") {
		t.Fatalf("startup error = %v", err)
	}
}
