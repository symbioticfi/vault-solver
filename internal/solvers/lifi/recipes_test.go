package lifi

import (
	"bytes"
	"context"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	lifiexecutor "github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type recipeSolverFixture struct {
	solver    *Solver
	lane      *filler.Swaps
	chain     *connectortest.Chain
	sender    *fillertest.Sender
	discounts *fillertest.Discounts
	order     *submittedOrder
	orderID   common.Hash
}

// The fixture oracle prices the 1e6 input at 1,020,408; the 2% discount pays 999,999 for the
// order's 990,000 output.
func newRecipeSolver(t *testing.T) recipeSolverFixture {
	t.Helper()
	workflow, chain := fillertest.Workflow(t, [4]byte(lifiExecutor.GetABI().Methods["finaliseWithCurrentTimestamp"].ID))
	chain.AmountOut, chain.Remaining = big.NewInt(1_020_408), big.NewInt(2_000_000)
	sender, listed := &fillertest.Sender{}, fillertest.NewDiscounts()
	listed.Listed[0].Discount, listed.Listed[0].CollateralDecimals = "2000000", 6
	listed.Listed[0].MaxAssets, listed.Listed[0].Deadline = "2000000", 4102444800
	lane := &filler.Swaps{
		Engine: &filler.Engine{
			Name: "lifi-merkle", Workflow: workflow, Spends: connector.NewSpendLedger(), Sender: sender,
			Routes:    connector.NewRoutes(workflow, []common.Address{fillertest.RecipeExecutor}),
			LaneReady: func() bool { return true },
		},
		Discounts: listed,
	}
	if err := lane.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg := testLifiConfig()
	cfg.Executor, cfg.Adapters, cfg.SolverMode, cfg.Gas = fillertest.Settlement, nil, solverModeInternal, nil
	orderID := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	solver := newProcessTestSolver(cfg, common.HexToAddress("0x55"), nil, nil,
		connectortest.TokenIn, connectortest.TokenOut, connectortest.Venue, lifiOrderStatusDeposited)
	solver.txm, solver.maxFeePerGas = nil, nil
	solver.name, solver.discounts, solver.txLaneState = "lifi-merkle", listed, recipeLane{lane}
	solver.quoteRefresh = make(chan struct{}, 1)
	lane.Decimals = func(_ context.Context, token common.Address) (int, error) {
		if token == connectortest.TokenOut {
			return 6, nil
		}
		return 18, nil
	}
	solver.settlement = &recipeSettlement{s: solver, lane: lane}
	order := testSubmittedOrder(t, cfg, connectortest.TokenIn, connectortest.TokenOut)
	order.Order.Expires, order.Order.FillDeadline = 4102444800, 4102444800
	return recipeSolverFixture{solver: solver, lane: lane, chain: chain, sender: sender, discounts: listed, order: order, orderID: orderID}
}

func (f recipeSolverFixture) process(t *testing.T, pending *pendingFillState) orderProcessingResult {
	t.Helper()
	return f.solver.processOrderWithPending(solverContext(t, f.solver), nil, f.order, pending)
}

func TestRecipeModeUsesMerkleTransactionLane(t *testing.T) {
	f := newRecipeSolver(t)
	if f.solver.RequiresTxManager() || !f.solver.RequiresMerkleTxManager() || !f.solver.transactionLaneReady() || f.solver.Name() != "lifi-merkle" {
		t.Fatal("recipe solver does not run on the Merkle transaction lane or lost its name")
	}
	if changes, stop := f.solver.subscribeTransactionLaneState(); changes != nil {
		t.Fatal("recipe lane advertises a change feed it does not have")
	} else {
		stop()
	}
	f.lane.LaneReady = func() bool { return false }
	if f.solver.transactionLaneReady() {
		t.Fatal("quotes stay enabled without a ready transaction lane")
	}
	native := &Solver{}
	if !native.RequiresTxManager() || native.RequiresMerkleTxManager() || native.Name() != Name {
		t.Fatal("adapter solver changed its transaction lane or name")
	}
}

func TestRecipeOrderSettlesThroughMerkleLane(t *testing.T) {
	for _, reverted := range []bool{false, true} {
		f := newRecipeSolver(t)
		f.sender.Result = fillertest.Confirmed()
		f.sender.Result.Reverted = reverted
		reg := prometheus.NewRegistry()
		var err error
		if f.solver.metrics, err = newLIFIMetrics("lifi-merkle", reg, nil, ""); err != nil {
			t.Fatal(err)
		}
		result := f.process(t, &pendingFillState{})
		if result.fill == nil || result.outcome != orderProcessingSubmitted || result.err != nil {
			t.Fatalf("result = %+v", result)
		}
		if result.fill.order != f.order || result.fill.orderID != f.orderID || result.fill.reservationKey != f.orderID.Hex() || result.fill.retryDeadline.IsZero() {
			t.Fatalf("pending fill = %+v", result.fill)
		}
		settled := <-result.fill.result
		if settled.Hash != f.sender.Result.Hash || reverted && settled.Outcome != txmanager.OutcomeReverted ||
			!reverted && (settled.Outcome != txmanager.OutcomeConfirmed || settled.Err != nil) {
			t.Fatalf("reverted=%v: settled = %+v", reverted, settled)
		}
		if f.sender.Submits != 1 || f.sender.Request.To != fillertest.Settlement || f.chain.OuterCalls != 1 || f.lane.Pending() != nil {
			t.Fatal("order was not simulated, submitted once and settled")
		}
		// This lane has no native receipt observer: the confirmed fill itself counts the success.
		successes := float64(1)
		if reverted {
			successes = 0
		}
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_events_total",
			map[string]string{"solver": "lifi-merkle", "event": "fill", "outcome": liquidlane.FillOutcomeSuccess}, successes)
		// The executor call finalises the unchanged order with a callback that funds the connector,
		// then calls the recipe.
		values, err := lifiExecutor.GetABI().Methods["finaliseWithCurrentTimestamp"].Inputs.Unpack(f.sender.Request.Data[4:])
		if err != nil {
			t.Fatal(err)
		}
		finalised := *abi.ConvertType(values[0], new(lifiexecutor.IInputSettlerStandardOrder)).(*lifiexecutor.IInputSettlerStandardOrder)
		calls := *abi.ConvertType(values[1], new([]execution.Call)).(*[]execution.Call)
		wantFunding := erc4626.NewIERC4626().PackTransfer(connectortest.Venue, f.order.AmountIn)
		if finalised.User != f.order.Order.User || finalised.Nonce.Cmp(f.order.Order.Nonce) != 0 || finalised.FillDeadline != f.order.Order.FillDeadline ||
			len(calls) != 2 || calls[0].Target != connectortest.TokenIn || !bytes.Equal(calls[0].Data, wantFunding) || calls[1].Target != f.chain.RecipeExecutor {
			t.Fatal("executor call changed the order or its recipe callback")
		}
		select {
		case <-f.solver.quoteRefresh:
		case <-time.After(time.Second):
			t.Fatal("settled fill did not refresh quotes")
		}
	}
}

func TestRecipeOrderWaitsForBusyLane(t *testing.T) {
	f := newRecipeSolver(t)
	first := f.process(t, &pendingFillState{})
	if first.fill == nil {
		t.Fatalf("first = %+v", first)
	}
	capacity := liquidlane.NewCapacityID(1, f.chain.RecipeAccount, connectortest.TokenOut)
	second := f.process(t, &pendingFillState{})
	if second.fill != nil || second.outcome != orderProcessingCapacityDeferred || !second.blockedOn[capacity] || f.sender.Submits != 1 {
		t.Fatalf("busy lane result = %+v", second)
	}
	if reserved := f.solver.fillPath().reservations(nil)[capacity]; reserved == nil || reserved.Int64() != 999_999 {
		t.Fatalf("pending fill reserves %v, want its connector payment", reserved)
	}
}

func TestRecipeOrderClassifiesFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*recipeSolverFixture)
		outcome   orderProcessingOutcome
		retryable bool
	}{
		{"payment below output", func(f *recipeSolverFixture) { f.order.OutputAmount = big.NewInt(1_000_000) }, orderProcessingStrategyDeclined, false},
		{"settlement reverts", func(f *recipeSolverFixture) { f.chain.OuterErr = errors.New("execution reverted") }, orderProcessingRetryableError, true},
		{"discount service down", func(f *recipeSolverFixture) { f.discounts.ListErr = errors.New("unavailable") }, orderProcessingRetryableError, true},
		{"signed terms unavailable", func(f *recipeSolverFixture) { f.discounts.ResolveErr = errors.New("backend timeout") }, orderProcessingRetryableError, true},
		{"refused before signing", func(f *recipeSolverFixture) {
			f.sender.Unsigned, f.sender.SubmitErr = true, errors.New("gas estimation reverted")
		}, orderProcessingRetryableError, true},
		{"expires too soon", func(f *recipeSolverFixture) { f.lane.MinValidity = 200 * 365 * 24 * time.Hour }, orderProcessingStrategyDeclined, false},
		{"signed discount expires before the order", func(f *recipeSolverFixture) {
			f.order.Order.Expires, f.order.Order.FillDeadline, f.lane.MinValidity = 4102444900, 4102444900, 200*365*24*time.Hour
		}, orderProcessingRetryableError, true},
		{"no deadline", func(f *recipeSolverFixture) { f.order.Order.Expires, f.order.Order.FillDeadline = 0, 0 }, orderProcessingNotActionable, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecipeSolver(t)
			tc.mutate(&f)
			result := f.process(t, &pendingFillState{})
			// A failed dependency is retried on the worker's own timer, not left to the next feed recovery.
			if result.fill != nil || result.outcome != tc.outcome || result.retryable != tc.retryable || result.retrySoon != tc.retryable ||
				(result.err != nil) != tc.retryable || f.lane.Pending() != nil {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestRecipeQuotesUseConnectorInventory(t *testing.T) {
	f := newRecipeSolver(t)
	source, err := f.solver.fillPath().quoteSource(t.Context(), nil, time.Now())
	inventory := source.inventory
	// Connector quotes follow the token policy like adapter quotes: this token may use several routes.
	if err != nil || source.degraded || source.discountErr != nil || len(inventory) != 1 || source.singleRouteTokens[connectortest.TokenIn] {
		t.Fatalf("source = %+v err=%v", source, err)
	}
	item := inventory[0]
	if item.Adapter != connectortest.Venue || item.RecipeExecutor != fillertest.RecipeExecutor || item.DiscountPrecision != connector.Precision ||
		item.Discount.Int64() != 2_000_000 || item.MaxAssets.Int64() != 2_000_000 {
		t.Fatalf("inventory = %+v", item)
	}
	// A read that fails for the whole inventory is an error, so the published quotes stay.
	f.chain.Header.Number = new(big.Int).Lsh(big.NewInt(1), 64)
	if _, err := f.solver.fillPath().quoteSource(t.Context(), nil, time.Now()); err == nil {
		t.Fatal("failed inventory read published an empty inventory")
	}
	f.chain.Header.Number = big.NewInt(7)
	if reserved := f.solver.fillPath().reservations(inventory); len(reserved) != 0 {
		t.Fatalf("idle lane reserves %v", reserved)
	}
	f.discounts.Listed[0].CollateralDecimals = 8
	if contradicted, err := f.solver.fillPath().quoteSource(t.Context(), nil, time.Now()); err != nil || !contradicted.degraded || len(contradicted.inventory) != 0 {
		t.Fatal("contradicted listing was quoted", err)
	}
	f.discounts.ListErr = errors.New("unavailable")
	if unavailable, err := f.solver.fillPath().quoteSource(t.Context(), nil, time.Now()); err != nil || unavailable.discountErr == nil || !unavailable.degraded {
		t.Fatal("unavailable discount service was not reported", err)
	}
}

// The real strategy prices connector inventory at its 1e8 discount scale and never quotes a
// rate above what the connector pays at fill time (0.98 at a 2% discount).
func TestRecipeQuotePricesConnectorDiscount(t *testing.T) {
	f := newRecipeSolver(t)
	f.chain.AmountOut, f.chain.Remaining = big.NewInt(1e18), big.NewInt(900_000_000)
	f.discounts.Listed[0].MaxAssets = "900000000"
	var raw RawEngineConfig
	node := fillertest.ExampleConfig(t, "../../../config/lifi-merkle.example.yaml")
	if err := node.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseRecipeConfig(raw)
	if err != nil || !cfg.usesDiscounts() || cfg.Gas != nil || len(cfg.Adapters) != 0 {
		t.Fatalf("example config = %+v err=%v", cfg, err)
	}
	strategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.solver.fillPath().quoteSource(t.Context(), nil, time.Now())
	inventory := source.inventory
	if err != nil || source.degraded || len(inventory) != 1 {
		t.Fatalf("source = %+v err=%v", source, err)
	}
	now := time.Now()
	out, err := strategy.DecideQuotes(t.Context(), types.QuoteInput{
		Solver: f.solver.cfg.Executor, Inventory: inventory, Reservations: f.solver.fillPath().reservations(inventory),
		SingleRouteTokens: source.singleRouteTokens, MaxFeePerGas: new(big.Int),
		ChainTime: now, ServerTime: now, QuoteExpiresAt: now.Add(36 * time.Second),
	})
	if err != nil || len(out.Quotes) != 1 || len(out.Quotes[0].Ranges) == 0 {
		t.Fatalf("quotes = %+v err=%v", out.Quotes, err)
	}
	quote := out.Quotes[0]
	if quote.FromAsset != connectortest.TokenIn || quote.ToAsset != connectortest.TokenOut || quote.FromDecimals != 18 || quote.ToDecimals != 6 {
		t.Fatalf("quote = %+v", quote)
	}
	// Dust ranges round further down; no range may exceed the connector rate and the largest
	// range sits just under it.
	best := new(big.Rat)
	for _, quoted := range quote.Ranges {
		rate, ok := new(big.Rat).SetString(quoted.Quote)
		if !ok || rate.Sign() <= 0 || rate.Cmp(big.NewRat(98, 100)) > 0 {
			t.Fatalf("range %s..%s quotes %s above the connector rate 0.98", quoted.MinAmount, quoted.MaxAmount, quoted.Quote)
		}
		if rate.Cmp(best) > 0 {
			best = rate
		}
	}
	if best.Cmp(big.NewRat(97, 100)) <= 0 {
		t.Fatalf("best quoted rate %s, want just under the connector rate 0.98", best.FloatString(6))
	}
}

// A lane whose sender dropped the transaction stops quoting.
func TestStalledRecipeLaneStopsQuotes(t *testing.T) {
	f := newRecipeSolver(t)
	f.sender.Unsigned = true // The sender accepts the fill, then drops it without a receipt.
	result := f.process(t, &pendingFillState{})
	if result.fill == nil {
		t.Fatalf("result = %+v", result)
	}
	settled := <-result.fill.result
	if !settled.Outcome.NonceUncertain() || !f.lane.Stalled() || f.solver.transactionLaneReady() {
		t.Fatalf("stalled lane keeps quoting: %+v", settled)
	}
}

// recoveringDiscounts fails signed-terms requests until it is switched on.
type recoveringDiscounts struct {
	*fillertest.Discounts

	up atomic.Bool
}

func (d *recoveringDiscounts) Resolve(ctx context.Context, id string) (*discounts.Resolved, error) {
	if !d.up.Load() {
		return nil, errors.New("backend timeout")
	}
	return d.Discounts.Resolve(ctx, id)
}

// Outside feed recovery a retryable result alone waits for the next reconnect. A failed fill
// dependency must instead be retried by the worker on its own timer, so the order settles as
// soon as the dependency is back.
func TestRecipeWorkerRetriesOrderAfterDependencyRecovers(t *testing.T) {
	f := newRecipeSolver(t)
	f.solver.wallNow = time.Now
	provider := &recoveringDiscounts{Discounts: f.discounts}
	f.lane.Discounts = provider
	submitted := make(chan struct{})
	f.sender.Before = func(transaction.ExecutionRequest) { close(submitted) }
	ctx, stop := context.WithCancel(solverContext(t, f.solver))
	orders := make(chan *submittedOrder, 1)
	done := make(chan error, 1)
	go func() { done <- f.solver.runOrderWorker(ctx, nil, orders, nil, nil) }()
	orders <- f.order
	select {
	case <-submitted:
		t.Fatal("fill submitted without signed terms")
	case <-time.After(700 * time.Millisecond):
	}
	provider.up.Store(true)
	select {
	case <-submitted:
	case <-time.After(10 * time.Second):
		t.Fatal("order was not retried after its dependency recovered")
	}
	stop()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop")
	}
}

// A recipe fill that reverts on chain leaves its order open. The worker plans it again from
// fresh state, and gives up after maxRevertRetries so a fill that keeps reverting cannot spend
// gas until the order deadline.
func TestRecipeWorkerRetriesOrderAfterRevert(t *testing.T) {
	for name, recovers := range map[string]bool{"next attempt confirms": true, "keeps reverting": false} {
		t.Run(name, func(t *testing.T) {
			f := newRecipeSolver(t)
			f.solver.wallNow = time.Now
			f.sender.Result = fillertest.Confirmed()
			f.sender.Result.Reverted = true
			submits := make(chan int, 8)
			count := 0
			f.sender.Before = func(transaction.ExecutionRequest) {
				count++
				if recovers && count == 2 {
					f.sender.Result = fillertest.Confirmed()
				}
				submits <- count
			}
			ctx, stop := context.WithCancel(solverContext(t, f.solver))
			orders := make(chan *submittedOrder, 1)
			done := make(chan error, 1)
			go func() { done <- f.solver.runOrderWorker(ctx, nil, orders, nil, nil) }()
			orders <- f.order
			want := 2
			if !recovers {
				want = 1 + maxRevertRetries
			}
			for attempt := 1; attempt <= want; attempt++ {
				select {
				case got := <-submits:
					if got != attempt {
						t.Fatalf("submit %d, want %d", got, attempt)
					}
				case <-time.After(10 * time.Second):
					t.Fatalf("order was not planned again after its fill reverted: %d of %d submits", attempt-1, want)
				}
			}
			select {
			case extra := <-submits:
				t.Fatalf("unexpected fill %d after the order settled or the retries were spent", extra)
			case <-time.After(3 * time.Second):
			}
			stop()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("worker did not stop")
			}
		})
	}
}

// Both LI.FI solvers can register their metrics in one process: the protocol gauges carry the
// solver label.
func TestBothSolversRegisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	for _, name := range []string{Name, "lifi-merkle"} {
		if _, err := newLIFIMetrics(name, reg, nil, ""); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
