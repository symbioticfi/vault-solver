package uniswapx

import (
	"context"
	"math/big"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type executionTestReader struct {
	chainReader

	resolved         []liquidlane.Route
	adapters         []common.Address
	fillRoutes       []liquidlane.Route
	fillAmounts      []*big.Int
	fillQuoteRoutes  [][]liquidlane.Route
	fillQuoteAmounts []*big.Int
	snapshot         fillSnapshot
	quoteSnapshot    snapshot
	fillSnapshotFn   func([]liquidlane.Route, *big.Int) fillSnapshot
	now              time.Time
	latestBlockReads int
}

type failingListener struct{ err error }

func (l failingListener) Accept() (net.Conn, error) { return nil, l.err }
func (failingListener) Close() error                { return nil }
func (failingListener) Addr() net.Addr              { return &net.TCPAddr{} }

func (r *executionTestReader) ResolveRoutes(
	_ context.Context,
	adapters []common.Address,
) ([]liquidlane.Route, error) {
	r.adapters = append([]common.Address(nil), adapters...)
	return append([]liquidlane.Route(nil), r.resolved...), nil
}

func (r *executionTestReader) ValidateGasTokens([]liquidlane.Route) error { return nil }

func (r *executionTestReader) latestBlockTime(context.Context) (time.Time, error) {
	r.latestBlockReads++
	return r.now, nil
}

func TestStartFillEncodesResolvedDiscountRoute(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	route := testDiscountRoute()
	configuredRoute := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		route.TokenIn,
		route.TokenOut,
		route.TokenInDecimals,
		route.TokenOutDecimals,
	)
	strategy := &executionTestStrategy{plan: &strategytypes.FillPlan{Routes: []strategytypes.FillRoute{{
		RouteID: route.ID, CapacityID: route.CapacityID, Adapter: route.Adapter,
		AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(100), MinAmountOut: big.NewInt(90),
		ReservedAmountOut: big.NewInt(100), DiscountID: new(common.HexToHash(testDiscountID)),
	}}}}
	policy, _ := tokenpolicy.New(tokenpolicy.All, nil)
	termsDeadline := now.Add(50 * time.Second).Unix()
	protocolDeadline := now.Add(45 * time.Second).Unix()
	provider := &fakeDiscountProvider{
		list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
			testDiscountOffer(route, now.Add(time.Minute), "100", "1000000000000000000"),
		}},
		resolved: &liquiddiscounts.Resolved{
			DiscountID: testDiscountID,
			Discount: liquiddiscounts.Terms{
				Adapter: route.Adapter.Hex(), TokenToRedeem: route.TokenIn.Hex(), Discount: "0",
				Signer:   common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
				Protocol: common.HexToAddress("0x6666666666666666666666666666666666666666").Hex(),
				Nonce:    "0x1", Deadline: termsDeadline,
			},
			SignerSignature: "0x01", ProtocolDeadline: protocolDeadline, ProtocolSignature: "0x02",
		},
	}
	physicalQuote := liquidlane.FillQuote{
		Inventory: testInventoryWithMinDiscount(
			route, big.NewInt(100), big.NewInt(1_000_000_000_000_000_000), new(big.Int),
		),
		AmountIn: big.NewInt(100), GrossAmountOut: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		MinDiscount: new(big.Int),
	}
	var packed uxexecutor.ILiquidLaneUniswapXExecutorFillCall
	reader := &executionTestReader{
		resolved: []liquidlane.Route{route},
		snapshot: fillSnapshot{
			Direct:   []liquidlane.FillQuote{physicalQuote},
			Physical: []liquidlane.FillQuote{physicalQuote},
		},
	}
	txm := &executionTestTxManager{result: make(chan txmanager.Result, 1)}
	solver := &Solver{
		cfg: &Config{
			Executor: common.HexToAddress("0x7777777777777777777777777777777777777777"), TokenPolicy: policy,
			Adapters:    []common.Address{configuredRoute.Adapter},
			SolverMode:  solverModeInternal,
			Discounts:   &DiscountConfig{HTTPTimeout: time.Second, MinimumValidity: 15 * time.Second},
			OrderServer: OrderServerConfig{PollInterval: time.Second},
		},
		solverAddress: common.HexToAddress("0x8888888888888888888888888888888888888888"),
		chain: contractCallerFunc(func(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			parsed, err := uxexecutor.LiquidLaneUniswapXExecutorMetaData.ParseABI()
			if err != nil {
				return nil, err
			}
			values, err := parsed.Methods["execute"].Inputs.Unpack(call.Data[4:])
			if err != nil {
				return nil, err
			}
			packed = *abi.ConvertType(values[1], new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall)).(*uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
			return nil, nil
		}),
		reader:   reader,
		strategy: strategy, txm: txm,
		discounts: provider, log: logr.Discard(),
		orderState: make(map[common.Hash]trackedOrder),
	}
	order := &resolvedOrder{
		Encoded: []byte{1}, Signature: []byte{2}, Hash: common.HexToHash("0x1"), Source: orderSourcePublicV2,
		Executor: solver.cfg.Executor, TokenIn: route.TokenIn, TokenOut: route.TokenOut,
		AmountIn: big.NewInt(100), AmountOut: big.NewInt(90), Deadline: uint32(now.Add(time.Minute).Unix()),
	}
	if _, err := solver.startFill(t.Context(), []liquidlane.Route{configuredRoute}, order, now, now); err != nil {
		t.Fatalf("startFill: %v", err)
	}
	if len(reader.adapters) != 1 || reader.adapters[0] != route.Adapter ||
		len(reader.fillRoutes) != 2 {
		t.Fatalf("dynamic fill routes: adapters=%+v routes=%+v", reader.adapters, reader.fillRoutes)
	}
	if len(strategy.input.Quotes) != 1 || strategy.input.Quotes[0].DiscountID == nil ||
		strategy.input.Quotes[0].Adapter != route.Adapter {
		t.Fatalf("fill candidates leaked dynamic direct route: %+v", strategy.input.Quotes)
	}
	if len(packed.Routes) != 0 || len(packed.DiscountRoutes) != 1 {
		t.Fatalf("packed fill call = %+v", packed)
	}
	if len(txm.reqs) != 1 || !txm.reqs[0].CancelAt.Equal(time.Unix(protocolDeadline, 0)) {
		t.Fatalf("discount fill cancelAt = %v, want %s", txm.reqs, time.Unix(protocolDeadline, 0))
	}
	discountRoute := packed.DiscountRoutes[0]
	if discountRoute.Adapter != route.Adapter ||
		discountRoute.AmountIn.Cmp(big.NewInt(100)) != 0 ||
		discountRoute.DiscountSwap.Discount.TokenToRedeem != route.TokenIn {
		t.Fatalf("packed discount route = %+v", discountRoute)
	}
}

func TestStartFillRepricesPartialDiscountLeg(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	discountRoute := testDiscountRoute()
	directRoute := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		discountRoute.TokenIn,
		discountRoute.TokenOut,
		discountRoute.TokenInDecimals,
		discountRoute.TokenOutDecimals,
	)
	strategy := &executionTestStrategy{plan: &strategytypes.FillPlan{Routes: []strategytypes.FillRoute{
		{
			RouteID: directRoute.ID, CapacityID: directRoute.CapacityID, Adapter: directRoute.Adapter,
			AmountIn: big.NewInt(60), ExpectedAmountOut: big.NewInt(60), MinAmountOut: big.NewInt(50),
			ReservedAmountOut: big.NewInt(60),
		},
		{
			RouteID: discountRoute.ID, CapacityID: discountRoute.CapacityID, Adapter: discountRoute.Adapter,
			AmountIn: big.NewInt(40), ExpectedAmountOut: big.NewInt(40), MinAmountOut: big.NewInt(40),
			ReservedAmountOut: big.NewInt(40), DiscountID: new(common.HexToHash(testDiscountID)),
		},
	}}}
	fullDirect := liquidlane.FillQuote{
		Inventory: liquidlane.DirectInventory(
			directRoute, big.NewInt(100), big.NewInt(1_000_000_000_000_000_000),
		),
		AmountIn: big.NewInt(100), GrossAmountOut: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		MinDiscount: new(big.Int),
	}
	fullDiscount := liquidlane.FillQuote{
		Inventory: testInventoryWithMinDiscount(
			discountRoute, big.NewInt(100), big.NewInt(1_000_000_000_000_000_000), new(big.Int),
		),
		AmountIn: big.NewInt(100), GrossAmountOut: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		MinDiscount: new(big.Int),
	}
	partialDiscount := fullDiscount
	partialDiscount.AmountIn = big.NewInt(40)
	partialDiscount.GrossAmountOut = big.NewInt(41)
	partialDiscount.MaxAmountOut = big.NewInt(41)

	reader := &executionTestReader{resolved: []liquidlane.Route{discountRoute}}
	reader.fillSnapshotFn = func(routes []liquidlane.Route, amountIn *big.Int) fillSnapshot {
		if amountIn.Cmp(big.NewInt(100)) == 0 {
			return fillSnapshot{
				Direct: []liquidlane.FillQuote{fullDirect, fullDiscount},
				Physical: []liquidlane.FillQuote{
					fullDirect,
					fullDiscount,
				},
			}
		}
		if len(routes) == 1 && routes[0].ID == discountRoute.ID && amountIn.Cmp(big.NewInt(40)) == 0 {
			return fillSnapshot{Physical: []liquidlane.FillQuote{partialDiscount}}
		}
		t.Fatalf("unexpected fill snapshot request: routes=%+v amountIn=%s", routes, amountIn)
		return fillSnapshot{}
	}
	policy, _ := tokenpolicy.New(tokenpolicy.All, nil)
	deadline := now.Add(time.Minute).Unix()
	provider := &fakeDiscountProvider{
		list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
			testDiscountOffer(discountRoute, now.Add(time.Minute), "100", "1000000000000000000"),
		}},
		resolved: &liquiddiscounts.Resolved{
			DiscountID: testDiscountID,
			Discount: liquiddiscounts.Terms{
				Adapter: discountRoute.Adapter.Hex(), TokenToRedeem: discountRoute.TokenIn.Hex(), Discount: "0",
				Signer:   common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
				Protocol: common.HexToAddress("0x6666666666666666666666666666666666666666").Hex(),
				Nonce:    "0x1", Deadline: deadline,
			},
			SignerSignature: "0x01", ProtocolDeadline: deadline, ProtocolSignature: "0x02",
		},
	}
	var packed uxexecutor.ILiquidLaneUniswapXExecutorFillCall
	solver := &Solver{
		cfg: &Config{
			Executor: common.HexToAddress("0x7777777777777777777777777777777777777777"), TokenPolicy: policy,
			Adapters:    []common.Address{directRoute.Adapter},
			SolverMode:  solverModeInternal,
			Discounts:   &DiscountConfig{HTTPTimeout: time.Second, MinimumValidity: 15 * time.Second},
			OrderServer: OrderServerConfig{PollInterval: time.Second},
		},
		solverAddress: common.HexToAddress("0x8888888888888888888888888888888888888888"),
		chain: contractCallerFunc(func(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			parsed, err := uxexecutor.LiquidLaneUniswapXExecutorMetaData.ParseABI()
			if err != nil {
				return nil, err
			}
			values, err := parsed.Methods["execute"].Inputs.Unpack(call.Data[4:])
			if err != nil {
				return nil, err
			}
			packed = *abi.ConvertType(
				values[1], new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall),
			).(*uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
			return nil, nil
		}),
		reader: reader, strategy: strategy,
		txm:       &executionTestTxManager{result: make(chan txmanager.Result, 1)},
		discounts: provider, log: logr.Discard(),
		orderState: make(map[common.Hash]trackedOrder),
	}
	order := &resolvedOrder{
		Encoded: []byte{1}, Signature: []byte{2}, Hash: common.HexToHash("0x1"), Source: orderSourcePublicV2,
		Executor: solver.cfg.Executor, TokenIn: discountRoute.TokenIn, TokenOut: discountRoute.TokenOut,
		AmountIn: big.NewInt(100), AmountOut: big.NewInt(90), Deadline: uint32(now.Add(time.Minute).Unix()),
	}

	if _, err := solver.startFill(t.Context(), []liquidlane.Route{directRoute}, order, now, now); err != nil {
		t.Fatalf("startFill: %v", err)
	}
	if len(reader.fillAmounts) != 1 || reader.fillAmounts[0].Cmp(big.NewInt(100)) != 0 ||
		len(reader.fillQuoteAmounts) != 1 || reader.fillQuoteAmounts[0].Cmp(big.NewInt(40)) != 0 {
		t.Fatalf("fill snapshot/quote amounts = %v/%v, want [100]/[40]",
			reader.fillAmounts, reader.fillQuoteAmounts)
	}
	if len(reader.fillQuoteRoutes) != 1 || len(reader.fillQuoteRoutes[0]) != 1 ||
		reader.fillQuoteRoutes[0][0].ID != discountRoute.ID {
		t.Fatalf("repriced routes = %+v, want only %s", reader.fillQuoteRoutes, discountRoute.ID)
	}
	if len(packed.Routes) != 1 || len(packed.DiscountRoutes) != 1 ||
		packed.DiscountRoutes[0].AmountIn.Cmp(big.NewInt(40)) != 0 {
		t.Fatalf("packed split fill = %+v", packed)
	}
}

func (r *executionTestReader) Fill(
	_ context.Context,
	routes []liquidlane.Route,
	_ common.Address,
	_ common.Address,
	amountIn *big.Int,
	_ time.Time,
) (fillSnapshot, error) {
	r.fillRoutes = append([]liquidlane.Route(nil), routes...)
	r.fillAmounts = append(r.fillAmounts, new(big.Int).Set(amountIn))
	if r.fillSnapshotFn != nil {
		return r.fillSnapshotFn(routes, amountIn), nil
	}
	return r.snapshot, nil
}

func (r *executionTestReader) Quote(
	context.Context,
	[]liquidlane.Route,
	common.Address,
	time.Time,
) (snapshot, error) {
	return r.quoteSnapshot, nil
}

func (r *executionTestReader) ReadFillQuotes(
	_ context.Context,
	routes []liquidlane.Route,
	_ common.Address,
	amountIn *big.Int,
) ([]liquidlane.FillQuote, error) {
	r.fillQuoteRoutes = append(r.fillQuoteRoutes, append([]liquidlane.Route(nil), routes...))
	r.fillQuoteAmounts = append(r.fillQuoteAmounts, new(big.Int).Set(amountIn))
	if r.fillSnapshotFn != nil {
		return r.fillSnapshotFn(routes, amountIn).Physical, nil
	}
	return r.snapshot.Physical, nil
}

func TestStartFillRejectsExpiredOrderBeforeStrategy(t *testing.T) {
	now := time.Unix(1_000, 0)
	strategy := &executionTestStrategy{}
	solver := &Solver{strategy: strategy}
	order := &resolvedOrder{
		TokenOut: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Deadline: uint32(now.Unix()),
	}

	if _, err := solver.startFill(t.Context(), nil, order, now, now); !errors.Is(err, errOrderNotFillable) {
		t.Fatalf("startFill error = %v, want %v", err, errOrderNotFillable)
	}
	if strategy.input.OrderID != "" {
		t.Fatal("expired order reached strategy")
	}
}

type executionTestStrategy struct {
	input   strategytypes.FillInput
	plan    *strategytypes.FillPlan
	quote   *strategytypes.Quote
	onInput func(strategytypes.FillInput)
}

func (s *executionTestStrategy) DecideQuote(
	context.Context,
	strategytypes.QuoteInput,
) (*strategytypes.Quote, error) {
	return s.quote, nil
}

func (s *executionTestStrategy) DecideFill(
	_ context.Context,
	input strategytypes.FillInput,
) (*strategytypes.FillPlan, error) {
	s.input = input
	if s.onInput != nil {
		s.onInput(input)
	}
	return s.plan, nil
}

type executionTestTxManager struct {
	result      chan txmanager.Result
	maxFeeReads int
	reqs        []txmanager.Request
	unavailable bool
	busy        bool
	reject      bool
	accepted    chan struct{}
}

func (m *executionTestTxManager) MaxFeePerGas(context.Context) (*big.Int, error) {
	m.maxFeeReads++
	return new(big.Int), nil
}

func (m *executionTestTxManager) Available() bool { return !m.unavailable }
func (m *executionTestTxManager) LaneReady() bool { return !m.unavailable && !m.busy }

func (m *executionTestTxManager) SendAsync(
	_ context.Context,
	request txmanager.Request,
) (<-chan txmanager.Result, bool) {
	m.reqs = append(m.reqs, request)
	if m.reject {
		return nil, false
	}
	if m.accepted != nil {
		select {
		case m.accepted <- struct{}{}:
		default:
		}
	}
	return m.result, true
}

func (m *executionTestTxManager) complete(result txmanager.Result) {
	m.result <- result
	m.busy = false
}

type contractCallerFunc func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)

func (f contractCallerFunc) CallContract(
	ctx context.Context,
	call ethereum.CallMsg,
	blockNumber *big.Int,
) ([]byte, error) {
	return f(ctx, call, blockNumber)
}

type uniswapFillTrace struct {
	Stage       string
	Mode        string
	Outcome     string
	RouteID     string
	CandidateID string
	Adapter     string
	CapacityID  string
	DiscountID  string
	Amount      string
	ChainTime   string
	MaxFee      string
	GasFacts    bool
	Quotes      int
	Pending     int
	Capacities  []string
	Direct      bool
	Signed      bool
	Preflight   bool
	Repriced    bool
	Resolved    bool
	Quoted      bool
	Failure     bool
	Closed      bool
	Attempts    int
}

type uniswapLifecycleFixture struct {
	now      time.Time
	route    liquidlane.Route
	routes   []liquidlane.Route
	order    *resolvedOrder
	solver   *Solver
	strategy *executionTestStrategy
	txm      *executionTestTxManager
	packed   *uxexecutor.ILiquidLaneUniswapXExecutorFillCall
	refresh  chan struct{}
}

func TestFillReservationLifecycleTrace(t *testing.T) {
	for _, mode := range []string{"direct", "signed-discount"} {
		t.Run(mode, func(t *testing.T) {
			for _, outcome := range []string{"success", "error", "cancellation", "closed", "not-admitted"} {
				t.Run(outcome, func(t *testing.T) {
					runUniswapFillReservationLifecycle(t, mode, outcome)
				})
			}
		})
	}
}

func runUniswapFillReservationLifecycle(t *testing.T, mode, outcome string) {
	t.Helper()
	assertUniswapRejectedAdmission(t, mode)

	fixture := newUniswapLifecycleFixture(t, mode, false)
	trace := []uniswapFillTrace{{Stage: "rejected", Mode: mode}}
	plan := fixture.strategy.plan
	fixture.strategy.onInput = func(input strategytypes.FillInput) {
		trace = append(trace,
			uniswapFillTrace{
				Stage: "facts", Mode: mode, ChainTime: strconv.FormatBool(input.ChainTime.Equal(fixture.now)),
				MaxFee: input.MaxFeePerGas.String(), GasFacts: input.GasPrices != nil || input.GasSnapshot != nil,
				Quotes: len(input.Quotes), Capacities: uniswapCapacityEntries(input.Reservations),
			},
			uniswapFillTrace{
				Stage: "strategy", Mode: mode, RouteID: string(plan.Routes[0].RouteID),
				Adapter: plan.Routes[0].Adapter.Hex(), CapacityID: string(plan.Routes[0].CapacityID),
				DiscountID: uniswapHashString(plan.Routes[0].DiscountID), Amount: plan.Routes[0].ReservedAmountOut.String(),
			},
		)
	}
	if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
		t.Fatal("claim order")
	}
	orders := make(chan *resolvedOrder, 1)
	orders <- fixture.order
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- fixture.solver.newFillWorker(t.Context(), fixture.routes, orders).run()
	}()
	select {
	case <-fixture.txm.accepted:
	case err := <-done:
		t.Fatalf("worker returned before admission: %v, strategy=%+v", err, fixture.strategy.input)
	case <-time.After(3 * time.Second):
		t.Fatal("fill was not admitted")
	}
	expectUniswapRefresh(t, fixture.refresh)
	select {
	case err := <-done:
		t.Fatalf("worker returned before terminal result: %v", err)
	default:
	}
	if len(fixture.txm.reqs) != 1 {
		t.Fatalf("accepted requests = %d, want 1", len(fixture.txm.reqs))
	}
	declined, err := fixture.solver.quote(
		t.Context(), validQuoteRequest(fixture.route.TokenIn, fixture.route.TokenOut),
	)
	if err != nil || declined.AmountOut != "0" {
		t.Fatalf("quote during accepted fill = %+v, err %v", declined, err)
	}
	reader := fixture.solver.reader.(*executionTestReader)
	repriced := mode == "signed-discount" && len(reader.fillQuoteAmounts) == 1 &&
		reader.fillQuoteAmounts[0].Cmp(big.NewInt(100)) == 0
	resolved := mode == "signed-discount" && len(fixture.packed.DiscountRoutes) == 1 &&
		fixture.packed.DiscountRoutes[0].DiscountSwap.Discount.TokenToRedeem == fixture.route.TokenIn
	if mode == "signed-discount" && (!repriced || !resolved) {
		t.Fatalf("selected discount repricing/terms = %v/%+v", reader.fillQuoteAmounts, fixture.packed.DiscountRoutes)
	}
	trace = append(trace,
		uniswapFillTrace{
			Stage: "validated", Mode: mode, RouteID: string(plan.Routes[0].RouteID),
			CandidateID: string(plan.Routes[0].CandidateID), Adapter: plan.Routes[0].Adapter.Hex(),
			CapacityID: string(plan.Routes[0].CapacityID), DiscountID: uniswapHashString(plan.Routes[0].DiscountID),
			Amount: plan.Routes[0].ReservedAmountOut.String(),
		},
		uniswapFillTrace{
			Stage: "calldata", Mode: mode, Direct: len(fixture.packed.Routes) == 1,
			Signed: len(fixture.packed.DiscountRoutes) == 1, Preflight: true,
			Repriced: repriced, Resolved: resolved,
		},
		uniswapFillTrace{
			Stage: "admitted", Mode: mode, Pending: fixture.solver.capacity.Len(),
			Capacities: uniswapCapacityEntries(fixture.solver.capacity.Snapshot()),
			Quoted:     false,
		},
	)
	tracked := completeUniswapLifecycle(t, fixture, done, outcome)
	trace = append(trace, uniswapFillTrace{
		Stage: "terminal", Mode: mode, Outcome: outcome, Pending: fixture.solver.capacity.Len(),
		Quoted: true, Failure: outcome != "success" && outcome != "not-admitted",
		Closed: outcome == "closed", Attempts: tracked.attempts,
	})

	discount := uniswapHashString(plan.Routes[0].DiscountID)
	attempts := 0
	failure := false
	if outcome != "success" && outcome != "not-admitted" {
		attempts = 1
		failure = true
	}
	want := []uniswapFillTrace{
		{Stage: "rejected", Mode: mode},
		{Stage: "facts", Mode: mode, ChainTime: "true", MaxFee: "0", Quotes: 1, Capacities: []string{}},
		{Stage: "strategy", Mode: mode, RouteID: string(fixture.route.ID), Adapter: common.HexToAddress("0xdead").Hex(), CapacityID: "forged-capacity", DiscountID: discount, Amount: "100"},
		{Stage: "validated", Mode: mode, RouteID: string(fixture.route.ID), CandidateID: string(liquidlane.NewCandidateID(fixture.route, plan.Routes[0].DiscountID)), Adapter: fixture.route.Adapter.Hex(), CapacityID: string(fixture.route.CapacityID), DiscountID: discount, Amount: "100"},
		{Stage: "calldata", Mode: mode, Direct: mode == "direct", Signed: mode == "signed-discount", Preflight: true, Repriced: mode == "signed-discount", Resolved: mode == "signed-discount"},
		{Stage: "admitted", Mode: mode, Pending: 1, Capacities: []string{string(fixture.route.CapacityID) + "=100"}},
		{Stage: "terminal", Mode: mode, Outcome: outcome, Quoted: true, Failure: failure, Closed: outcome == "closed", Attempts: attempts},
	}
	if !reflect.DeepEqual(trace, want) {
		t.Fatalf("lifecycle trace =\n%#v\nwant\n%#v", trace, want)
	}
}

func assertUniswapRejectedAdmission(t *testing.T, mode string) {
	t.Helper()
	fixture := newUniswapLifecycleFixture(t, mode, true)
	initialState := fixture.solver.quoteState.Load()
	initialEpoch := fixture.solver.quoteEpoch.Load()
	pending, err := fixture.solver.startFill(
		t.Context(), fixture.routes, fixture.order, fixture.now, time.Now(),
	)
	if err == nil || pending != nil || fixture.solver.capacity.Len() != 0 ||
		fixture.solver.quoteState.Load() != initialState || fixture.solver.quoteEpoch.Load() != initialEpoch {
		t.Fatalf("rejected admission changed lifecycle state: pending=%v err=%v capacity=%d epoch=%d",
			pending, err, fixture.solver.capacity.Len(), fixture.solver.quoteEpoch.Load())
	}
	select {
	case <-fixture.refresh:
		t.Fatal("rejected admission requested quote refresh")
	default:
	}
}

func completeUniswapLifecycle(
	t *testing.T,
	fixture *uniswapLifecycleFixture,
	done <-chan error,
	outcome string,
) trackedOrder {
	t.Helper()
	switch outcome {
	case "success":
		fixture.txm.result <- txmanager.Result{Hash: common.HexToHash("0x2")}
	case "error":
		fixture.txm.result <- txmanager.Result{Err: errors.New("terminal send failure")}
	case "cancellation":
		fixture.txm.result <- txmanager.Result{Err: context.Canceled}
	case "closed":
		close(fixture.txm.result)
	case "not-admitted":
		fixture.txm.result <- txmanager.Result{Err: errors.New("not admitted"), NotAdmitted: true}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not consume terminal result")
	}
	expectUniswapRefresh(t, fixture.refresh)
	if fixture.solver.quoteState.Load() != nil || fixture.solver.capacity.Len() != 0 {
		t.Fatal("terminal result released capacity without retaining invalidated quote state")
	}
	declined, err := fixture.solver.quote(
		t.Context(), validQuoteRequest(fixture.route.TokenIn, fixture.route.TokenOut),
	)
	if err != nil || declined.AmountOut != "0" {
		t.Fatalf("quote before refreshed state = %+v, err %v", declined, err)
	}
	if err := fixture.solver.refreshQuoteState(t.Context(), fixture.routes); err != nil {
		t.Fatalf("refresh quote state: %v", err)
	}
	if fixture.solver.quoteState.Load() == nil {
		t.Fatalf("refreshed quote state was not published: planning=%d epoch=%d",
			fixture.solver.planningFills.Load(), fixture.solver.quoteEpoch.Load())
	}
	quoted, err := fixture.solver.quote(
		t.Context(), validQuoteRequest(fixture.route.TokenIn, fixture.route.TokenOut),
	)
	if err != nil || quoted.AmountOut != "90" {
		t.Fatalf("quote after refreshed state = %+v, err %v", quoted, err)
	}
	return fixture.solver.orderState[fixture.order.Hash]
}

func newUniswapLifecycleFixture(t *testing.T, mode string, reject bool) *uniswapLifecycleFixture {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	route := testDiscountRoute()
	discountID := (*common.Hash)(nil)
	if mode == "signed-discount" {
		discountID = new(common.HexToHash(testDiscountID))
	}
	plan := &strategytypes.FillPlan{Routes: []strategytypes.FillRoute{{
		CandidateID: "forged-candidate", RouteID: route.ID,
		CapacityID: "forged-capacity", Adapter: common.HexToAddress("0xdead"),
		AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(100), MinAmountOut: big.NewInt(90),
		ReservedAmountOut: big.NewInt(100), DiscountID: liquidlane.CloneHash(discountID),
	}}}
	inventory := testInventoryWithMinDiscount(
		route, big.NewInt(100), big.NewInt(1_000_000_000_000_000_000), new(big.Int),
	)
	fillQuote := liquidlane.FillQuote{
		Inventory: inventory, AmountIn: big.NewInt(100), GrossAmountOut: big.NewInt(100),
		MaxAmountOut: big.NewInt(100), MinDiscount: new(big.Int),
	}
	reader := &executionTestReader{now: now}
	cfg := &Config{
		Executor:    route.Adapter,
		OrderServer: OrderServerConfig{PollInterval: time.Second},
		QuoteServer: QuoteServerConfig{QuoteTTL: time.Minute},
		Breaker:     BreakerConfig{MaxFailures: 100, Window: time.Minute},
	}
	routes := []liquidlane.Route{route}
	if mode == "signed-discount" {
		cfg.SolverMode = solverModeInternal
		cfg.Discounts = &DiscountConfig{HTTPTimeout: time.Second, MinimumValidity: 15 * time.Second}
		reader.resolved = []liquidlane.Route{route}
		reader.snapshot = fillSnapshot{Physical: []liquidlane.FillQuote{fillQuote}}
		reader.quoteSnapshot = snapshot{Physical: []liquidlane.Inventory{inventory}}
		routes = nil
	} else {
		reader.snapshot = fillSnapshot{Direct: []liquidlane.FillQuote{fillQuote}}
		reader.quoteSnapshot = snapshot{Direct: []liquidlane.Inventory{inventory}}
	}
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatalf("token policy: %v", err)
	}
	cfg.TokenPolicy = policy
	strategy := &executionTestStrategy{
		plan: plan, quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)},
	}
	txm := &executionTestTxManager{
		result: make(chan txmanager.Result, 1), reject: reject, accepted: make(chan struct{}, 1),
	}
	packed := new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
	solver := &Solver{
		chainID: 1, cfg: cfg,
		solverAddress: common.HexToAddress("0x8888888888888888888888888888888888888888"),
		chain: contractCallerFunc(func(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			parsed, err := uxexecutor.LiquidLaneUniswapXExecutorMetaData.ParseABI()
			if err != nil {
				return nil, err
			}
			values, err := parsed.Methods["execute"].Inputs.Unpack(call.Data[4:])
			if err != nil {
				return nil, err
			}
			*packed = *abi.ConvertType(
				values[1], new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall),
			).(*uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
			return nil, nil
		}),
		reader: reader, strategy: strategy, txm: txm, log: logr.Discard(),
		orderState: make(map[common.Hash]trackedOrder), refreshCh: make(chan struct{}, 4),
	}
	if mode == "signed-discount" {
		deadline := now.Add(time.Minute).Unix()
		solver.discounts = &fakeDiscountProvider{
			list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
				testDiscountOffer(route, now.Add(time.Minute), "100", "1000000000000000000"),
			}},
			resolved: &liquiddiscounts.Resolved{
				DiscountID: testDiscountID,
				Discount: liquiddiscounts.Terms{
					Adapter: route.Adapter.Hex(), TokenToRedeem: route.TokenIn.Hex(), Discount: "0",
					Signer:   common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
					Protocol: common.HexToAddress("0x6666666666666666666666666666666666666666").Hex(),
					Nonce:    "0x1", Deadline: deadline,
				},
				SignerSignature: "0x01", ProtocolDeadline: deadline, ProtocolSignature: "0x02",
			},
		}
	}
	solver.quoteState.Store(&quoteState{
		inventory: []liquidlane.Inventory{inventory}, chainTime: now,
		expiresAt: time.Now().Add(time.Minute), singleRouteFor: map[common.Address]bool{},
	})
	order := &resolvedOrder{
		Encoded: []byte{1}, Signature: []byte{2}, Hash: common.HexToHash("0x1234"), QuoteID: "quote-1",
		Source: orderSourcePublicV2, Executor: cfg.Executor,
		TokenIn: route.TokenIn, TokenOut: route.TokenOut, AmountIn: big.NewInt(100), AmountOut: big.NewInt(90),
		Deadline: uint32(now.Add(time.Minute).Unix()),
	}
	return &uniswapLifecycleFixture{
		now: now, route: route, routes: routes, order: order, solver: solver,
		strategy: strategy, txm: txm, packed: packed, refresh: solver.refreshCh,
	}
}

func uniswapHashString(hash *common.Hash) string {
	if hash == nil {
		return ""
	}
	return hash.Hex()
}

func uniswapCapacityEntries(reservations liquidlane.CapacityReservations) []string {
	entries := make([]string, 0, len(reservations))
	for capacityID, amount := range reservations {
		entries = append(entries, string(capacityID)+"="+amount.String())
	}
	slices.Sort(entries)
	return entries
}

func expectUniswapRefresh(t *testing.T, refresh <-chan struct{}) {
	t.Helper()
	select {
	case <-refresh:
	case <-time.After(3 * time.Second):
		t.Fatal("quote refresh was not requested")
	}
}

type directExecutionFixture struct {
	now      time.Time
	route    liquidlane.Route
	order    *resolvedOrder
	solver   *Solver
	strategy *executionTestStrategy
	txm      *executionTestTxManager
	packed   *uxexecutor.ILiquidLaneUniswapXExecutorFillCall
}

func newDirectExecutionFixture(t *testing.T) *directExecutionFixture {
	t.Helper()
	now := time.Now()
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	executor := common.HexToAddress("0x3333333333333333333333333333333333333333")
	adapter := common.HexToAddress("0x4444444444444444444444444444444444444444")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter,
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 18, TokenOutDecimals: 18,
	}
	strategy := &executionTestStrategy{plan: &strategytypes.FillPlan{Routes: []strategytypes.FillRoute{{
		RouteID: route.ID, CapacityID: route.CapacityID, Adapter: adapter,
		AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(100), MinAmountOut: big.NewInt(90),
		ReservedAmountOut: big.NewInt(100),
	}}}}
	txm := &executionTestTxManager{result: make(chan txmanager.Result, 1)}
	packed := new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
	solver := &Solver{
		cfg:           &Config{Executor: executor, OrderServer: OrderServerConfig{PollInterval: time.Second}},
		solverAddress: common.HexToAddress("0x5555555555555555555555555555555555555555"),
		chain: contractCallerFunc(func(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			parsed, err := uxexecutor.LiquidLaneUniswapXExecutorMetaData.ParseABI()
			if err != nil {
				return nil, err
			}
			values, err := parsed.Methods["execute"].Inputs.Unpack(call.Data[4:])
			if err != nil {
				return nil, err
			}
			*packed = *abi.ConvertType(
				values[1], new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall),
			).(*uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
			return nil, nil
		}),
		reader: &executionTestReader{now: now, snapshot: fillSnapshot{Direct: []liquidlane.FillQuote{{
			Inventory: liquidlane.DirectInventory(route, big.NewInt(100), big.NewInt(1_000_000_000_000_000_000)),
			AmountIn:  big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}}}}, strategy: strategy, txm: txm, log: logr.Discard(),
		orderState: make(map[common.Hash]trackedOrder),
	}
	order := &resolvedOrder{
		Encoded: []byte{1}, Signature: []byte{2}, Hash: common.HexToHash("0x1"), QuoteID: "quote-1",
		Source:   orderSourceExclusiveV2,
		Executor: executor, TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), AmountOut: big.NewInt(90),
		Deadline: uint32(now.Add(time.Minute).Unix()), ExclusiveUntil: uint64(now.Add(30 * time.Second).Unix()),
	}
	solver.trackExclusive(order, now)
	return &directExecutionFixture{
		now: now, route: route, order: order, solver: solver, strategy: strategy, txm: txm, packed: packed,
	}
}

func TestStartFillSubmitsAsynchronouslyAndReservesCapacity(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	chainObservedAt := fixture.now.Add(-10 * time.Second)
	pending, err := fixture.solver.startFill(
		t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, chainObservedAt,
	)
	if err != nil {
		t.Fatalf("startFill: %v", err)
	}
	if pending == nil || len(fixture.txm.reqs) != 1 {
		t.Fatalf("pending/requests = %v/%d", pending, len(fixture.txm.reqs))
	}
	if fixture.strategy.input.MaxFeePerGas.Sign() != 0 {
		t.Fatalf("strategy max fee = %v, want zero with gas accounting disabled", fixture.strategy.input.MaxFeePerGas)
	}
	if fixture.txm.maxFeeReads != 0 {
		t.Fatalf("gas-disabled fill read max fee %d times", fixture.txm.maxFeeReads)
	}
	if fixture.txm.reqs[0].MaxFeePerGas != nil {
		t.Fatalf("gas-disabled transaction hard-capped fees at %s", fixture.txm.reqs[0].MaxFeePerGas)
	}
	wantCancelAt := time.Unix(int64(fixture.order.Deadline), 0).Add(-10 * time.Second)
	if got := fixture.txm.reqs[0].CancelAt; got.Sub(wantCancelAt).Abs() > time.Millisecond {
		t.Fatalf("transaction cancelAt = %s, want %s", got, wantCancelAt)
	}
	if fixture.txm.reqs[0].Confirmations != nil {
		t.Fatalf("fill confirmations override = %d, want global txmanager configuration", *fixture.txm.reqs[0].Confirmations)
	}
	if len(fixture.packed.Routes) != 1 || fixture.packed.Routes[0].AmountOut.Cmp(big.NewInt(90)) != 0 ||
		len(fixture.packed.DiscountRoutes) != 0 {
		t.Fatalf("packed direct fill call = %+v", fixture.packed)
	}
	if got := fixture.solver.capacity.Snapshot()[fixture.route.CapacityID]; got == nil || got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("pending reservation = %v", got)
	}
	if reservations := fixture.strategy.input.Reservations; len(reservations) != 0 {
		t.Fatalf("unexpected pre-existing reservations: %v", reservations)
	}
	fixture.txm.result <- txmanager.Result{Hash: common.HexToHash("0x2")}
	result := <-pending.result
	fixture.solver.completePendingFill(uniswapFillCompletion{fill: pending, result: result})
	if fixture.solver.capacity.Len() != 0 {
		t.Fatal("pending reservation was not released")
	}
	if !fixture.solver.exclusiveState[fixture.order.Hash].pending() {
		t.Fatal("successful tx cleared exclusive obligation before its canonical block time was reconciled")
	}
}

func TestFillLoopKeepsQuotesBlockedUntilAcceptedLifecycleCompletes(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	refreshes := make(chan struct{}, 2)
	fixture.solver.refreshCh = refreshes
	accepted := make(chan struct{}, 1)
	fixture.txm.accepted = accepted
	fixture.txm.busy = true
	if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
		t.Fatal("order was not claimed")
	}
	orders := make(chan *resolvedOrder, 1)
	orders <- fixture.order
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- fixture.solver.newFillWorker(t.Context(), []liquidlane.Route{fixture.route}, orders).run()
	}()

	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("fill was not accepted")
	}
	waitForQuoteRefreshes(t, refreshes)
	if fixture.solver.planningFills.Load() != 0 || fixture.solver.capacity.Len() != 1 {
		t.Fatal("accepted fill did not publish its reservation and finish planning")
	}
	if !fixture.solver.quoteBlocked(time.Now().Unix()) {
		t.Fatal("accepted transaction lifecycle did not block quoting after fill planning completed")
	}

	fixture.txm.complete(txmanager.Result{Hash: common.HexToHash("0x2")})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fill loop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fill loop did not finish after transaction lifecycle completed")
	}
	if fixture.solver.quoteBlocked(time.Now().Unix()) {
		t.Fatal("completed transaction lifecycle kept quoting blocked")
	}
}

func TestFillLoopDrainsAcceptedFillAfterQuoteServerFailure(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	refreshes := make(chan struct{}, 2)
	fixture.solver.refreshCh = refreshes
	accepted := make(chan struct{}, 1)
	fixture.txm.accepted = accepted
	if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
		t.Fatal("order was not claimed")
	}
	orders := make(chan *resolvedOrder, 1)
	orders <- fixture.order
	close(orders)
	runCtx, reportFatal := context.WithCancelCause(t.Context())
	fixture.solver.reportFatal = reportFatal
	done := make(chan error, 1)
	go func() {
		done <- fixture.solver.newFillWorker(runCtx, []liquidlane.Route{fixture.route}, orders).run()
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("fill was not accepted")
	}
	waitForQuoteRefreshes(t, refreshes)
	if fixture.solver.planningFills.Load() != 0 || fixture.solver.capacity.Len() != 1 {
		t.Fatal("accepted fill did not publish its reservation and finish planning")
	}
	listenErr := errors.New("accept failed")
	server := &http.Server{ReadHeaderTimeout: time.Second}
	serveErr := fixture.solver.serveQuoteServer(runCtx, server, failingListener{err: listenErr})
	if !errors.Is(serveErr, listenErr) {
		t.Fatalf("serve error = %v, want %v", serveErr, listenErr)
	}
	if cause := context.Cause(runCtx); !errors.Is(cause, listenErr) {
		t.Fatalf("runtime cancellation cause = %v, want %v", cause, listenErr)
	}
	select {
	case err := <-done:
		t.Fatalf("fill loop returned before accepted lifecycle completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	fixture.txm.result <- txmanager.Result{Hash: common.HexToHash("0x2")}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("fill loop result = %v, want context cancellation after drain", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fill loop did not finish after accepted lifecycle completed")
	}
	tracked := fixture.solver.orderState[fixture.order.Hash]
	if fixture.solver.capacity.Len() != 0 || tracked.inFlight {
		t.Fatal("drained fill retained reservation or in-flight state")
	}
	if tracked.completedAt.IsZero() {
		t.Fatal("drained fill was not terminalized")
	}
}

func TestFillLoopDropsQueuedOrderAfterCancellation(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
		t.Fatal("order was not claimed")
	}
	orders := make(chan *resolvedOrder, 1)
	orders <- fixture.order
	close(orders)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := fixture.solver.newFillWorker(ctx, []liquidlane.Route{fixture.route}, orders).run()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fill loop result = %v, want context cancellation", err)
	}
	if len(fixture.txm.reqs) != 0 || fixture.solver.planningFills.Load() != 0 ||
		fixture.solver.orderState[fixture.order.Hash].inFlight {
		t.Fatal("queued order was submitted or retained after cancellation")
	}
}

func TestFillLoopDefersQueuedOrderWhileNonceLaneUnavailable(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	fixture.txm.unavailable = true
	if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
		t.Fatal("order was not claimed")
	}
	orders := make(chan *resolvedOrder, 1)
	orders <- fixture.order
	close(orders)

	if err := fixture.solver.newFillWorker(
		t.Context(), []liquidlane.Route{fixture.route}, orders,
	).run(); err != nil {
		t.Fatalf("fill loop: %v", err)
	}
	reader := fixture.solver.reader.(*executionTestReader)
	if reader.latestBlockReads != 0 || fixture.strategy.input.OrderID != "" || len(fixture.txm.reqs) != 0 ||
		len(fixture.packed.Routes) != 0 {
		t.Fatalf(
			"unavailable lane performed fill work: chainReads=%d strategyOrder=%q requests=%d packedRoutes=%d",
			reader.latestBlockReads, fixture.strategy.input.OrderID, len(fixture.txm.reqs), len(fixture.packed.Routes),
		)
	}
	tracked := fixture.solver.orderState[fixture.order.Hash]
	if fixture.solver.planningFills.Load() != 0 || tracked.inFlight || tracked.attempts != 0 {
		t.Fatal("deferred order retained planning/in-flight state or counted as a failed attempt")
	}
	retryAt := tracked.retryAt
	if retryAt.IsZero() {
		t.Fatal("deferred order did not receive a normal retry")
	}
	if fixture.solver.claim(fixture.order.Hash, retryAt.Add(-time.Nanosecond)) {
		t.Fatal("deferred order was reclaimable before its normal retry")
	}
	if !fixture.solver.claim(fixture.order.Hash, retryAt) {
		t.Fatal("deferred order was not reclaimable at its normal retry")
	}
	fixture.solver.endFillPlanning()
}

func TestCompletePendingFillClassifiesNotAdmittedWithoutFailure(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	fixture.order.Source = orderSourcePublicV2
	fixture.solver.cfg.Breaker = BreakerConfig{MaxFailures: 1, Window: time.Minute}
	metrics, err := newUniswapXMetrics(prometheus.NewRegistry(), fixture.solver.ready)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	fixture.solver.metrics = metrics
	fixture.solver.orderState[fixture.order.Hash] = trackedOrder{inFlight: true}
	fixture.solver.setPendingReservations(
		fixture.order.Hash,
		liquidlane.CapacityReservations{fixture.route.CapacityID: big.NewInt(100)},
	)
	pending := &pendingUniswapFill{order: fixture.order}

	fixture.solver.completePendingFill(uniswapFillCompletion{
		fill: pending,
		result: txmanager.Result{
			Err:         errors.New("transaction was not admitted"),
			NotAdmitted: true,
		},
	})

	tracked := fixture.solver.orderState[fixture.order.Hash]
	if fixture.solver.capacity.Len() != 0 || tracked.inFlight {
		t.Fatal("not-admitted fill retained reservation or in-flight state")
	}
	if tracked.attempts != 0 || len(fixture.solver.failureTimes) != 0 ||
		fixture.solver.localBlockUntil.Load() != 0 {
		t.Fatal("not-admitted fill counted toward retry or fade breaker failures")
	}
	if got := testutil.ToFloat64(metrics.fills.WithLabelValues("not-admitted")); got != 1 {
		t.Fatalf("not-admitted fill metric = %v, want 1", got)
	}
}

func waitForQuoteRefreshes(t *testing.T, refreshes <-chan struct{}) {
	t.Helper()
	for range 2 {
		select {
		case <-refreshes:
		case <-time.After(time.Second):
			t.Fatal("quote refresh was not requested")
		}
	}
}

func TestExclusiveExecutionFailureWaitsForTerminalReconciliation(t *testing.T) {
	now := time.Now()
	solver := &Solver{
		cfg: &Config{Breaker: BreakerConfig{MaxFailures: 1, Window: time.Minute}},
		log: logr.Discard(),
	}

	solver.recordOrderFillFailure(&resolvedOrder{Source: orderSourceExclusiveV2}, now)

	if len(solver.failureTimes) != 0 || solver.localBlockUntil.Load() != 0 {
		t.Fatal("exclusive execution failure opened the ordinary local breaker")
	}

	solver.recordOrderFillFailure(&resolvedOrder{Source: orderSourcePublicV2}, now)

	if solver.localBlockUntil.Load() == 0 {
		t.Fatal("public execution failure did not open the ordinary local breaker")
	}
}
