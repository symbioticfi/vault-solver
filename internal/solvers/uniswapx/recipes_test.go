package uniswapx

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type recipeSolverFixture struct {
	solver    *Solver
	lane      *filler.Swaps
	chain     *connectortest.Chain
	sender    *fillertest.Sender
	discounts *fillertest.Discounts
	order     *resolvedOrder
}

func newRecipeSolver(t *testing.T) recipeSolverFixture {
	t.Helper()
	selector := [4]byte(uniswapXExecutor.GetABI().Methods["execute"].ID)
	workflow, chain := fillertest.Workflow(t, selector)
	sender, listed := &fillertest.Sender{}, fillertest.NewDiscounts()
	listed.Listed[0].Discount, listed.Listed[0].CollateralDecimals = "2000000", 6
	listed.Listed[0].Deadline = 4102444800
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatal(err)
	}
	lane := &filler.Swaps{
		Engine: &filler.Engine{
			Name: "uniswapx-merkle", Workflow: workflow, Spends: connector.NewSpendLedger(), Sender: sender,
			Routes: connector.NewRoutes(workflow, []common.Address{fillertest.RecipeExecutor}),
		},
		Discounts: listed,
	}
	if err := lane.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	solver := &Solver{
		name: "uniswapx-merkle", chainID: 1, discounts: listed, reader: &countingChainReader{}, log: logr.Discard(),
		cfg: &Config{
			Executor: fillertest.Settlement, SolverMode: solverModeInternal, TokenPolicy: policy,
			QuoteServer: QuoteServerConfig{HTTPTimeout: time.Second, QuoteTTL: time.Minute},
			Discounts:   &DiscountConfig{HTTPTimeout: time.Second},
		},
	}
	lane.Decimals = func(_ context.Context, token common.Address) (int, error) {
		if token == connectortest.TokenOut {
			return 6, nil
		}
		return 18, nil
	}
	solver.settlement = &recipeSettlement{s: solver, lane: lane}
	return recipeSolverFixture{
		solver: solver, lane: lane, chain: chain, sender: sender, discounts: listed,
		order: &resolvedOrder{
			Encoded: []byte{0x01, 0x02}, Signature: []byte{0x03}, Hash: common.HexToHash("0xabc"), Executor: fillertest.Settlement,
			TokenIn: connectortest.TokenIn, TokenOut: connectortest.TokenOut, AmountIn: big.NewInt(123), AmountOut: big.NewInt(480),
			Deadline: 4102444800,
		},
	}
}

func TestRecipeModeUsesMerkleTransactionLane(t *testing.T) {
	f := newRecipeSolver(t)
	if f.solver.RequiresTxManager() || !f.solver.RequiresMerkleTxManager() || f.solver.Name() != "uniswapx-merkle" {
		t.Fatal("recipe solver still requires the native transaction manager or lost its name")
	}
	native := &Solver{}
	if !native.RequiresTxManager() || native.RequiresMerkleTxManager() || native.fillPath().laneBlockReason() != "" || native.Name() != Name {
		t.Fatal("adapter solver changed its transaction lane or name")
	}
	if got := f.solver.fillPath().laneBlockReason(); got != "recipe transaction manager is not ready" {
		t.Fatalf("unready recipe lane blocker = %q", got)
	}
}

func TestRecipeQuotesUseConnectorInventory(t *testing.T) {
	f := newRecipeSolver(t)
	if err := f.solver.refreshQuoteState(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	state := f.solver.quoteState.Load()
	if state == nil || len(state.inventory) != 1 || len(state.routes) != 1 {
		t.Fatalf("quote state = %+v", state)
	}
	item := state.inventory[0]
	if item.Adapter != connectortest.Venue || item.RecipeExecutor != fillertest.RecipeExecutor || item.DiscountPrecision != connector.Precision ||
		item.Discount.Int64() != 2_000_000 || item.MaxAssets.Int64() != 900 || item.Price.Sign() <= 0 {
		t.Fatalf("inventory = %+v", item)
	}
	// A read that fails for the whole inventory keeps the published state.
	f.chain.Header.Number = new(big.Int).Lsh(big.NewInt(1), 64)
	if err := f.solver.refreshQuoteState(t.Context(), nil); err == nil || f.solver.quoteState.Load() != state {
		t.Fatal("failed inventory read replaced the published quote state")
	}
	f.chain.Header.Number = big.NewInt(7)
	if !f.solver.inventoryPairInScope(connectortest.TokenIn, connectortest.TokenOut, state) {
		t.Fatal("connector pair is out of quote scope")
	}
	if reserved := f.solver.fillPath().reservations(state); len(reserved) != 0 {
		t.Fatalf("idle lane reserves %v", reserved)
	}
	// While the single lane owns a fill, its payment is reserved and quotes are declined.
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(123), AmountOut: big.NewInt(480)}}
	f.solver.strategy = strategy
	f.lane.LaneReady = func() bool { return true }
	f.sender.Result = fillertest.Confirmed()
	f.sender.DuringPoll = func() {
		if got := f.solver.quoteBlockReason(time.Now().Unix()); got != "recipe lane is filling order "+f.order.Hash.Hex() {
			t.Errorf("pending fill quote blocker = %q", got)
		}
	}
	fill, err := f.solver.fillPath().startFill(t.Context(), nil, f.order, time.Unix(1_000, 0), time.Now())
	if err != nil || fill == nil {
		t.Fatal(fill, err)
	}
	if settled := <-fill.result; settled.Outcome != txmanager.OutcomeConfirmed {
		t.Fatalf("settlement = %+v", settled)
	}
	fill.endFill(nil)
	// The settled payment stays reserved until inventory is observed past the fill block.
	capacity := liquidlane.NewCapacityID(1, f.chain.RecipeAccount, connectortest.TokenOut)
	if reserved := f.solver.fillPath().reservations(state)[capacity]; reserved == nil || reserved.Int64() != 490 {
		t.Fatalf("settled fill reserves %v, want its connector payment", reserved)
	}
	response, err := f.solver.evaluateQuote(t.Context(), quoteRequest{
		TokenInChainID: 1, TokenOutChainID: 1, RequestID: "request", QuoteID: "quote", Type: quoteTypeExactInput, Protocol: "v2",
		Swapper: common.HexToAddress("0x44").Hex(), NumOutputs: 1,
		TokenIn: connectortest.TokenIn.Hex(), TokenOut: connectortest.TokenOut.Hex(), Amount: "123",
	})
	if err != nil || response.AmountOut != "480" || len(strategy.inputs) != 1 {
		t.Fatalf("quote = %+v err=%v", response, err)
	}
	input := strategy.inputs[0]
	// Connector quotes follow the token policy like adapter quotes: this token may use several routes.
	if len(input.Inventory) != 1 || input.RequireSingleRoute || input.Reservations[capacity] == nil || input.Reservations[capacity].Int64() != 490 {
		t.Fatal("strategy input lost connector inventory or lane reservations, or forced a single route")
	}
	f.lane.LaneReady = func() bool { return false }
	if blocked, err := f.solver.evaluateQuote(t.Context(), quoteRequest{Type: quoteTypeExactInput, Amount: "123"}); err != nil || blocked.AmountOut != "0" {
		t.Fatal("quotes continued without a ready transaction lane")
	}

	f.discounts.Listed = nil
	if err := f.solver.refreshQuoteState(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if withdrawn := f.solver.quoteState.Load(); len(withdrawn.inventory) != 0 || len(withdrawn.routes) != 0 {
		t.Fatal("withdrawn discount stayed quotable")
	}
}

func TestRecipeFillReportsCanonicalOutcome(t *testing.T) {
	for _, reverted := range []bool{false, true} {
		f := newRecipeSolver(t)
		if err := f.solver.refreshQuoteState(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		f.sender.Result = fillertest.Confirmed()
		f.sender.Result.Reverted = reverted
		reg := prometheus.NewRegistry()
		var err error
		if f.solver.metrics, err = newUniswapXMetrics(reg, f.solver, ""); err != nil {
			t.Fatal(err)
		}
		fill, err := f.solver.fillPath().startFill(t.Context(), nil, f.order, time.Unix(1_000, 0), time.Now())
		if err != nil || fill == nil || fill.order != f.order {
			t.Fatal(fill, err)
		}
		result := <-fill.result
		fill.endFill(nil)
		// This lane has no native receipt observer: the confirmed fill itself counts the success.
		successes := float64(1)
		if reverted {
			successes = 0
		}
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_events_total",
			map[string]string{"solver": "uniswapx-merkle", "event": "fill", "outcome": liquidlane.FillOutcomeSuccess}, successes)
		if !reverted {
			metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total",
				map[string]string{"solver": "uniswapx-merkle", "event": "fill", "result": "mined_success"}, 1)
		}
		if result.Hash != f.sender.Result.Hash || len(result.Attempts) != 1 || result.NotAdmitted {
			t.Fatalf("result = %+v", result)
		}
		if reverted && (result.Outcome != txmanager.OutcomeReverted || result.Err == nil) ||
			!reverted && (result.Outcome != txmanager.OutcomeConfirmed || result.Err != nil) {
			t.Fatalf("reverted=%v: outcome = %s err=%v", reverted, result.Outcome, result.Err)
		}
		if f.sender.Submits != 1 || f.sender.Request.To != fillertest.Settlement || f.chain.OuterCalls != 1 || f.lane.Pending() != nil {
			t.Fatal("fill was not simulated, submitted once and settled")
		}
		// The executor call carries the unchanged signed order and a callback that funds the
		// connector, then calls the recipe.
		values, err := uniswapXExecutor.GetABI().Methods["execute"].Inputs.Unpack(f.sender.Request.Data[4:])
		if err != nil {
			t.Fatal(err)
		}
		type signedOrder struct{ Order, Sig []byte }
		signed := *abi.ConvertType(values[0], new(signedOrder)).(*signedOrder)
		calls := *abi.ConvertType(values[1], new([]execution.Call)).(*[]execution.Call)
		wantFunding := erc4626.NewIERC4626().PackTransfer(connectortest.Venue, f.order.AmountIn)
		if !bytes.Equal(signed.Order, f.order.Encoded) || !bytes.Equal(signed.Sig, f.order.Signature) || len(calls) != 2 ||
			calls[0].Target != connectortest.TokenIn || !bytes.Equal(calls[0].Data, wantFunding) || calls[1].Target != f.chain.RecipeExecutor {
			t.Fatal("executor call changed the signed order or its recipe callback")
		}
		// Inventory observed before the fill block still counts the confirmed payment.
		if reserved := len(f.solver.fillPath().reservations(f.solver.quoteState.Load())) != 0; reserved == reverted {
			t.Fatal("confirmed payment must stay reserved until inventory reflects it; a revert must release it")
		}
	}
}

func TestRecipeFillClassifiesFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*recipeSolverFixture)
		is     error
	}{
		{"payment below output", func(f *recipeSolverFixture) { f.order.AmountOut = big.NewInt(491) }, errOrderNotFillable},
		{"another executor", func(f *recipeSolverFixture) { f.order.Executor = common.HexToAddress("0x11") }, errOrderNotFillable},
		{"settlement reverts", func(f *recipeSolverFixture) { f.chain.OuterErr = errors.New("execution reverted") }, errFillPreflight},
		{"refused before signing", func(f *recipeSolverFixture) {
			f.sender.Unsigned, f.sender.SubmitErr = true, errors.New("gas estimation reverted")
		}, errFillPreflight},
		{"discount service down", func(f *recipeSolverFixture) { f.discounts.ListErr = errors.New("unavailable") }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecipeSolver(t)
			tc.mutate(&f)
			fill, err := f.solver.fillPath().startFill(t.Context(), nil, f.order, time.Unix(1_000, 0), time.Now())
			if err == nil || fill != nil || f.lane.Pending() != nil {
				t.Fatalf("fill=%v err=%v", fill, err)
			}
			if tc.is != nil && !errors.Is(err, tc.is) || tc.is == nil && (errors.Is(err, errOrderNotFillable) || errors.Is(err, errFillPreflight)) {
				t.Fatalf("error = %v, want class %v", err, tc.is)
			}
		})
	}
}

func TestRecipeFillDefersWhileLaneIsBusy(t *testing.T) {
	f := newRecipeSolver(t)
	first, err := f.solver.fillPath().startFill(t.Context(), nil, f.order, time.Unix(1_000, 0), time.Now())
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	second := *f.order
	second.Hash = common.HexToHash("0xdef")
	if fill, err := f.solver.fillPath().startFill(t.Context(), nil, &second, time.Unix(1_000, 0), time.Now()); !errors.Is(err, errOrderNotFillable) || fill != nil {
		t.Fatal("second order entered a busy lane", err)
	}
	f.lane.LaneReady = func() bool { return true }
	if f.solver.fillPath().laneBlockReason() == "" || !f.solver.quoteBlocked(time.Now().Unix()) {
		t.Fatal("busy lane admits new fills or keeps quoting")
	}
}

// The real strategies price connector inventory at its 1e8 discount scale and never quote more
// than the connector pays at fill time (980000 for one whole 18-decimal token at a 2% discount).
func TestRecipeQuotePricesConnectorDiscount(t *testing.T) {
	for _, name := range []string{strategytypes.DefaultName, strategytypes.SingleName} {
		t.Run(name, func(t *testing.T) {
			f := newRecipeSolver(t)
			f.chain.AmountOut, f.chain.Remaining = big.NewInt(1e18), big.NewInt(900_000_000)
			f.discounts.Listed[0].MaxAssets = "900000000"
			var raw RawEngineConfig
			node := fillertest.ExampleConfig(t, "../../../config/uniswapx-merkle.example.yaml")
			if err := node.Decode(&raw); err != nil {
				t.Fatal(err)
			}
			cfg, err := ParseRecipeConfig(raw)
			if err != nil || !cfg.usesDiscounts() || cfg.Gas != nil || len(cfg.Adapters) != 0 {
				t.Fatalf("example config = %+v err=%v", cfg, err)
			}
			cfg.Strategy.Name = name
			if f.solver.strategy, err = newStrategy(cfg.Strategy); err != nil {
				t.Fatal(err)
			}
			f.lane.LaneReady = func() bool { return true }
			if err := f.solver.refreshQuoteState(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			response, err := f.solver.evaluateQuote(t.Context(), quoteRequest{
				TokenInChainID: 1, TokenOutChainID: 1, RequestID: "request", QuoteID: "quote", Type: quoteTypeExactInput, Protocol: "v2",
				Swapper: common.HexToAddress("0x44").Hex(), NumOutputs: 1,
				TokenIn: connectortest.TokenIn.Hex(), TokenOut: connectortest.TokenOut.Hex(), Amount: "1000000000000000000",
			})
			if err != nil {
				t.Fatal(err)
			}
			quoted, ok := new(big.Int).SetString(response.AmountOut, 10)
			if !ok || quoted.Int64() <= 970_000 || quoted.Int64() > 980_000 {
				t.Fatalf("quote = %+v, want just under the connector payment of 980000", response)
			}
		})
	}
}

// A settled recipe fill invalidates quote state; without a native reservation to release, the
// completion itself must request the refresh or quotes stay down until the next ticker.
func TestRecipeFillCompletionRefreshesQuotes(t *testing.T) {
	f := newRecipeSolver(t)
	f.solver.refreshCh = make(chan struct{}, 1)
	f.solver.filled, f.solver.retryAt = map[common.Hash]time.Time{}, map[common.Hash]time.Time{}
	f.solver.inFlight, f.solver.attempts, f.solver.ownedFillAttempts = map[common.Hash]bool{}, map[common.Hash]int{}, map[common.Hash]bool{}
	if err := f.solver.refreshQuoteState(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	f.sender.Result = fillertest.Confirmed()
	fill, err := f.solver.fillPath().startFill(t.Context(), nil, f.order, time.Unix(1_000, 0), time.Now())
	if err != nil || fill == nil {
		t.Fatal(fill, err)
	}
	f.solver.completePendingFill(t.Context(), fill, <-fill.result)
	select {
	case <-f.solver.refreshCh:
	default:
		t.Fatal("completed recipe fill left quotes invalidated without requesting a refresh")
	}
	if _, filled := f.solver.filled[f.order.Hash]; !filled {
		t.Fatal("confirmed recipe fill did not complete the order")
	}
}

// A lane whose sender dropped the transaction accepts no fills and stops quoting.
func TestStalledRecipeLaneBlocksQuotesAndFills(t *testing.T) {
	f := newRecipeSolver(t)
	f.lane.LaneReady = func() bool { return true }
	f.sender.Unsigned = true // The sender accepts the fill, then drops it without a receipt.
	fill, err := f.solver.fillPath().startFill(t.Context(), nil, f.order, time.Unix(1_000, 0), time.Now())
	if err != nil || fill == nil {
		t.Fatal(fill, err)
	}
	result := <-fill.result
	fill.endFill(nil)
	if !result.Outcome.NonceUncertain() || !f.lane.Stalled() {
		t.Fatalf("result = %+v", result)
	}
	if got := f.solver.quoteBlockReason(time.Now().Unix()); got != "recipe lane stalled while filling order "+f.order.Hash.Hex() {
		t.Fatalf("stalled fill quote blocker = %q", got)
	}
}

// Both UniswapX solvers can register their metrics in one process: the protocol gauges carry
// the solver label.
func TestBothSolversRegisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	for _, name := range []string{Name, "uniswapx-merkle"} {
		if _, err := newUniswapXMetrics(reg, &Solver{name: name}, ""); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
