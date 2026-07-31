package lifi

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestRunLogsExternalAdapterAuthorizationFailure(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	var logs []string
	s := &Solver{
		cfg: &Config{
			SolverMode: solverModeExternal,
			Adapters:   []common.Address{adapter},
			Executor:   executor,
		},
		reader: fakeLifiReader{
			routes:        []route{{Adapter: adapter}},
			directAuthErr: errors.New("executor is not an authorized filler"),
		},
		log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}

	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "validate direct authorization") {
		t.Fatalf("Run() error = %v", err)
	}
	logged := strings.Join(logs, "\n")
	if !strings.Contains(logged, "external adapter authorization failed") ||
		!strings.Contains(logged, "executor is not an authorized filler") ||
		!strings.Contains(logged, executor.Hex()) ||
		!strings.Contains(logged, `"error"`) {
		t.Fatalf("authorization failure was not logged as an error: %s", logged)
	}
}

func TestRunRejectsNonZeroGovernanceFee(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	s := &Solver{
		cfg: &Config{Adapters: []common.Address{adapter}},
		reader: fakeLifiReader{
			routes:           []route{{Adapter: adapter}},
			governanceFeeErr: errors.New("input settler governance fee is 1, expected zero"),
		},
		log: logr.Discard(),
	}

	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "validate governance fee") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunLogsExecutorValidationFailure(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	caller := common.HexToAddress("0x3333333333333333333333333333333333333333")
	var logs []string
	s := &Solver{
		cfg:    &Config{Adapters: []common.Address{adapter}, Executor: executor},
		caller: caller,
		reader: fakeLifiReader{
			routes:      []route{{Adapter: adapter}},
			executorErr: errors.New("caller is not authorized"),
		},
		log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}

	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "lifi: validate executor: caller is not authorized") {
		t.Fatalf("Run() error = %v", err)
	}
	logged := strings.Join(logs, "\n")
	if !strings.Contains(logged, "executor validation failed") ||
		!strings.Contains(logged, "caller is not authorized") ||
		!strings.Contains(logged, executor.Hex()) ||
		!strings.Contains(logged, caller.Hex()) ||
		!strings.Contains(logged, `"error"`) {
		t.Fatalf("executor failure was not logged with its reason: %s", logged)
	}
}

func (s *Solver) processOrder(ctx context.Context, routes []route, order *submittedOrder) {
	s.processOrderWithPending(ctx, routes, order, nil)
}

type fakeLifiTxSender struct {
	reqs    []txmanager.Request
	results []chan txmanager.Result
	result  txmanager.Result
	reject  bool
	hold    bool
	onSend  func(int, chan<- txmanager.Result)
}

func (f *fakeLifiTxSender) SendAsync(
	_ context.Context,
	req txmanager.Request,
) (<-chan txmanager.Result, bool) {
	if f.reject {
		return nil, false
	}
	f.reqs = append(f.reqs, req)
	result := make(chan txmanager.Result, 1)
	f.results = append(f.results, result)
	if f.onSend != nil {
		f.onSend(len(f.reqs), result)
	}
	if !f.hold {
		result <- f.fillResult()
	}
	return result, true
}

func (f *fakeLifiTxSender) fillResult() txmanager.Result {
	if f.result.Outcome != "" || f.result.Err != nil ||
		f.result.Receipt != nil || f.result.Hash != (common.Hash{}) {
		return f.result
	}
	return txmanager.Result{
		Hash:    common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Receipt: &ethtypes.Receipt{Status: ethtypes.ReceiptStatusSuccessful},
		Outcome: txmanager.OutcomeConfirmed,
	}
}

type fixedFillStrategy struct {
	plan *types.FillPlan
}

func (s fixedFillStrategy) DecideQuotes(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s fixedFillStrategy) DecideFill(context.Context, types.FillInput) (*types.FillPlan, error) {
	return s.plan, nil
}

type reservationAwareFillStrategy struct {
	plan   *types.FillPlan
	inputs chan types.FillInput
}

func (s reservationAwareFillStrategy) DecideQuotes(
	context.Context,
	types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s reservationAwareFillStrategy) DecideFill(
	_ context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	s.inputs <- input
	if len(input.Reservations) != 0 {
		return nil, nil
	}
	return s.plan, nil
}

func TestProcessOrderSubmitsImmediateFill(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy, fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)

	s.processOrder(context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut))
	if len(txm.reqs) != 1 {
		t.Fatalf("txmanager.Send calls = %d, want 1", len(txm.reqs))
	}
	if txm.reqs[0].To != fixture.cfg.Executor || txm.reqs[0].Label != "lifi-fill" || len(txm.reqs[0].Data) == 0 {
		t.Fatalf("bad fill request: %+v", txm.reqs[0])
	}
	if txm.reqs[0].MaxFeePerGas == nil || txm.reqs[0].MaxFeePerGas.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("fill max fee per gas = %v, want 1", txm.reqs[0].MaxFeePerGas)
	}
}

func TestProcessOrderSkipsInputTokenOutsideScopeBeforeChainReads(t *testing.T) {
	fixture := immediateTestSetup(t)
	otherToken := common.HexToAddress("0x9999999999999999999999999999999999999999")
	fixture.cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, otherToken)
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, fixedFillStrategy{},
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	orderIDReads := 0
	s.reader = fakeLifiReader{orderIDFn: func(inputsettler.StandardOrder) common.Hash {
		orderIDReads++
		return common.Hash{}
	}}

	s.processOrder(
		context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)
	if orderIDReads != 0 || len(txm.reqs) != 0 {
		t.Fatalf("out-of-scope order: orderID reads=%d txs=%d", orderIDReads, len(txm.reqs))
	}
}

func TestProcessOrderSkipsWhenGovernanceFeeInvariantFails(t *testing.T) {
	fixture := immediateTestSetup(t)
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, fixedFillStrategy{},
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	orderIDReads := 0
	s.reader = fakeLifiReader{
		governanceFeeErr: errors.New("input settler governance fee is 1, expected zero"),
		orderIDFn: func(inputsettler.StandardOrder) common.Hash {
			orderIDReads++
			return common.Hash{}
		},
	}
	var logs []string
	s.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})

	s.processOrder(
		context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)
	if orderIDReads != 0 || len(txm.reqs) != 0 {
		t.Fatalf("fee-bearing order: orderID reads=%d txs=%d", orderIDReads, len(txm.reqs))
	}
	logged := strings.Join(logs, "\n")
	if !strings.Contains(logged, "governance fee invariant failed") || !strings.Contains(logged, `"error"`) {
		t.Fatalf("governance fee failure was not logged as an error: %s", logged)
	}
}

func TestProcessOrderFillsThroughPrivateDiscountWithoutDirectAuthorization(t *testing.T) {
	fixture := immediateTestSetup(t)
	now := time.Unix(1_700_000_000, 0)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	routeItem := testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter)[0]
	baseInventory := liquidlane.DirectInventory(
		routeItem,
		big.NewInt(2_000_000),
		big.NewInt(1_000_000_000_000_000_000),
	)
	baseInventory.AdapterMinDiscount = big.NewInt(100_000)
	base := liquidlane.FillQuote{
		Inventory: baseInventory, AmountIn: big.NewInt(1_000_000), GrossAmountOut: big.NewInt(1_100_100),
		MaxAmountOut: big.NewInt(990_090), MinDiscount: big.NewInt(100_000),
	}
	discounts := &fakeDiscountClient{
		listed: &discounts.List{Discounts: []discounts.ListItem{
			testDiscountListItem(base.Inventory, 2_000_000, now.Add(time.Minute)),
		}},
		resolved: testResolvedDiscount(base.Inventory, 100_000, now.Add(time.Minute)),
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	s.discounts = discounts
	fillReads := 0
	s.reader = fakeLifiReader{
		orderID: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		status:  lifiOrderStatusDeposited,
		fillSetFn: func() fillSnapshotSet {
			fillReads++
			return fillSnapshotSet{Physical: []liquidlane.FillQuote{base}}
		},
	}
	s.now = func(context.Context) (time.Time, error) { return now, nil }

	s.processOrder(
		context.Background(), []route{routeItem},
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)
	if len(txm.reqs) != 1 || discounts.listCalls != 1 || discounts.resolveCalls != 1 || fillReads != 2 {
		t.Fatalf(
			"txs=%d discount calls=%d/%d fill reads=%d",
			len(txm.reqs), discounts.listCalls, discounts.resolveCalls, fillReads,
		)
	}
}

func TestProcessOrderRejectsMultiRoutePlanForPermissionedToken(t *testing.T) {
	fixture := immediateTestSetup(t)
	fixture.cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, fixture.tokenIn)
	txm := &fakeLifiTxSender{}
	plan := &types.FillPlan{Routes: []types.FillRoute{
		{RouteID: "route-1", Adapter: fixture.adapter, AmountIn: big.NewInt(500_000)},
		{RouteID: "route-2", Adapter: fixture.adapter, AmountIn: big.NewInt(500_000)},
	}}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, fixedFillStrategy{plan: plan},
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)

	s.processOrder(
		context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)
	if len(txm.reqs) != 0 {
		t.Fatalf("permissioned multi-route plan submitted %d transactions", len(txm.reqs))
	}
}

func TestRoutesForPairUsesBothTokens(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	other := common.HexToAddress("0x3333333333333333333333333333333333333333")
	routes := []route{
		{ID: "exact", TokenIn: tokenIn, TokenOut: tokenOut},
		{ID: "wrong-output", TokenIn: tokenIn, TokenOut: other},
		{ID: "wrong-input", TokenIn: other, TokenOut: tokenOut},
	}

	got := routesForPair(routes, tokenIn, tokenOut)
	if len(got) != 1 || got[0].ID != "exact" {
		t.Fatalf("routes = %+v", got)
	}
}

func TestProcessOrderChecksOnChainStatusBeforeSend(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy, fixture.tokenIn, fixture.tokenOut, fixture.adapter, 2)
	fillReads := 0
	s.reader = fakeLifiReader{
		orderID: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		status:  2,
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			fillReads++
			return profitableFillSnapshots(fixture.tokenIn, fixture.tokenOut, fixture.adapter, 1_000_000)
		},
	}

	s.processOrder(context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut))
	if len(txm.reqs) != 0 || fillReads != 0 {
		t.Fatalf("closed order txs = %d fillReads = %d", len(txm.reqs), fillReads)
	}
}

func TestProcessOrderDoesNotRetryFailedSend(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{result: txmanager.Result{
		Outcome: txmanager.OutcomeSubmissionError,
		Err:     errors.New("send failed"),
	}}
	s := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy, fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)

	s.processOrder(context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut))
	if len(txm.reqs) != 1 {
		t.Fatalf("failed send attempts = %d, want 1", len(txm.reqs))
	}
}

func TestProcessOrderDropsWhenTransactionSubmissionIsRejected(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{reject: true}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)

	s.processOrder(
		context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)
	if len(txm.reqs) != 0 {
		t.Fatalf("busy sender accepted %d requests", len(txm.reqs))
	}
}

func TestOrderWorkerReplansQueuedOrderBeforeSend(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	availableOutput := int64(1_000_000)
	maxFeePerGas := big.NewInt(1)
	txm := &fakeLifiTxSender{onSend: func(attempt int, _ chan<- txmanager.Result) {
		if attempt == 1 {
			availableOutput = 980_000
			maxFeePerGas = big.NewInt(2)
		}
	}}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	fillReads := 0
	feeReads := 0
	s.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			fillReads++
			return profitableFillSnapshots(
				fixture.tokenIn, fixture.tokenOut, fixture.adapter, availableOutput,
			)
		},
	}
	s.maxFeePerGas = func(context.Context) (*big.Int, error) {
		feeReads++
		return new(big.Int).Set(maxFeePerGas), nil
	}
	first := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	secondValue := *first
	secondValue.OrderID = "order-2"
	secondValue.OnChainOrderID = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	secondValue.Order.Nonce = new(big.Int).Add(first.Order.Nonce, big.NewInt(1))
	orders := make(chan *submittedOrder, 2)
	orders <- first
	orders <- &secondValue
	close(orders)

	if err := s.runOrderWorker(
		context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), orders,
	); err != nil {
		t.Fatalf("runOrderWorker: %v", err)
	}
	if fillReads != 2 || feeReads != 2 {
		t.Fatalf("fresh state reads: fills=%d fees=%d, want 2/2", fillReads, feeReads)
	}
	if len(txm.reqs) != 1 {
		t.Fatalf("fill attempts = %d, want 1 after second order becomes unprofitable", len(txm.reqs))
	}
}

func TestOrderWorkerSubmitsAllFillsWithoutWaitingForReceipts(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	submitted := make(chan chan<- txmanager.Result, 5)
	txm := &fakeLifiTxSender{
		hold: true,
		onSend: func(_ int, result chan<- txmanager.Result) {
			submitted <- result
		},
	}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	fillReads := 0
	feeReads := 0
	s.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			fillReads++
			fills := profitableFillSnapshots(fixture.tokenIn, fixture.tokenOut, fixture.adapter, 1_000_000)
			fills[0].MaxAssets = big.NewInt(10_000_000)
			return fills
		},
	}
	s.maxFeePerGas = func(context.Context) (*big.Int, error) {
		feeReads++
		return big.NewInt(1), nil
	}

	base := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	orders := make(chan *submittedOrder, 5)
	for i := int64(0); i < 5; i++ {
		order := *base
		order.OrderID = "order-" + big.NewInt(i+1).String()
		order.Order.Nonce = new(big.Int).Add(base.Order.Nonce, big.NewInt(i))
		orders <- &order
	}
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- s.runOrderWorker(
			context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), orders,
		)
	}()

	results := make([]chan<- txmanager.Result, 0, 5)
	for range 5 {
		results = append(results, receiveFillSubmission(t, submitted))
	}
	for _, result := range results {
		result <- txm.fillResult()
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after receipts")
	}
	if fillReads != 5 || feeReads != 5 || len(txm.reqs) != 5 {
		t.Fatalf("fills=%d fees=%d submissions=%d, want 5/5/5", fillReads, feeReads, len(txm.reqs))
	}
	for i, req := range txm.reqs {
		if req.Confirmations != nil {
			t.Fatalf("request %d confirmations = %v, want global txmanager configuration", i, req.Confirmations)
		}
	}
}

func TestOrderWorkerPassesPendingReservationsToNextFillDecision(t *testing.T) {
	fixture := immediateTestSetup(t)
	inputs := make(chan types.FillInput, 2)
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID:           "route-1",
		CapacityID:        "capacity-1",
		Adapter:           fixture.adapter,
		AmountIn:          big.NewInt(1_000_000),
		ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut:      big.NewInt(990_001),
		ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	txm := &fakeLifiTxSender{hold: true}
	s := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		reservationAwareFillStrategy{plan: plan, inputs: inputs},
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	s.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			return profitableFillSnapshots(fixture.tokenIn, fixture.tokenOut, fixture.adapter, 1_000_000)
		},
	}

	first := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	secondValue := *first
	secondValue.OrderID = "order-2"
	secondValue.Order.Nonce = new(big.Int).Add(first.Order.Nonce, big.NewInt(1))
	orders := make(chan *submittedOrder, 2)
	orders <- first
	orders <- &secondValue
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- s.runOrderWorker(
			t.Context(),
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
		)
	}()

	firstInput := receiveFillInput(t, inputs)
	if firstInput.Solver != fixture.cfg.Executor {
		t.Fatalf("solver = %s, want executor %s", firstInput.Solver.Hex(), fixture.cfg.Executor.Hex())
	}
	if len(firstInput.Reservations) != 0 {
		t.Fatalf("first fill reservations = %v, want none", firstInput.Reservations)
	}
	secondInput := receiveFillInput(t, inputs)
	if got := secondInput.Reservations["capacity-1"]; got == nil || got.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("second fill reservations = %v, want capacity-1=1000000", secondInput.Reservations)
	}
	if len(txm.reqs) != 1 {
		t.Fatalf("submitted fills = %d, want 1", len(txm.reqs))
	}
	txm.results[0] <- txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after receipt")
	}
}

func receiveFillInput(t *testing.T, inputs <-chan types.FillInput) types.FillInput {
	t.Helper()
	select {
	case input := <-inputs:
		return input
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fill decision")
		return types.FillInput{}
	}
}

func receiveFillSubmission(t *testing.T, submitted <-chan chan<- txmanager.Result) chan<- txmanager.Result {
	t.Helper()
	select {
	case result := <-submitted:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fill submission")
		return nil
	}
}

type processTestFixture struct {
	cfg      *Config
	caller   common.Address
	tokenIn  common.Address
	tokenOut common.Address
	adapter  common.Address
}

func immediateTestSetup(t *testing.T) processTestFixture {
	t.Helper()
	cfg := testLifiConfig()
	caller := common.HexToAddress("0x5555555555555555555555555555555555555555")
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	adapter := common.HexToAddress("0x9999999999999999999999999999999999999999")
	return processTestFixture{cfg: cfg, caller: caller, tokenIn: tokenIn, tokenOut: tokenOut, adapter: adapter}
}

func newProcessTestSolver(
	cfg *Config,
	caller common.Address,
	txm *fakeLifiTxSender,
	strategy types.Strategy,
	tokenIn, tokenOut, adapter common.Address,
	status uint8,
) *Solver {
	return &Solver{
		cfg: cfg, chainID: 11155111,
		reader: fakeLifiReader{
			orderID: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			status:  status, fill: profitableFillSnapshots(tokenIn, tokenOut, adapter, 1_000_000),
		},
		strategy: strategy, caller: caller, txm: txm, log: logr.Discard(),
		now:          func(context.Context) (time.Time, error) { return time.Unix(1_700_000_000, 0), nil },
		maxFeePerGas: func(context.Context) (*big.Int, error) { return big.NewInt(1), nil },
	}
}

func profitableFillSnapshots(tokenIn, tokenOut, adapter common.Address, amountOut int64) []liquidlane.FillQuote {
	return []liquidlane.FillQuote{{
		Inventory: liquidlane.Inventory{
			Route: liquidlane.Route{
				ID: "route-1", CapacityID: "capacity-1", Adapter: adapter, TokenIn: tokenIn, TokenOut: tokenOut,
			},
			MaxAssets: big.NewInt(2_000_000),
		},
		AmountIn:     big.NewInt(1_000_000),
		MaxAmountOut: big.NewInt(amountOut),
	}}
}

func testResolvedRoutes(tokenIn, tokenOut, adapter common.Address) []route {
	return []route{{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter, TokenIn: tokenIn, TokenOut: tokenOut,
		TokenInDecimals: 6, TokenOutDecimals: 6,
	}}
}

func testSubmittedOrder(t *testing.T, cfg *Config, tokenIn, tokenOut common.Address) *submittedOrder {
	t.Helper()
	order, err := parseSubmittedOrder(testOrderJSON(t, cfg, tokenIn, tokenOut), cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	return order
}
