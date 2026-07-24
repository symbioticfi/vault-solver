package uniswapx

import (
	"context"
	"math/big"
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

	resolved   []liquidlane.Route
	adapters   []common.Address
	fillRoutes []liquidlane.Route
	snapshot   fillSnapshot
}

func (r *executionTestReader) resolveRoutes(
	_ context.Context,
	adapters []common.Address,
) ([]liquidlane.Route, error) {
	r.adapters = append([]common.Address(nil), adapters...)
	return append([]liquidlane.Route(nil), r.resolved...), nil
}

func (r *executionTestReader) validateGasTokens([]liquidlane.Route) error { return nil }

func TestStartFillEncodesResolvedDiscountRoute(t *testing.T) {
	now := time.Unix(1_000, 0)
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
	deadline := now.Add(time.Minute).Unix()
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
				Nonce:    "0x1", Deadline: deadline,
			},
			SignerSignature: "0x01", ProtocolDeadline: deadline, ProtocolSignature: "0x02",
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
		strategy: strategy, txm: &executionTestTxManager{result: make(chan txmanager.Result, 1)},
		discounts: provider, log: logr.Discard(),
		filled: make(map[common.Hash]time.Time), retryAt: make(map[common.Hash]time.Time),
		inFlight: make(map[common.Hash]bool), attempts: make(map[common.Hash]int),
	}
	order := &resolvedOrder{
		Encoded: []byte{1}, Signature: []byte{2}, Hash: common.HexToHash("0x1"), Source: orderSourcePublicV2,
		Executor: solver.cfg.Executor, TokenIn: route.TokenIn, TokenOut: route.TokenOut,
		AmountIn: big.NewInt(100), AmountOut: big.NewInt(90), Deadline: uint32(now.Add(time.Minute).Unix()),
	}
	if _, err := solver.startFill(t.Context(), []liquidlane.Route{configuredRoute}, order, now); err != nil {
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
	discountRoute := packed.DiscountRoutes[0]
	if discountRoute.Adapter != route.Adapter ||
		discountRoute.AmountIn.Cmp(big.NewInt(100)) != 0 ||
		discountRoute.DiscountSwap.Discount.TokenToRedeem != route.TokenIn {
		t.Fatalf("packed discount route = %+v", discountRoute)
	}
}

func hashPointer(hash common.Hash) *common.Hash { return &hash }

func (r *executionTestReader) fillSnapshot(
	_ context.Context,
	routes []liquidlane.Route,
	_ common.Address,
	_ common.Address,
	_ *big.Int,
	_ time.Time,
) (fillSnapshot, error) {
	r.fillRoutes = append([]liquidlane.Route(nil), routes...)
	return r.snapshot, nil
}

func TestStartFillRejectsExpiredOrderBeforeStrategy(t *testing.T) {
	now := time.Unix(1_000, 0)
	strategy := &executionTestStrategy{}
	solver := &Solver{strategy: strategy}
	order := &resolvedOrder{
		TokenOut: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Deadline: uint32(now.Unix()),
	}

	if _, err := solver.startFill(t.Context(), nil, order, now); !errors.Is(err, errOrderNotFillable) {
		t.Fatalf("startFill error = %v, want %v", err, errOrderNotFillable)
	}
	if strategy.input.OrderID != "" {
		t.Fatal("expired order reached strategy")
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
	result chan txmanager.Result
	sent   int
	reqs   []txmanager.Request
}

func (m *executionTestTxManager) MaxFeePerGas(context.Context) (*big.Int, error) {
	return new(big.Int), nil
}

func (m *executionTestTxManager) SendAsync(
	_ context.Context,
	request txmanager.Request,
) (<-chan txmanager.Result, bool) {
	m.sent++
	m.reqs = append(m.reqs, request)
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

func TestStartFillSubmitsAsynchronouslyAndReservesCapacity(t *testing.T) {
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
	var packed uxexecutor.ILiquidLaneUniswapXExecutorFillCall
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
			packed = *abi.ConvertType(
				values[1], new(uxexecutor.ILiquidLaneUniswapXExecutorFillCall),
			).(*uxexecutor.ILiquidLaneUniswapXExecutorFillCall)
			return nil, nil
		}),
		reader: &executionTestReader{snapshot: fillSnapshot{Direct: []liquidlane.FillQuote{{
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
	pending, err := solver.startFill(t.Context(), []liquidlane.Route{route}, order, now)
	if err != nil {
		t.Fatalf("startFill: %v", err)
	}
	if pending == nil || txm.sent != 1 {
		t.Fatalf("pending/sent = %v/%d", pending, txm.sent)
	}
	if txm.reqs[0].Confirmations != nil {
		t.Fatalf("fill confirmations override = %d, want global txmanager configuration", *txm.reqs[0].Confirmations)
	}
	if len(packed.Routes) != 1 || packed.Routes[0].AmountOut.Cmp(big.NewInt(90)) != 0 ||
		len(packed.DiscountRoutes) != 0 {
		t.Fatalf("packed direct fill call = %+v", packed)
	}
	if got := solver.capacity.Snapshot()[route.CapacityID]; got == nil || got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("pending reservation = %v", got)
	}
	if reservations := strategy.input.Reservations; len(reservations) != 0 {
		t.Fatalf("unexpected pre-existing reservations: %v", reservations)
	}
	txm.result <- txmanager.Result{Hash: common.HexToHash("0x2")}
	result := <-pending.result
	solver.completePendingFill(uniswapFillCompletion{fill: pending, result: result})
	if solver.capacity.Len() != 0 {
		t.Fatal("pending reservation was not released")
	}
	if _, pending := solver.exclusiveUntil[order.Hash]; !pending {
		t.Fatal("successful tx cleared exclusive obligation before its canonical block time was reconciled")
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
