package filler

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"
)

var settleSelector = [4]byte{0xaa, 0xbb, 0xcc, 0xdd}

type swapFixture struct {
	swaps     *Swaps
	chain     *connectortest.Chain
	sender    *fillertest.Sender
	discounts *fillertest.Discounts
	swap      Swap
	calls     []execution.Call
}

func newSwapFixture(t *testing.T) *swapFixture {
	t.Helper()
	workflow, chain := fillertest.Workflow(t, settleSelector)
	f := &swapFixture{chain: chain, sender: &fillertest.Sender{}, discounts: fillertest.NewDiscounts()}
	f.swaps = &Swaps{
		Engine:    &Engine{Name: "test", Workflow: workflow, Routes: connector.NewRoutes(workflow, []common.Address{fillertest.RecipeExecutor}), Spends: connector.NewSpendLedger(), Sender: f.sender},
		Discounts: f.discounts,
	}
	if err := f.swaps.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.swap = Swap{
		OrderID: "order", TokenIn: connectortest.TokenIn, AmountIn: big.NewInt(123), TokenOut: connectortest.TokenOut,
		MinAmountOut: big.NewInt(480), Expiry: time.Unix(4102444800, 0),
	}
	return f
}

func (f *swapFixture) envelope(calls []execution.Call) ([]byte, error) {
	f.calls = calls
	data := bytes.Clone(settleSelector[:])
	for _, call := range calls {
		data = append(data, call.Data...)
	}
	return data, nil
}

func TestSwapFundsConnectorThenCallsRecipe(t *testing.T) {
	f := newSwapFixture(t)
	intent, err := f.swaps.Fill(t.Context(), f.swap, f.envelope)
	if err != nil || intent == nil {
		t.Fatal(intent, err)
	}
	if f.chain.OuterCalls != 1 || !bytes.Equal(f.chain.OuterData, intent.Data) {
		t.Fatal("complete settlement call was not simulated unchanged")
	}
	if len(f.calls) != 2 || f.calls[0].Value.Sign() != 0 || f.calls[1].Value.Sign() != 0 {
		t.Fatalf("calls = %d, want funding then recipe", len(f.calls))
	}
	wantFunding := erc4626.NewIERC4626().PackTransfer(connectortest.Venue, big.NewInt(123))
	if f.calls[0].Target != connectortest.TokenIn || !bytes.Equal(f.calls[0].Data, wantFunding) {
		t.Fatal("order input is not forwarded to the connector")
	}
	if f.calls[1].Target != f.chain.RecipeExecutor {
		t.Fatal("recipe is not called directly by its configured caller")
	}
	payload, err := recipeexecutor.NewRecipeExecutor().GetABI().Methods["execute"].Inputs.Unpack(f.calls[1].Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if inputs := payload[2].([]*big.Int); len(inputs) != 2 || inputs[0].Int64() != 123 || inputs[1].Int64() != 490 {
		t.Fatalf("recipe inputs = %v, want the whole input and the connector payment", inputs)
	}
	capacity := liquidlane.NewCapacityID(1, f.chain.RecipeAccount, connectortest.TokenOut)
	if amount := intent.Spend.Amounts[capacity]; len(intent.Spend.Amounts) != 1 || amount == nil || amount.Int64() != 490 || intent.Spend.Deadline != 4102444800 {
		t.Fatalf("spend = %+v, want the connector payment against the recipe account", intent.Spend)
	}
	if reserved := f.swaps.Reservations(nil)[capacity]; reserved == nil || reserved.Int64() != 490 {
		t.Fatal("admitted fill did not reserve its payment")
	}
	if f.sender.Submits != 1 || f.sender.Request.To != fillertest.Settlement || !bytes.Equal(f.sender.Request.Data, intent.Data) || intent.Deadline.IsZero() {
		t.Fatal("validated settlement was not submitted unchanged")
	}
	if again, err := f.swaps.Fill(t.Context(), f.swap, f.envelope); !errors.Is(err, ErrBusy) || again != nil || f.discounts.Lists != 1 {
		t.Fatal("busy lane planned another fill", err)
	}
	f.sender.Result = fillertest.Confirmed()
	if settled, err := f.swaps.Await(t.Context()); err != nil || settled == nil || settled.Reverted {
		t.Fatal(settled, err)
	}
	if again, err := f.swaps.Fill(t.Context(), f.swap, f.envelope); err != nil || again != nil || f.sender.Submits != 1 {
		t.Fatal("settled order was filled again", err)
	}
}

func TestSwapRejectsUnfillableOrders(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*swapFixture)
		want   string
		is     error
		outer  int
		// submits counts fills that reached the lane and were refused there.
		submits int
		// reserved counts capacities the ledger held before the order arrived.
		reserved int
	}{
		{name: "payment below order output", mutate: func(f *swapFixture) { f.swap.MinAmountOut = big.NewInt(491) }, want: "connectors pay 490 below order output 491", is: ErrNotFillable},
		{name: "missing output", mutate: func(f *swapFixture) { f.swap.MinAmountOut = nil }, want: "no output amount", is: ErrNotFillable},
		{name: "expired", mutate: func(f *swapFixture) { f.swap.Expiry = time.Unix(7, 0) }, want: "expiry 7 is not after chain time 7", is: ErrNotFillable},
		{name: "elapsed in wall time", mutate: func(f *swapFixture) { f.swap.Expiry = time.Unix(1000, 0) }, want: "order expiry 1000, signed terms valid until 4102444800", is: ErrNotFillable, outer: 1},
		{name: "nothing advertised", mutate: func(f *swapFixture) { f.discounts.Listed = nil }, want: "0 connector discounts listed for", is: ErrNotFillable},
		{name: "native precision", mutate: func(f *swapFixture) { f.discounts.Listed[0].DiscountPrecision = "1000000" }, want: "1 with precision other than 100000000", is: ErrNotFillable},
		{name: "insufficient capacity", mutate: func(f *swapFixture) { f.discounts.Listed[0].MaxAssets = "489" }, want: "maxAssets 489 below required output 490; split: disabled, order allows 0 route(s)", is: ErrNotFillable},
		{name: "recipe caller differs", mutate: func(f *swapFixture) { f.chain.RecipeCaller = common.HexToAddress("0x88") }, want: "prepare connector settlement"},
		{name: "settlement reverts", mutate: func(f *swapFixture) { f.chain.OuterErr = errors.New("execution reverted") }, want: "simulation", is: ErrSimulation, outer: 1},
		{name: "settlement revert is decoded", mutate: func(f *swapFixture) { f.chain.OuterErr = connectortest.Revert("InvalidSwapRate()") }, want: "(revert InvalidSwapRate())", is: ErrSimulation, outer: 1},
		{name: "signed terms unavailable", mutate: func(f *swapFixture) { f.discounts.ResolveErr = errors.New("backend timeout") }, want: "backend timeout"},
		{name: "expires too soon", mutate: func(f *swapFixture) { f.swaps.MinValidity = 200 * 365 * 24 * time.Hour }, want: "expires within", is: ErrNotFillable, outer: 1},
		{name: "signed discount expires before the order", mutate: func(f *swapFixture) {
			f.swap.Expiry, f.swaps.MinValidity = time.Unix(4102444900, 0), 200*365*24*time.Hour
		}, want: "expires within", is: ErrTermsExpiring, outer: 1},
		{name: "funding spent by this lane's last fill", mutate: func(f *swapFixture) {
			capacity := liquidlane.NewCapacityID(1, f.chain.RecipeAccount, connectortest.TokenOut)
			// Included at block 8; the listing still shows the funding from before it.
			if err := f.swaps.Spends.Include("earlier", connector.Spend{Amounts: liquidlane.CapacityReservations{capacity: big.NewInt(480)}, IncludedAt: 8, Deadline: 4102444800}); err != nil {
				panic(err)
			}
		}, want: "unreserved 420 below order output 480", is: ErrNotFillable, reserved: 1},
		{name: "split with one connector", mutate: func(f *swapFixture) {
			f.discounts.Listed[0].MaxAssets, f.swap.MaxRoutes = "489", 2
		}, want: "a split needs two connectors, 1 distinct listed", is: ErrNotFillable},
		{name: "refused before signing", mutate: func(f *swapFixture) {
			f.sender.Unsigned, f.sender.SubmitErr = true, errors.New("gas estimation reverted")
		}, want: "gas estimation", is: ErrSubmission, outer: 1, submits: 1},
		{name: "gas estimate revert is decoded", mutate: func(f *swapFixture) {
			f.sender.Unsigned, f.sender.SubmitErr = true, protocolio.Failure("estimate transaction gas", connectortest.Revert("ExpiredSwap()"))
		}, want: "estimate transaction gas failed: JSON-RPC error rpc_code=3 classification=execution reverted (revert ExpiredSwap())", is: ErrSubmission, outer: 1, submits: 1},
		{name: "malformed listing", mutate: func(f *swapFixture) { f.discounts.Listed[0].MaxAssets = "many" }, want: "maxAssets"},
		{name: "discount service down", mutate: func(f *swapFixture) { f.discounts.ListErr = errors.New("unavailable") }, want: "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSwapFixture(t)
			tc.mutate(f)
			intent, err := f.swaps.Fill(t.Context(), f.swap, f.envelope)
			if err == nil || intent != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("intent=%v error=%v, want %s", intent, err, tc.want)
			}
			if tc.is != nil && !errors.Is(err, tc.is) || tc.is == nil && (errors.Is(err, ErrNotFillable) || errors.Is(err, ErrSimulation) || errors.Is(err, ErrSubmission)) {
				t.Fatalf("error = %v, want class %v", err, tc.is)
			}
			if f.chain.OuterCalls != tc.outer || f.sender.Submits != tc.submits || f.swaps.Pending() != nil || len(f.swaps.Reservations(nil)) != tc.reserved {
				t.Fatalf("rejected order reached the lane: simulations=%d submits=%d", f.chain.OuterCalls, f.sender.Submits)
			}
		})
	}
}

func TestLaneRequiresDiscountServiceAndHost(t *testing.T) {
	cfg := Config{Executor: fillertest.Settlement, RecipeExecutors: []common.Address{fillertest.RecipeExecutor}}
	cfg.Curator.URL = "https://curator.example"
	if _, err := NewSwaps("test", cfg, nil, solver.Deps{}); err == nil || !strings.Contains(err.Error(), "discount service") {
		t.Fatal("lane built without a discount service", err)
	}
	if _, err := NewSwaps("test", cfg, fillertest.NewDiscounts(), solver.Deps{}); err == nil || !strings.Contains(err.Error(), "host chain") {
		t.Fatal("lane built without host dependencies", err)
	}
	if _, err := New("test", Config{}, solver.Deps{}); err == nil {
		t.Fatal("lane built without a settlement contract")
	}
}

func TestInventoryPricesAdvertisedConnectorDiscounts(t *testing.T) {
	f := newSwapFixture(t)
	f.discounts.Listed[0].Discount, f.discounts.Listed[0].CollateralDecimals = "2000000", 6
	f.discounts.Listed[0].BlockNumber, f.discounts.Listed[0].Deadline = 5, 4102444800
	native := f.discounts.Listed[0]
	native.DiscountPrecision = "1000000"
	wrongDecimals := f.discounts.Listed[0]
	wrongDecimals.DiscountID, wrongDecimals.CollateralDecimals = common.HexToHash("0x98").Hex(), 8
	f.swaps.Decimals = func(_ context.Context, token common.Address) (int, error) {
		if token == connectortest.TokenOut {
			return 6, nil
		}
		return 18, nil
	}
	inventory, err, readErr := f.swaps.Inventory(t.Context(), []discounts.ListItem{native, wrongDecimals, f.discounts.Listed[0]})
	if readErr != nil || len(inventory) != 1 || err == nil || !strings.Contains(err.Error(), "advertised output decimals 8 differ from token decimals 6") {
		t.Fatalf("inventory=%d err=%v, want one priced offer and the skipped listing reported", len(inventory), err)
	}
	item := inventory[0]
	if item.RecipeExecutor != fillertest.RecipeExecutor || item.DiscountPrecision != connector.Precision || item.Discount.Int64() != 2_000_000 ||
		item.MaxAssets.Int64() != 900 || item.BlockNumber != 5 || item.TokenInDecimals != 18 || item.TokenOutDecimals != 6 || item.ValidUntil.Unix() != 4102444800 {
		t.Fatalf("inventory = %+v", item)
	}
	malformed := f.discounts.Listed[0]
	malformed.DiscountID = "0x00"
	if _, _, err := f.swaps.Inventory(t.Context(), []discounts.ListItem{malformed}); err == nil {
		t.Fatal("malformed listing was skipped silently")
	}
}

// Neither connector can pay the whole order, but together they can: the fill funds each
// connector with its share of the input and runs both recipes in one settlement call.
func TestSwapSplitsAcrossConnectors(t *testing.T) {
	newPair := func(t *testing.T) (*Swaps, connectortest.Group, *fillertest.Sender, Swap) {
		t.Helper()
		workflow, chains, provider := fillertest.WorkflowPair(t, settleSelector)
		chains[0].Remaining, chains[1].Remaining = big.NewInt(500), big.NewInt(600)
		provider.Listed[0].MaxAssets, provider.Listed[1].MaxAssets = "500", "600"
		sender := &fillertest.Sender{}
		swaps := &Swaps{
			Engine: &Engine{
				Name: "test", Workflow: workflow, Spends: connector.NewSpendLedger(), Sender: sender,
				Routes:   connector.NewRoutes(workflow, []common.Address{fillertest.RecipeExecutor, fillertest.SecondRecipeExecutor}),
				Decimals: func(context.Context, common.Address) (int, error) { return 18, nil },
			},
			Discounts: provider,
		}
		if err := swaps.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
		return swaps, chains, sender, Swap{
			OrderID: "order", TokenIn: connectortest.TokenIn, AmountIn: big.NewInt(1000), TokenOut: connectortest.TokenOut,
			MinAmountOut: big.NewInt(970), Expiry: time.Unix(4102444800, 0),
		}
	}
	recorded := &swapFixture{}
	envelope := recorded.envelope

	t.Run("one route allowed", func(t *testing.T) {
		swaps, chains, sender, swap := newPair(t)
		swap.MaxRoutes = 1
		if intent, err := swaps.Fill(t.Context(), swap, envelope); !errors.Is(err, ErrNotFillable) || intent != nil || sender.Submits != 0 || chains[0].OuterCalls != 0 ||
			!strings.Contains(err.Error(), "split: disabled, order allows 1 route(s)") {
			t.Fatalf("single-route order was split or filled: intent=%v err=%v", intent, err)
		}
	})
	t.Run("split", func(t *testing.T) {
		swaps, chains, sender, swap := newPair(t)
		swap.MaxRoutes = 2
		intent, err := swaps.Fill(t.Context(), swap, envelope)
		if err != nil || intent == nil {
			t.Fatal(intent, err)
		}
		calls := recorded.calls
		if len(calls) != 4 || chains[0].OuterCalls != 1 || sender.Submits != 1 || !bytes.Equal(sender.Request.Data, intent.Data) {
			t.Fatalf("calls=%d simulations=%d submits=%d, want two funded recipes in one simulated call", len(calls), chains[0].OuterCalls, sender.Submits)
		}
		transfer := erc4626.NewIERC4626().GetABI().Methods["transfer"]
		funded, paid := new(big.Int), new(big.Int)
		venues := map[common.Address]bool{}
		for leg := range 2 {
			funding, recipe := calls[2*leg], calls[2*leg+1]
			args, err := transfer.Inputs.Unpack(funding.Data[4:])
			if err != nil || funding.Target != connectortest.TokenIn {
				t.Fatalf("leg %d is not funded with the order input: %v", leg, err)
			}
			venue, amountIn := args[0].(common.Address), args[1].(*big.Int)
			venues[venue] = true
			funded.Add(funded, amountIn)
			chain := chains[0]
			if venue == fillertest.SecondVenue {
				chain = chains[1]
			}
			if recipe.Target != chain.RecipeExecutor {
				t.Fatalf("leg %d funds %s but calls another connector's recipe", leg, venue)
			}
			payload, err := recipeexecutor.NewRecipeExecutor().GetABI().Methods["execute"].Inputs.Unpack(recipe.Data[4:])
			if err != nil {
				t.Fatal(err)
			}
			inputs := payload[2].([]*big.Int)
			// Each connector pays 98% of the input it receives, floored, within its budget.
			want := new(big.Int).Div(new(big.Int).Mul(amountIn, big.NewInt(98)), big.NewInt(100))
			if inputs[0].Cmp(amountIn) != 0 || inputs[1].Cmp(want) != 0 || inputs[1].Cmp(chain.Remaining) > 0 {
				t.Fatalf("leg %d recipe inputs = %v for input %s, budget %s", leg, inputs, amountIn, chain.Remaining)
			}
			paid.Add(paid, inputs[1])
			capacity := liquidlane.NewCapacityID(1, chain.RecipeAccount, connectortest.TokenOut)
			if spent := intent.Spend.Amounts[capacity]; spent == nil || spent.Cmp(inputs[1]) != 0 {
				t.Fatalf("leg %d spend = %v, want %s against its funding account", leg, spent, inputs[1])
			}
		}
		if len(venues) != 2 || funded.Cmp(swap.AmountIn) != 0 || paid.Cmp(swap.MinAmountOut) < 0 || len(intent.Spend.Amounts) != 2 {
			t.Fatalf("legs fund %s of %s and pay %s for an order output of %s", funded, swap.AmountIn, paid, swap.MinAmountOut)
		}
		// Neither connector is funded for the whole output, so neither was tried alone.
		if resolves := swaps.Discounts.(*fillertest.Discounts).Resolves; resolves != 2 {
			t.Fatalf("signed terms requested %d times, want once per split leg", resolves)
		}
	})
	t.Run("connector cannot be priced", func(t *testing.T) {
		swaps, chains, sender, swap := newPair(t)
		swap.MaxRoutes = 2
		chains[1].AmountOutFor = nil // its oracle read fails
		intent, err := swaps.Fill(t.Context(), swap, envelope)
		if err == nil || errors.Is(err, ErrNotFillable) || intent != nil || sender.Submits != 0 {
			t.Fatalf("a read failure that may hide the rest of the split was declined: intent=%v err=%v", intent, err)
		}
	})
	t.Run("connectors cannot cover the order together", func(t *testing.T) {
		swaps, chains, sender, swap := newPair(t)
		swap.MaxRoutes = 2
		chains[1].Remaining = big.NewInt(400)
		if intent, err := swaps.Fill(t.Context(), swap, envelope); !errors.Is(err, ErrNotFillable) || intent != nil || sender.Submits != 0 || !strings.Contains(err.Error(), "split found no allocation (2 of 2 connectors priced, 2 live, 2 with unreserved capacity, 2 quotable): liquidlane quote declined reason=insufficient-capacity") {
			t.Fatalf("underfunded order was filled: intent=%v err=%v", intent, err)
		}
	})
}

func TestObserveLogsRetiredSpends(t *testing.T) {
	var lines []string
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(func(obj string) { lines = append(lines, obj) }, funcr.Options{Verbosity: 1}))
	f := newSwapFixture(t)
	capacity := liquidlane.NewCapacityID(1, f.chain.RecipeAccount, connectortest.TokenOut)
	if err := f.swaps.Spends.ReserveQueued("award", connector.Spend{Amounts: liquidlane.CapacityReservations{capacity: big.NewInt(1)}, Deadline: 5}); err != nil {
		t.Fatal(err)
	}
	observed, err := f.swaps.Observe(ctx)
	if err != nil || f.swaps.Spends.Queued("award") {
		t.Fatalf("queued award outlived its deadline: %v", err)
	}
	want := fmt.Sprintf(`"msg":"connector spend retired","orderId":"award","kind":"queued award","reason":"signed deadline passed","deadline":5,"chainTime":%d`, observed.Chain.Unix())
	if len(lines) != 1 || !strings.Contains(lines[0], want) {
		t.Fatalf("logs = %v, want %s", lines, want)
	}
}
