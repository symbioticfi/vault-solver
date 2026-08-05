package uniswapx

import (
	"context"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

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
	fillSnapshotFn   func([]liquidlane.Route, *big.Int) fillSnapshot
	now              time.Time
}

type failingListener struct{ err error }

func (l failingListener) Accept() (net.Conn, error) { return nil, l.err }
func (failingListener) Close() error                { return nil }
func (failingListener) Addr() net.Addr              { return &net.TCPAddr{} }

func (r *executionTestReader) resolveRoutes(
	_ context.Context,
	adapters []common.Address,
) ([]liquidlane.Route, error) {
	r.adapters = append([]common.Address(nil), adapters...)
	return append([]liquidlane.Route(nil), r.resolved...), nil
}

func (r *executionTestReader) validateGasTokens([]liquidlane.Route) error { return nil }

func (r *executionTestReader) latestBlockTime(context.Context) (time.Time, error) {
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
		ReservedAmountOut: big.NewInt(100), DiscountID: hashPointer(common.HexToHash(testDiscountID)),
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
		filled: make(map[common.Hash]time.Time), retryAt: make(map[common.Hash]time.Time),
		inFlight: make(map[common.Hash]bool), attempts: make(map[common.Hash]int),
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
			ReservedAmountOut: big.NewInt(40), DiscountID: hashPointer(common.HexToHash(testDiscountID)),
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
		filled: make(map[common.Hash]time.Time), retryAt: make(map[common.Hash]time.Time),
		inFlight: make(map[common.Hash]bool), attempts: make(map[common.Hash]int),
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

func hashPointer(hash common.Hash) *common.Hash { return &hash }

func (r *executionTestReader) fillSnapshot(
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

func (r *executionTestReader) physicalFillQuotes(
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

func TestFillCancellationDeadlineUsesChainClock(t *testing.T) {
	chainObservedAt := time.Unix(1_000, 0)
	chainNow := time.Unix(1_010, 0)
	deadline := time.Unix(1_030, 0)
	cancelAt, ok := fillCancellationDeadline(deadline, chainNow, chainObservedAt, chainObservedAt)
	if !ok || !cancelAt.Equal(time.Unix(1_020, 0)) {
		t.Fatalf("cancelAt = %s, %v; want 1020, true", cancelAt, ok)
	}

	wallNow := chainObservedAt.Add(15 * time.Second)
	cancelAt, ok = fillCancellationDeadline(deadline, chainNow, chainObservedAt, wallNow)
	if !ok || !cancelAt.Equal(time.Unix(1_020, 0)) {
		t.Fatalf("cancelAt after planning = %s, %v; want 1020, true", cancelAt, ok)
	}
	if _, valid := fillCancellationDeadline(
		deadline, chainNow, chainObservedAt, chainObservedAt.Add(20*time.Second),
	); valid {
		t.Fatal("expired chain deadline was accepted")
	}
}

type executionTestStrategy struct {
	input strategytypes.FillInput
	plan  *strategytypes.FillPlan
}

func (s *executionTestStrategy) DecideQuote(
	context.Context,
	strategytypes.QuoteInput,
) (*strategytypes.Quote, error) {
	return nil, nil
}

func (s *executionTestStrategy) DecideFill(
	_ context.Context,
	input strategytypes.FillInput,
) (*strategytypes.FillPlan, error) {
	s.input = input
	return s.plan, nil
}

type executionTestTxManager struct {
	result      chan txmanager.Result
	maxFeeReads int
	reqs        []txmanager.Request
	unavailable bool
	accepted    chan<- struct{}
}

func (m *executionTestTxManager) MaxFeePerGas(context.Context) (*big.Int, error) {
	m.maxFeeReads++
	return new(big.Int), nil
}

func (m *executionTestTxManager) Available() bool { return !m.unavailable }

func (m *executionTestTxManager) SendAsync(
	_ context.Context,
	request txmanager.Request,
) (<-chan txmanager.Result, bool) {
	m.reqs = append(m.reqs, request)
	if m.accepted != nil {
		select {
		case m.accepted <- struct{}{}:
		default:
		}
	}
	return m.result, true
}

type contractCallerFunc func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)

func (f contractCallerFunc) CallContract(
	ctx context.Context,
	call ethereum.CallMsg,
	blockNumber *big.Int,
) ([]byte, error) {
	return f(ctx, call, blockNumber)
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
		filled: make(map[common.Hash]time.Time), retryAt: make(map[common.Hash]time.Time),
		inFlight: make(map[common.Hash]bool), attempts: make(map[common.Hash]int),
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
	if _, pending := fixture.solver.exclusiveUntil[fixture.order.Hash]; !pending {
		t.Fatal("successful tx cleared exclusive obligation before its canonical block time was reconciled")
	}
}

func TestFillLoopDrainsAcceptedFillAfterQuoteServerFailure(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
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
	go func() { done <- fixture.solver.fillLoop(runCtx, []liquidlane.Route{fixture.route}, orders) }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("fill was not accepted")
	}
	waitForExecutionCondition(t, func() bool {
		return fixture.solver.planningFills.Load() == 0 && fixture.solver.capacity.Len() == 1
	})
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
	if fixture.solver.capacity.Len() != 0 || fixture.solver.inFlight[fixture.order.Hash] {
		t.Fatal("drained fill retained reservation or in-flight state")
	}
	if _, filled := fixture.solver.filled[fixture.order.Hash]; !filled {
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

	err := fixture.solver.fillLoop(ctx, []liquidlane.Route{fixture.route}, orders)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fill loop result = %v, want context cancellation", err)
	}
	if len(fixture.txm.reqs) != 0 || fixture.solver.planningFills.Load() != 0 ||
		fixture.solver.inFlight[fixture.order.Hash] {
		t.Fatal("queued order was submitted or retained after cancellation")
	}
}

func waitForExecutionCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("execution condition was not met")
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
