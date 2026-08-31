package lifi

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/gorilla/websocket"

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

type recoveryGateStrategy struct {
	tokenIn  common.Address
	tokenOut common.Address
}

type testTransactionLaneState struct {
	ready       func() bool
	changes     <-chan struct{}
	onSubscribe func()
	unsubscribe func()
}

func (s *testTransactionLaneState) LaneReady() bool {
	return s.ready()
}

func (s *testTransactionLaneState) SubscribeLaneState() (<-chan struct{}, func()) {
	if s.onSubscribe != nil {
		s.onSubscribe()
	}
	unsubscribe := s.unsubscribe
	if unsubscribe == nil {
		unsubscribe = func() {}
	}
	return s.changes, unsubscribe
}

func alwaysReadyTransactionLane() transactionLaneState {
	return &testTransactionLaneState{ready: func() bool { return true }}
}

type quoteSubmission struct {
	FromAsset string                 `json:"fromAsset"`
	ToAsset   string                 `json:"toAsset"`
	Ranges    []quoteSubmissionRange `json:"ranges"`
	Expiry    int64                  `json:"expiry"`
}

type quoteSubmissionRange struct {
	MaxAmount string `json:"maxAmount"`
}

type quoteSubmissionRequest struct {
	Quotes []quoteSubmission `json:"quotes"`
}

func (s recoveryGateStrategy) DecideQuotes(
	_ context.Context,
	input types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{Quotes: []types.Quote{{
		FromAsset: s.tokenIn, ToAsset: s.tokenOut,
		FromDecimals: 6, ToDecimals: 6,
		Ranges: []types.QuoteRange{{
			MinAmount: big.NewInt(1), MaxAmount: big.NewInt(10), Quote: "1",
		}},
		Expiry: input.QuoteExpiresAt.Unix(), ExclusiveFor: input.Solver,
	}}}, nil
}

func (recoveryGateStrategy) DecideFill(context.Context, types.FillInput) (*types.FillPlan, error) {
	return nil, nil
}

func TestRunGatesQuotesOnRecoveryAndDisconnect(t *testing.T) {
	cfg := testLifiConfig()
	cfg.SolverMode = solverModeExternal
	cfg.QuoteRefreshMode = quoteRefreshModeInterval
	cfg.QuoteInterval = time.Hour
	cfg.QuoteTTL = 2 * time.Hour
	cfg.OrderServer.HTTPTimeout = 5 * time.Second
	adapter := common.HexToAddress("0x9999999999999999999999999999999999999999")
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")

	recoveryStarted := make(chan struct{})
	releaseRecovery := make(chan struct{})
	quoteSubmitted := make(chan struct{}, 1)
	quoteExpired := make(chan struct{}, 1)
	renewalStarted := make(chan struct{}, 1)
	renewalCanceled := make(chan struct{}, 1)
	var recoveryStart sync.Once
	var releaseRecoveryOnce sync.Once
	var quoteRequests atomic.Int32
	var expiryRequests atomic.Int32
	var wallUnix atomic.Int64
	wallUnix.Store(1_700_000_000)
	orderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/solver-api/solver/identities":
			_, _ = fmt.Fprintf(
				w,
				`{"data":[{"id":1,"createdAt":"now","updatedAt":"now","address":%q,"solverId":1}]}`,
				cfg.Executor.Hex(),
			)
		case "/api/v1/solver/supported-contracts":
			_, _ = fmt.Fprintf(
				w,
				`{"data":{"oracle":[],"inputSettler":[{"chain":"eip155:11155111","address":%q}],`+
					`"outputSettler":[{"chain":"eip155:11155111","address":%q}]}}`,
				cfg.InputSettler.Hex(),
				cfg.OutputSettler.Hex(),
			)
		case "/orders":
			recoveryStart.Do(func() { close(recoveryStarted) })
			select {
			case <-r.Context().Done():
				return
			case <-releaseRecovery:
			}
			_, _ = w.Write(testListedOrdersPageJSON(t, nil, 0, 0))
		case "/quotes/submit":
			var request quoteSubmissionRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode quote submission: %v", err)
				return
			}
			if len(request.Quotes) > 0 && request.Quotes[0].Expiry < wallUnix.Load() {
				if expiryRequests.Add(1) == 1 {
					http.Error(w, "temporary expiry failure", http.StatusServiceUnavailable)
					return
				}
				select {
				case quoteExpired <- struct{}{}:
				default:
				}
				_, _ = w.Write([]byte(`{"status":"success","quotesAdded":1}`))
				return
			}
			if quoteRequests.Add(1) == 2 {
				renewalStarted <- struct{}{}
				<-r.Context().Done()
				renewalCanceled <- struct{}{}
				return
			}
			select {
			case quoteSubmitted <- struct{}{}:
			default:
			}
			_, _ = w.Write([]byte(`{"status":"success","quotesAdded":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer orderServer.Close()
	defer releaseRecoveryOnce.Do(func() { close(releaseRecovery) })

	upgrader := websocket.Upgrader{}
	stopWebSocket := make(chan struct{})
	var stopWebSocketOnce sync.Once
	webSocketServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		<-stopWebSocket
	}))
	defer webSocketServer.Close()
	defer stopWebSocketOnce.Do(func() { close(stopWebSocket) })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	solver := &Solver{
		cfg: cfg, chainID: 11155111,
		reader: fakeLifiReader{routes: []route{{
			ID: "route-1", Adapter: adapter, TokenIn: tokenIn, TokenOut: tokenOut,
			TokenInDecimals: 6, TokenOutDecimals: 6,
		}}},
		strategy: recoveryGateStrategy{tokenIn: tokenIn, tokenOut: tokenOut},
		caller:   common.HexToAddress("0x5555555555555555555555555555555555555555"),
		orders:   newOrderClient(orderServer.URL, "test-key", cfg.OrderServer.HTTPTimeout, 11155111),
		feed: newOrderFeed(
			"ws"+strings.TrimPrefix(webSocketServer.URL, "http"),
			"test-key",
			logr.Discard(),
		),
		txm: &fakeLifiTxSender{}, log: logr.Discard(),
		now:          func(context.Context) (time.Time, error) { return time.Unix(1_700_000_000, 0), nil },
		maxFeePerGas: func(context.Context) (*big.Int, error) { return big.NewInt(1), nil },
		wallNow:      func() time.Time { return time.Unix(wallUnix.Load(), 0) },
		txLaneState:  alwaysReadyTransactionLane(),
	}
	done := make(chan error, 1)
	go func() { done <- solver.Run(ctx) }()

	expectSignal(t, recoveryStarted)
	select {
	case <-quoteSubmitted:
		t.Fatal("quote was published before initial recovery completed")
	case <-time.After(100 * time.Millisecond):
	}
	releaseRecoveryOnce.Do(func() { close(releaseRecovery) })
	expectSignal(t, quoteSubmitted)
	wallUnix.Store(1_700_007_200)
	solver.requestQuoteRefresh()
	expectSignal(t, renewalStarted)
	stopWebSocketOnce.Do(func() { close(stopWebSocket) })
	expectSignal(t, renewalCanceled)
	select {
	case <-quoteExpired:
	case <-time.After(5 * time.Second):
		t.Fatal("active quote was not expired after order feed disconnected")
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestQuoteLoopExpiresQuotesOnRootCancellation(t *testing.T) {
	cfg := testLifiConfig()
	cfg.QuoteRefreshMode = quoteRefreshModeInterval
	cfg.QuoteInterval = time.Hour
	cfg.QuoteTTL = 2 * time.Hour
	cfg.OrderServer.HTTPTimeout = time.Second
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	now := time.Unix(1_700_000_000, 0)
	quoteSubmitted := make(chan struct{}, 1)
	quoteExpired := make(chan struct{}, 1)
	orderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quotes/submit" {
			http.NotFound(w, r)
			return
		}
		var request quoteSubmissionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode quote submission: %v", err)
			return
		}
		signal := quoteSubmitted
		if len(request.Quotes) > 0 && request.Quotes[0].Expiry < now.Unix() {
			signal = quoteExpired
		}
		select {
		case signal <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","quotesAdded":1}`))
	}))
	defer orderServer.Close()

	solver := &Solver{
		cfg:      cfg,
		reader:   fakeLifiReader{},
		strategy: recoveryGateStrategy{tokenIn: tokenIn, tokenOut: tokenOut},
		orders:   newOrderClient(orderServer.URL, "test-key", time.Second, 11155111),
		log:      logr.Discard(),
		now:      func(context.Context) (time.Time, error) { return now, nil },
		wallNow:  func() time.Time { return now },
		maxFeePerGas: func(context.Context) (*big.Int, error) {
			return big.NewInt(1), nil
		},
		txLaneState: alwaysReadyTransactionLane(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	connectionCtx, cancelConnection := context.WithCancel(t.Context())
	defer cancelConnection()
	feedConnections := make(chan context.Context, 1)
	feedConnections <- connectionCtx
	done := make(chan error, 1)
	go func() {
		done <- solver.quoteLoop(ctx, nil, make(chan struct{}), feedConnections)
	}()

	expectSignal(t, quoteSubmitted)
	cancel()
	expectSignal(t, quoteExpired)
	if connectionCtx.Err() != nil {
		t.Fatalf("feed connection was canceled before quote expiry: %v", connectionCtx.Err())
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("quoteLoop error = %v, want context cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quoteLoop did not stop after expiring quotes")
	}
}

func TestQuoteLoopSuspendsWhileLaneBusyAndRepublishesOnCoalescedIdle(t *testing.T) {
	cfg := testLifiConfig()
	cfg.QuoteRefreshMode = quoteRefreshModeInterval
	cfg.QuoteInterval = time.Hour
	cfg.QuoteTTL = 2 * time.Hour
	cfg.OrderServer.HTTPTimeout = time.Second
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	now := time.Unix(1_700_000_000, 0)
	quoteSubmitted := make(chan struct{}, 2)
	quoteExpired := make(chan struct{}, 2)
	orderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quotes/submit" {
			http.NotFound(w, r)
			return
		}
		var request quoteSubmissionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode quote submission: %v", err)
			return
		}
		signal := quoteSubmitted
		if len(request.Quotes) > 0 && request.Quotes[0].Expiry < now.Unix() {
			signal = quoteExpired
		}
		select {
		case signal <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","quotesAdded":1}`))
	}))
	defer orderServer.Close()

	var ready atomic.Bool
	ready.Store(false)
	laneStateChanges := make(chan struct{}, 1)
	laneStateSubscribed := make(chan struct{})
	var unsubscribed atomic.Bool
	refresh := make(chan struct{}, 1)
	solver := &Solver{
		cfg:      cfg,
		reader:   fakeLifiReader{},
		strategy: recoveryGateStrategy{tokenIn: tokenIn, tokenOut: tokenOut},
		orders:   newOrderClient(orderServer.URL, "test-key", time.Second, 11155111),
		log:      logr.Discard(),
		now:      func(context.Context) (time.Time, error) { return now, nil },
		wallNow:  func() time.Time { return now },
		maxFeePerGas: func(context.Context) (*big.Int, error) {
			return big.NewInt(1), nil
		},
		txLaneState: &testTransactionLaneState{
			ready:       ready.Load,
			changes:     laneStateChanges,
			onSubscribe: func() { close(laneStateSubscribed) },
			unsubscribe: func() {
				unsubscribed.Store(true)
			},
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	connectionCtx, cancelConnection := context.WithCancel(t.Context())
	defer cancelConnection()
	feedConnections := make(chan context.Context, 1)
	feedConnections <- connectionCtx
	done := make(chan error, 1)
	go func() {
		done <- solver.quoteLoop(ctx, nil, refresh, feedConnections)
	}()

	// A busy initial state must suppress the first connected refresh.
	expectSignal(t, laneStateSubscribed)
	select {
	case <-quoteSubmitted:
		t.Fatal("quote was published while transaction lane was initially not ready")
	case <-time.After(50 * time.Millisecond):
	}
	ready.Store(true)
	laneStateChanges <- struct{}{}
	expectSignal(t, quoteSubmitted)
	ready.Store(false)
	laneStateChanges <- struct{}{}
	expectSignal(t, quoteExpired)
	refresh <- struct{}{}
	select {
	case <-quoteSubmitted:
		t.Fatal("quote was renewed while transaction lane was not ready")
	case <-time.After(50 * time.Millisecond):
	}

	ready.Store(true)
	laneStateChanges <- struct{}{}
	expectSignal(t, quoteSubmitted)

	// A coalesced busy+idle transition leaves one undifferentiated notification with LaneReady already
	// true. The listener must still expire the old curve before publishing the fresh one.
	ready.Store(false)
	ready.Store(true)
	laneStateChanges <- struct{}{}
	expectSignal(t, quoteExpired)
	expectSignal(t, quoteSubmitted)
	cancel()
	expectSignal(t, quoteExpired)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("quoteLoop error = %v, want context cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quoteLoop did not stop after resumed quote expiry")
	}
	if !unsubscribed.Load() {
		t.Fatal("quoteLoop did not unsubscribe from transaction lane state")
	}
}

func TestShutdownPreparationTimeoutIncludesQuoteAndInboxDrain(t *testing.T) {
	solver := &Solver{cfg: &Config{OrderServer: OrderServerConfig{HTTPTimeout: 3 * time.Second}}}
	if got, want := solver.ShutdownPreparationTimeout(), 6*time.Second; got != want {
		t.Fatalf("shutdown preparation timeout = %s, want %s", got, want)
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
	if f.result.Err != nil || f.result.Receipt != nil || f.result.Hash != (common.Hash{}) {
		return f.result
	}
	return txmanager.Result{
		Hash:    common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Receipt: &ethtypes.Receipt{Status: ethtypes.ReceiptStatusSuccessful},
	}
}

type fixedFillStrategy struct {
	plan *types.FillPlan
}

type lifiLifecycleStrategy struct {
	plan          *types.FillPlan
	quoteRoute    route
	quoteCapacity *big.Int
	onInput       func(types.FillInput)
}

func (s *lifiLifecycleStrategy) DecideQuotes(
	_ context.Context,
	input types.QuoteInput,
) (types.QuoteOutput, error) {
	if s.quoteCapacity == nil {
		return types.QuoteOutput{}, nil
	}
	available := liquidlane.CloneBig(s.quoteCapacity)
	if reserved := input.Reservations[s.quoteRoute.CapacityID]; reserved != nil {
		available.Sub(available, reserved)
	}
	if available.Sign() <= 0 {
		return types.QuoteOutput{}, nil
	}
	quote := testStandingQuote(s.quoteRoute, available.Int64())
	quote.Expiry = input.QuoteExpiresAt.Unix()
	quote.ExclusiveFor = input.Solver
	return types.QuoteOutput{Quotes: []types.Quote{quote}}, nil
}

func (s *lifiLifecycleStrategy) DecideFill(_ context.Context, input types.FillInput) (*types.FillPlan, error) {
	if s.onInput != nil {
		s.onInput(input)
	}
	return s.plan, nil
}

type lifiFillTrace struct {
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
	Reconciled  bool
	Resolved    bool
	Refreshes   int
	Failure     bool
	Closed      bool
}

func (s fixedFillStrategy) DecideQuotes(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s fixedFillStrategy) DecideFill(context.Context, types.FillInput) (*types.FillPlan, error) {
	return s.plan, nil
}

type errorFillStrategy struct {
	err error
}

func (errorFillStrategy) DecideQuotes(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s errorFillStrategy) DecideFill(context.Context, types.FillInput) (*types.FillPlan, error) {
	return nil, s.err
}

type terminalNilFillStrategy struct {
	calls int
}

func (*terminalNilFillStrategy) DecideQuotes(
	context.Context,
	types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s *terminalNilFillStrategy) DecideFill(
	context.Context,
	types.FillInput,
) (*types.FillPlan, error) {
	s.calls++
	return nil, nil
}

type reservationAwareFillStrategy struct {
	plan            *types.FillPlan
	blockAtReserved *big.Int
	inputs          chan types.FillInput
	beforeReturn    func(types.FillInput)
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
	if s.beforeReturn != nil {
		s.beforeReturn(input)
	}
	reserved := input.Reservations[s.plan.Routes[0].CapacityID]
	if reserved != nil && (s.blockAtReserved == nil || reserved.Cmp(s.blockAtReserved) >= 0) {
		return nil, nil
	}
	return s.plan, nil
}

func (s reservationAwareFillStrategy) DecideFillWithoutReservations(
	_ context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	input.Reservations = nil
	s.inputs <- input
	if s.beforeReturn != nil {
		s.beforeReturn(input)
	}
	return s.plan, nil
}

type recoveryBarrierRetryStrategy struct {
	plan          *types.FillPlan
	failNextRetry bool
	events        chan string
}

func (*recoveryBarrierRetryStrategy) DecideQuotes(
	context.Context,
	types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s *recoveryBarrierRetryStrategy) DecideFill(
	_ context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	if input.OrderID == "pending" {
		return s.plan, nil
	}
	if reserved := input.Reservations[s.plan.Routes[0].CapacityID]; reserved != nil && reserved.Sign() > 0 {
		s.events <- "blocked"
		return nil, nil
	}
	if s.failNextRetry {
		s.failNextRetry = false
		s.events <- "transient"
		return nil, errors.New("temporary retry failure")
	}
	return s.plan, nil
}

func (s *recoveryBarrierRetryStrategy) DecideFillWithoutReservations(
	context.Context,
	types.FillInput,
) (*types.FillPlan, error) {
	s.events <- "probe"
	return s.plan, nil
}

type reroutingFillStrategy struct {
	plans           map[liquidlane.CapacityID]*types.FillPlan
	blockedCapacity liquidlane.CapacityID
	events          chan string
}

func (*reroutingFillStrategy) DecideQuotes(
	context.Context,
	types.QuoteInput,
) (types.QuoteOutput, error) {
	return types.QuoteOutput{}, nil
}

func (s *reroutingFillStrategy) DecideFill(
	_ context.Context,
	input types.FillInput,
) (*types.FillPlan, error) {
	switch input.OrderID {
	case "pending-a":
		return s.plans["capacity-a"], nil
	case "pending-b":
		return s.plans["capacity-b"], nil
	case "unrelated":
		return s.plans["capacity-c"], nil
	case "blocked":
		for _, capacityID := range []liquidlane.CapacityID{"capacity-a", "capacity-b"} {
			if reserved := input.Reservations[capacityID]; reserved != nil && reserved.Sign() > 0 {
				s.blockedCapacity = capacityID
				s.events <- "blocked-" + string(capacityID)
				return nil, nil
			}
		}
		s.events <- "fill-capacity-b"
		return s.plans["capacity-b"], nil
	default:
		return nil, nil
	}
}

func (s *reroutingFillStrategy) DecideFillWithoutReservations(
	context.Context,
	types.FillInput,
) (*types.FillPlan, error) {
	s.events <- "probe-" + string(s.blockedCapacity)
	return s.plans[s.blockedCapacity], nil
}

func TestFillReservationLifecycleTrace(t *testing.T) {
	for _, mode := range []string{"direct", "signed-discount"} {
		t.Run(mode, func(t *testing.T) {
			for _, outcome := range []string{"success", "error", "cancellation", "closed"} {
				t.Run(outcome, func(t *testing.T) {
					runLifiFillReservationLifecycle(t, mode, outcome)
				})
			}
		})
	}
}

func runLifiFillReservationLifecycle(t *testing.T, mode, outcome string) {
	t.Helper()
	fixture := immediateTestSetup(t)
	routes := testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter)
	capacityID := routes[0].CapacityID
	var discountID *common.Hash
	expected := int64(1_000_000)
	minimum := int64(990_001)
	wantRefreshes := 0
	if mode == "signed-discount" {
		discountID = new(common.HexToHash(testDiscountID))
		expected = 990_090
		minimum = 990_001
		wantRefreshes = 2
	}
	refresh := make(chan struct{}, 4)
	assertLifiRejectedAdmission(t, mode, fixture, routes, discountID, expected, minimum, refresh)

	trace := []lifiFillTrace{{Stage: "rejected", Mode: mode}}
	fillReads := 0
	plan := newLifiLifecyclePlan(routes[0], discountID, expected, minimum)
	strategy := &lifiLifecycleStrategy{
		plan: plan, quoteRoute: routes[0], quoteCapacity: big.NewInt(2_000_000),
	}
	strategy.onInput = func(input types.FillInput) {
		trace = append(trace,
			lifiFillTrace{
				Stage: "facts", Mode: mode, ChainTime: strconv.FormatInt(input.ChainTime.Unix(), 10),
				MaxFee: input.MaxFeePerGas.String(), GasFacts: input.GasPrices != nil, Quotes: len(input.Quotes),
				Capacities: lifiCapacityEntries(input.Reservations),
			},
			lifiFillTrace{
				Stage: "strategy", Mode: mode, RouteID: string(plan.Routes[0].RouteID),
				Adapter: plan.Routes[0].Adapter.Hex(), CapacityID: string(plan.Routes[0].CapacityID),
				DiscountID: hashString(plan.Routes[0].DiscountID), Amount: plan.Routes[0].ReservedAmountOut.String(),
			},
		)
	}
	submitted := make(chan chan<- txmanager.Result, 1)
	txm := &fakeLifiTxSender{hold: true, onSend: func(_ int, result chan<- txmanager.Result) {
		submitted <- result
	}}
	solver := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	solver.quoteRefresh = refresh
	solver.txLaneState = alwaysReadyTransactionLane()
	quoteRequests, stopQuotes := startLifiLifecycleQuoteLoop(t, solver, routes)
	defer stopQuotes()
	assertLifiLifecycleQuoteRequest(
		t, receiveLifiLifecycleQuoteRequest(t, quoteRequests), routes[0], 2_000_000, false,
	)
	configureLifiLifecycleSolver(mode, solver, routes, &fillReads)

	orders := make(chan *submittedOrder, 1)
	orders <- testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	close(orders)
	worker := solver.newOrderWorker(t.Context(), routes, orders, nil, nil)
	done := make(chan error, 1)
	go func() { done <- worker.run() }()
	result := receiveFillSubmission(t, submitted)
	select {
	case err := <-done:
		t.Fatalf("worker returned before terminal result: %v", err)
	default:
	}
	if len(txm.reqs) != 1 {
		t.Fatalf("accepted requests = %d, want 1", len(txm.reqs))
	}
	reducedAmount := 2_000_000 - expected
	assertLifiLifecycleQuoteRequest(
		t, receiveLifiLifecycleQuoteRequest(t, quoteRequests), routes[0], reducedAmount, false,
	)
	if solver.capacity.Len() != 1 {
		t.Fatalf("standing quote changed after reservation release: ledger entries = %d", solver.capacity.Len())
	}

	_, directRoutes, discountRoutes := unpackFinaliseCalldata(t, txm.reqs[0].Data)
	resolved := false
	if mode == "signed-discount" {
		client := solver.discounts.(*fakeDiscountClient)
		resolved = client.listCalls == 2 && client.resolveCalls == 1 && fillReads == 2
		if !resolved {
			t.Fatalf("signed quote/fill terms and final snapshots = %d/%d/%d, want 2/1/2",
				client.listCalls, client.resolveCalls, fillReads)
		}
	}
	trace = append(trace,
		lifiFillTrace{Stage: "terms", Mode: mode, Signed: mode == "signed-discount", Resolved: resolved, Refreshes: fillReads},
		lifiFillTrace{
			Stage: "validated", Mode: mode, RouteID: string(plan.Routes[0].RouteID),
			CandidateID: string(plan.Routes[0].CandidateID), Adapter: plan.Routes[0].Adapter.Hex(),
			CapacityID: string(plan.Routes[0].CapacityID), DiscountID: hashString(plan.Routes[0].DiscountID),
			Amount: plan.Routes[0].ReservedAmountOut.String(),
		},
		lifiFillTrace{
			Stage: "calldata", Mode: mode, Direct: len(directRoutes) == 1,
			Signed: len(discountRoutes) == 1,
		},
		lifiFillTrace{
			Stage: "admitted", Mode: mode, Pending: solver.capacity.Len(),
			Capacities: lifiCapacityEntries(solver.capacity.Snapshot()),
		},
		lifiFillTrace{
			Stage: "quote-reconciled", Mode: mode, Amount: strconv.FormatInt(reducedAmount, 10), Reconciled: true,
		},
	)
	completeLifiLifecycle(t, txm, result, done, outcome)
	assertLifiLifecycleQuoteRequest(
		t, receiveLifiLifecycleQuoteRequest(t, quoteRequests), routes[0], 2_000_000, false,
	)
	trace = append(trace, lifiFillTrace{
		Stage: "terminal", Mode: mode, Outcome: outcome, Pending: solver.capacity.Len(),
		Failure: outcome != "success", Closed: outcome == "closed",
	})

	discount := hashString(discountID)
	candidate := string(liquidlane.NewCandidateID(routes[0], discountID))
	want := []lifiFillTrace{
		{Stage: "rejected", Mode: mode},
		{Stage: "facts", Mode: mode, ChainTime: "1700000000", MaxFee: "1", GasFacts: true, Quotes: 1, Capacities: []string{}},
		{Stage: "strategy", Mode: mode, RouteID: "route-1", Adapter: common.HexToAddress("0xdead").Hex(), CapacityID: "forged-capacity", DiscountID: discount, Amount: big.NewInt(expected).String()},
		{Stage: "terms", Mode: mode, Signed: mode == "signed-discount", Resolved: mode == "signed-discount", Refreshes: wantRefreshes},
		{Stage: "validated", Mode: mode, RouteID: "route-1", CandidateID: candidate, Adapter: fixture.adapter.Hex(), CapacityID: string(capacityID), DiscountID: discount, Amount: big.NewInt(expected).String()},
		{Stage: "calldata", Mode: mode, Direct: mode == "direct", Signed: mode == "signed-discount"},
		{Stage: "admitted", Mode: mode, Pending: 1, Capacities: []string{string(capacityID) + "=" + big.NewInt(expected).String()}},
		{Stage: "quote-reconciled", Mode: mode, Amount: strconv.FormatInt(2_000_000-expected, 10), Reconciled: true},
		{Stage: "terminal", Mode: mode, Outcome: outcome, Failure: outcome != "success", Closed: outcome == "closed"},
	}
	if !reflect.DeepEqual(trace, want) {
		t.Fatalf("lifecycle trace =\n%#v\nwant\n%#v", trace, want)
	}
}

func newLifiLifecyclePlan(
	route route,
	discountID *common.Hash,
	expected, minimum int64,
) *types.FillPlan {
	return &types.FillPlan{Routes: []types.FillRoute{{
		CandidateID: "forged-candidate", RouteID: route.ID,
		CapacityID: "forged-capacity", Adapter: common.HexToAddress("0xdead"),
		AmountIn: big.NewInt(1_000_000), ExpectedAmountOut: big.NewInt(expected),
		MinAmountOut: big.NewInt(minimum), ReservedAmountOut: big.NewInt(expected),
		DiscountID: liquidlane.CloneHash(discountID),
	}}}
}

func configureLifiLifecycleSolver(mode string, solver *Solver, routes []route, fillReads *int) {
	if mode != "signed-discount" {
		return
	}
	now := time.Unix(1_700_000_000, 0)
	baseInventory := liquidlane.DirectInventory(
		routes[0], big.NewInt(2_000_000), big.NewInt(1_000_000_000_000_000_000),
	)
	baseInventory.AdapterMinDiscount = big.NewInt(100_000)
	base := liquidlane.FillQuote{
		Inventory: baseInventory, AmountIn: big.NewInt(1_000_000),
		GrossAmountOut: big.NewInt(1_100_100), MaxAmountOut: big.NewInt(990_090),
		MinDiscount: big.NewInt(100_000),
	}
	solver.discounts = &fakeDiscountClient{
		listed: &discounts.List{Discounts: []discounts.ListItem{
			testDiscountListItem(base.Inventory, 2_000_000, now.Add(time.Minute)),
		}},
		resolved: testResolvedDiscount(base.Inventory, 100_000, now.Add(time.Minute)),
	}
	solver.reader = fakeLifiReader{
		orderID: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		status:  lifiOrderStatusDeposited,
		fillSetFn: func() fillSnapshotSet {
			*fillReads++
			return fillSnapshotSet{Physical: []liquidlane.FillQuote{base}}
		},
	}
}

func assertLifiRejectedAdmission(
	t *testing.T,
	mode string,
	fixture processTestFixture,
	routes []route,
	discountID *common.Hash,
	expected, minimum int64,
	refresh chan struct{},
) {
	t.Helper()
	txm := &fakeLifiTxSender{reject: true}
	solver := newProcessTestSolver(
		fixture.cfg, fixture.caller, txm,
		&lifiLifecycleStrategy{plan: newLifiLifecyclePlan(routes[0], discountID, expected, minimum)},
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	solver.quoteRefresh = refresh
	rejectedFillReads := 0
	configureLifiLifecycleSolver(mode, solver, routes, &rejectedFillReads)
	result := solver.processOrderWithPending(
		t.Context(), routes, testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut), nil,
	)
	if result.fill != nil || solver.capacity.Len() != 0 || len(txm.reqs) != 0 {
		t.Fatalf("rejected admission retained fill/capacity/request: %+v/%d/%d",
			result.fill, solver.capacity.Len(), len(txm.reqs))
	}
	select {
	case <-refresh:
		t.Fatal("rejected admission requested quote reconciliation")
	default:
	}
}

func startLifiLifecycleQuoteLoop(
	t *testing.T,
	solver *Solver,
	routes []route,
) (<-chan quoteSubmissionRequest, func()) {
	t.Helper()
	requests := make(chan quoteSubmissionRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quotes/submit" {
			http.NotFound(w, r)
			return
		}
		var request quoteSubmissionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode lifecycle quote submission: %v", err)
			return
		}
		requests <- request
		ranges := 0
		for _, quote := range request.Quotes {
			ranges += len(quote.Ranges)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","quotesAdded":%d}`, ranges)
	}))
	solver.orders = newOrderClient(server.URL, "test-key", time.Second, 11155111)
	solver.cfg.QuoteRefreshMode = quoteRefreshModeInterval
	solver.cfg.QuoteInterval = time.Hour
	solver.cfg.QuoteTTL = 2 * time.Hour
	solver.cfg.OrderServer.HTTPTimeout = time.Second

	ctx, cancel := context.WithCancel(t.Context())
	connectionCtx, cancelConnection := context.WithCancel(t.Context())
	feedConnections := make(chan context.Context, 1)
	feedConnections <- connectionCtx
	done := make(chan error, 1)
	go func() {
		done <- solver.quoteLoop(ctx, routes, solver.quoteRefresh, feedConnections)
	}()
	return requests, func() {
		cancel()
		request := receiveLifiLifecycleQuoteRequest(t, requests)
		assertLifiLifecycleQuoteRequest(t, request, routes[0], 2_000_000, true)
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("lifecycle quoteLoop error = %v, want context cancellation", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("lifecycle quoteLoop did not stop")
		}
		cancelConnection()
		server.Close()
	}
}

func receiveLifiLifecycleQuoteRequest(
	t *testing.T,
	requests <-chan quoteSubmissionRequest,
) quoteSubmissionRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for lifecycle quote submission")
		return quoteSubmissionRequest{}
	}
}

func assertLifiLifecycleQuoteRequest(
	t *testing.T,
	request quoteSubmissionRequest,
	route route,
	wantMax int64,
	expired bool,
) {
	t.Helper()
	if len(request.Quotes) != 1 || len(request.Quotes[0].Ranges) != 1 {
		t.Fatalf("lifecycle quote request = %#v, want one quote range", request)
	}
	quote := request.Quotes[0]
	if !strings.EqualFold(quote.FromAsset, route.TokenIn.Hex()) ||
		!strings.EqualFold(quote.ToAsset, route.TokenOut.Hex()) ||
		quote.Ranges[0].MaxAmount != strconv.FormatInt(wantMax, 10) {
		t.Fatalf("lifecycle quote request = %#v, want pair %s/%s max %d",
			request, route.TokenIn.Hex(), route.TokenOut.Hex(), wantMax)
	}
	isExpired := quote.Expiry < time.Unix(1_700_000_000, 0).Unix()
	if isExpired != expired {
		t.Fatalf("lifecycle quote expiry = %d, expired = %t, want %t", quote.Expiry, isExpired, expired)
	}
}

func completeLifiLifecycle(
	t *testing.T,
	txm *fakeLifiTxSender,
	result chan<- txmanager.Result,
	done <-chan error,
	outcome string,
) {
	t.Helper()
	switch outcome {
	case "success":
		result <- txm.fillResult()
	case "error":
		result <- txmanager.Result{Err: errors.New("terminal send failure")}
	case "cancellation":
		result <- txmanager.Result{Err: context.Canceled}
	case "closed":
		close(result)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not consume terminal result")
	}
}

func lifiCapacityEntries(reservations liquidlane.CapacityReservations) []string {
	entries := make([]string, 0, len(reservations))
	for capacityID, amount := range reservations {
		entries = append(entries, string(capacityID)+"="+amount.String())
	}
	slices.Sort(entries)
	return entries
}

func hashString(hash *common.Hash) string {
	if hash == nil {
		return ""
	}
	return hash.Hex()
}

func TestBlockedPlanCapacityIDsUsesOnlySelectedRoutes(t *testing.T) {
	reservations := liquidlane.CapacityReservations{
		"capacity-selected": big.NewInt(100),
		"capacity-other":    big.NewInt(200),
	}
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		CapacityID: "capacity-selected",
	}}}

	blocked := blockedPlanCapacityIDs(reservations, plan)
	if len(blocked) != 1 || !blocked["capacity-selected"] || blocked["capacity-other"] {
		t.Fatalf("blocked capacity = %v, want only selected route", blocked)
	}
}

func TestProcessOrderDoesNotProbeExternalNilDecision(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy := &terminalNilFillStrategy{}
	s := newProcessTestSolver(
		fixture.cfg, fixture.caller, &fakeLifiTxSender{}, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
	)
	s.capacity.Set("pending-order", liquidlane.CapacityReservations{
		"capacity-1": big.NewInt(1),
	})

	result := s.processOrderWithPending(
		t.Context(),
		testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
		nil,
	)
	if result.fill != nil || len(result.blockedOn) != 0 {
		t.Fatalf("external nil decision was retained: %+v", result)
	}
	if strategy.calls != 1 {
		t.Fatalf("external fill decisions = %d, want 1", strategy.calls)
	}
}

func TestProcessOrderClassifiesStrategyErrors(t *testing.T) {
	transient := errors.New("strategy transport unavailable")
	tests := []struct {
		name             string
		err              error
		wantRetryable    bool
		wantAttemptLimit int
	}{
		{
			name:             "transient",
			err:              transient,
			wantRetryable:    true,
			wantAttemptLimit: maximumStrategyRecoveryAttempts,
		},
		{
			name: "permanent input rejection",
			err:  types.MarkPermanentFillDecisionError(errors.New("unsupported output context")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := immediateTestSetup(t)
			s := newProcessTestSolver(
				fixture.cfg, fixture.caller, &fakeLifiTxSender{}, errorFillStrategy{err: tt.err},
				fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited,
			)

			result := s.processOrderWithPending(
				t.Context(),
				testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
				testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
				nil,
			)
			if result.retryable != tt.wantRetryable {
				t.Fatalf("retryable = %v, want %v", result.retryable, tt.wantRetryable)
			}
			if result.recoveryAttemptLimit != tt.wantAttemptLimit {
				t.Fatalf(
					"recovery attempt limit = %d, want %d",
					result.recoveryAttemptLimit,
					tt.wantAttemptLimit,
				)
			}
		})
	}
}

func TestProcessOrderSubmitsImmediateFill(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy, fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)
	var logs []string
	s.log = funcr.NewJSON(
		func(entry string) { logs = append(logs, entry) },
		funcr.Options{Verbosity: 1},
	)

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
	if want := time.Unix(1_800_000_000, 0); !txm.reqs[0].CancelAt.Equal(want) {
		t.Fatalf("fill CancelAt = %v, want order deadline %v", txm.reqs[0].CancelAt, want)
	}
	logged := strings.Join(logs, "\n")
	for _, want := range []string{
		"liquidlane fill selected",
		"order fill submitted",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("fill lifecycle log %q missing: %s", want, logged)
		}
	}
	if !strings.Contains(logged, `"orderId":`) ||
		!strings.Contains(logged, `"onChainOrderId":`) ||
		!strings.Contains(logged, `"quoteId":`) {
		t.Fatalf("fill lifecycle logs are missing correlation fields: %s", logged)
	}
}

func TestProcessOrderAttachesOnChainObsolescenceCheck(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	orderID := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	status := lifiOrderStatusDeposited
	var statusErr error
	reader := s.reader.(fakeLifiReader)
	reader.statusForOrderFn = func(got common.Hash) (uint8, error) {
		if got != orderID {
			t.Fatalf("order status ID = %s, want %s", got.Hex(), orderID.Hex())
		}
		return status, statusErr
	}
	s.reader = reader

	s.processOrder(
		t.Context(),
		testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)
	if len(txm.reqs) != 1 || txm.reqs[0].Obsolete == nil {
		t.Fatalf("submitted requests = %d, obsolescence check configured = %v",
			len(txm.reqs), len(txm.reqs) == 1 && txm.reqs[0].Obsolete != nil)
	}
	check := txm.reqs[0].Obsolete

	for _, test := range []struct {
		name         string
		status       uint8
		wantObsolete bool
		wantErr      bool
	}{
		{name: "deposited", status: lifiOrderStatusDeposited},
		{name: "lagging none", status: lifiOrderStatusNone},
		{name: "claimed", status: lifiOrderStatusClaimed, wantObsolete: true},
		{name: "refunded", status: lifiOrderStatusRefunded, wantObsolete: true},
		{name: "unknown", status: 255, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			status = test.status
			obsolete, checkErr := check(t.Context())
			if obsolete != test.wantObsolete || (checkErr != nil) != test.wantErr {
				t.Fatalf("obsolete = %v, error = %v; want obsolete=%v, error=%v",
					obsolete, checkErr, test.wantObsolete, test.wantErr)
			}
		})
	}

	status = lifiOrderStatusDeposited
	statusErr = errors.New("status RPC unavailable")
	obsolete, checkErr := check(t.Context())
	if obsolete || !errors.Is(checkErr, statusErr) {
		t.Fatalf("status error: obsolete = %v, error = %v; want false, wrapped RPC error", obsolete, checkErr)
	}
}

func TestProcessOrderWithoutGasAccountingSkipsFeeReaderAndRequestCap(t *testing.T) {
	fixture := immediateTestSetup(t)
	fixture.cfg.Gas = nil
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	reader := s.reader.(fakeLifiReader)
	reader.omitGasFacts = true
	s.reader = reader
	feeReads := 0
	s.maxFeePerGas = func(context.Context) (*big.Int, error) {
		feeReads++
		return big.NewInt(1), nil
	}

	s.processOrder(
		t.Context(),
		testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)

	if feeReads != 0 {
		t.Fatalf("max fee reads = %d, want 0", feeReads)
	}
	if len(txm.reqs) != 1 {
		t.Fatalf("txmanager.Send calls = %d, want 1", len(txm.reqs))
	}
	if txm.reqs[0].MaxFeePerGas != nil {
		t.Fatalf("fill max fee per gas = %v, want nil", txm.reqs[0].MaxFeePerGas)
	}
}

func TestProcessOrderWithoutDeadlineUsesPendingTimeout(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	order.Order.Expires = 0
	order.Order.FillDeadline = 0

	s.processOrder(
		t.Context(),
		testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		order,
	)

	if len(txm.reqs) != 1 {
		t.Fatalf("submitted fills = %d, want 1", len(txm.reqs))
	}
	if !txm.reqs[0].CancelAt.IsZero() {
		t.Fatalf("fill CancelAt = %v, want global pending timeout", txm.reqs[0].CancelAt)
	}
}

func TestProcessOrderCancellationDeadlineIncludesPreAdmissionLatency(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{}
	s := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	wallNow := time.Unix(1_700_000_000, 0)
	s.wallNow = func() time.Time { return wallNow }
	s.now = func(context.Context) (time.Time, error) {
		return time.Unix(1_700_000_010, 0), nil
	}
	statusReads := 0
	reader := s.reader.(fakeLifiReader)
	reader.statusFn = func() (uint8, error) {
		statusReads++
		if statusReads == 2 {
			wallNow = wallNow.Add(15 * time.Second)
		}
		return lifiOrderStatusDeposited, nil
	}
	s.reader = reader

	s.processOrder(
		t.Context(),
		testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
		testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
	)

	if len(txm.reqs) != 1 {
		t.Fatalf("submitted fills = %d, want 1", len(txm.reqs))
	}
	if want := time.Unix(1_799_999_990, 0); !txm.reqs[0].CancelAt.Equal(want) {
		t.Fatalf("fill CancelAt = %v, want skew-preserving %v", txm.reqs[0].CancelAt, want)
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
	s := newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusClaimed,
	)
	fillReads := 0
	s.reader = fakeLifiReader{
		orderID: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		status:  lifiOrderStatusClaimed,
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

func TestOpenedOrderIDClassifiesOIFStatuses(t *testing.T) {
	fixture := immediateTestSetup(t)
	order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	orderID := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	tests := []struct {
		name    string
		status  uint8
		wantErr error
	}{
		{name: "none", status: lifiOrderStatusNone, wantErr: errOrderDepositNotVisible},
		{name: "deposited", status: lifiOrderStatusDeposited},
		{name: "claimed", status: lifiOrderStatusClaimed, wantErr: errOrderNotFillable},
		{name: "refunded", status: lifiOrderStatusRefunded, wantErr: errOrderNotFillable},
		{name: "unknown", status: 255, wantErr: errOrderNotFillable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			solver := &Solver{
				cfg: fixture.cfg,
				reader: fakeLifiReader{
					orderID: orderID,
					status:  test.status,
				},
				log: logr.Discard(),
			}
			got, err := solver.openedOrderID(t.Context(), order)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("openedOrderID error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && got != orderID {
				t.Fatalf("openedOrderID = %s, want %s", got.Hex(), orderID.Hex())
			}
		})
	}
}

func TestProcessOrderClassifiesSubmissionStatus(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	tests := []struct {
		name                  string
		status                uint8
		wantDepositNotVisible bool
	}{
		{name: "none", status: lifiOrderStatusNone, wantDepositNotVisible: true},
		{name: "claimed", status: lifiOrderStatusClaimed},
		{name: "refunded", status: lifiOrderStatusRefunded},
		{name: "unknown", status: 255},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			txm := &fakeLifiTxSender{}
			solver := newProcessTestSolver(
				fixture.cfg,
				fixture.caller,
				txm,
				strategy,
				fixture.tokenIn,
				fixture.tokenOut,
				fixture.adapter,
				lifiOrderStatusDeposited,
			)
			statusReads := 0
			reader := solver.reader.(fakeLifiReader)
			reader.statusFn = func() (uint8, error) {
				statusReads++
				if statusReads == 1 {
					return lifiOrderStatusDeposited, nil
				}
				return test.status, nil
			}
			solver.reader = reader

			result := solver.processOrderWithPending(
				t.Context(),
				testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
				testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut),
				nil,
			)
			if result.depositNotVisible != test.wantDepositNotVisible {
				t.Fatalf("depositNotVisible = %t, want %t", result.depositNotVisible, test.wantDepositNotVisible)
			}
			if result.fill != nil || result.retryable {
				t.Fatalf("submission status result = %+v, want no fill or generic retry", result)
			}
			if statusReads != 2 || len(txm.reqs) != 0 {
				t.Fatalf("status reads = %d submissions = %d, want 2/0", statusReads, len(txm.reqs))
			}
		})
	}
}

func TestProcessOrderDoesNotRetryFailedSend(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	txm := &fakeLifiTxSender{result: txmanager.Result{Err: errors.New("send failed")}}
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

	if err := s.newOrderWorker(
		context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), orders, nil, nil,
	).run(); err != nil {
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
	for i := range int64(5) {
		order := *base
		order.OrderID = "order-" + big.NewInt(i+1).String()
		order.Order.Nonce = new(big.Int).Add(base.Order.Nonce, big.NewInt(i))
		orders <- &order
	}
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- s.newOrderWorker(
			context.Background(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), orders, nil, nil,
		).run()
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

func TestOrderWorkerRetriesReservationBlockedOrderAfterPartialRelease(t *testing.T) {
	fixture := immediateTestSetup(t)
	inputs := make(chan types.FillInput, 6)
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID:           "route-1",
		CapacityID:        "capacity-1",
		Adapter:           fixture.adapter,
		AmountIn:          big.NewInt(1_000_000),
		ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut:      big.NewInt(990_001),
		ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	submitted := make(chan chan<- txmanager.Result, 3)
	retryTrace := make(chan lifiFillTrace, 1)
	retryRelease := make(chan struct{})
	txm := &fakeLifiTxSender{
		hold: true,
		onSend: func(_ int, result chan<- txmanager.Result) {
			submitted <- result
		},
	}
	var s *Solver
	strategy := reservationAwareFillStrategy{
		plan: plan, blockAtReserved: big.NewInt(2_000_000), inputs: inputs,
	}
	strategy.beforeReturn = func(input types.FillInput) {
		reserved := input.Reservations["capacity-1"]
		if input.OrderID != "order-3" || reserved == nil || reserved.Cmp(big.NewInt(1_000_000)) != 0 {
			return
		}
		global := s.capacity.Snapshot()
		retryTrace <- lifiFillTrace{
			Stage: "retry-planned-before-release", Capacities: lifiCapacityEntries(input.Reservations),
			Amount: global["capacity-1"].String(),
		}
		<-retryRelease
	}
	s = newProcessTestSolver(
		fixture.cfg,
		fixture.caller,
		txm,
		strategy,
		fixture.tokenIn,
		fixture.tokenOut,
		fixture.adapter,
		lifiOrderStatusDeposited,
	)
	s.quoteRefresh = make(chan struct{}, 8)
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
	thirdValue := *first
	thirdValue.OrderID = "order-3"
	thirdValue.Order.Nonce = new(big.Int).Add(first.Order.Nonce, big.NewInt(2))
	orders := make(chan *submittedOrder, 3)
	orders <- first
	orders <- &secondValue
	orders <- &thirdValue
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- s.newOrderWorker(
			t.Context(),
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			nil,
			nil,
		).run()
	}()

	firstInput := receiveFillInput(t, inputs)
	if firstInput.Solver != fixture.cfg.Executor {
		t.Fatalf("solver = %s, want executor %s", firstInput.Solver.Hex(), fixture.cfg.Executor.Hex())
	}
	if len(firstInput.Reservations) != 0 {
		t.Fatalf("first fill reservations = %v, want none", firstInput.Reservations)
	}
	firstResult := receiveFillSubmission(t, submitted)
	secondInput := receiveFillInput(t, inputs)
	if got := secondInput.Reservations["capacity-1"]; got == nil || got.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("second fill reservations = %v, want capacity-1=1000000", secondInput.Reservations)
	}
	secondResult := receiveFillSubmission(t, submitted)
	thirdInput := receiveFillInput(t, inputs)
	if got := thirdInput.Reservations["capacity-1"]; got == nil || got.Cmp(big.NewInt(2_000_000)) != 0 {
		t.Fatalf("third fill reservations = %v, want capacity-1=2000000", thirdInput.Reservations)
	}
	unreservedInput := receiveFillInput(t, inputs)
	if len(unreservedInput.Reservations) != 0 {
		t.Fatalf("reservation probe = %v, want no reservations", unreservedInput.Reservations)
	}
	expectSignal(t, s.quoteRefresh)
	expectSignal(t, s.quoteRefresh)
	firstResult <- txm.fillResult()
	retryInput := receiveFillInput(t, inputs)
	if got := retryInput.Reservations["capacity-1"]; got == nil || got.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("retried fill reservations = %v, want capacity-1=1000000", retryInput.Reservations)
	}
	var replacementTrace []lifiFillTrace
	select {
	case event := <-retryTrace:
		replacementTrace = append(replacementTrace, event)
	case <-time.After(5 * time.Second):
		t.Fatal("retry plan did not capture old reservation before release")
	}
	close(retryRelease)
	thirdResult := receiveFillSubmission(t, submitted)
	if len(txm.reqs) != 3 {
		t.Fatalf("submitted fills = %d, want retry after first of two reservations released", len(txm.reqs))
	}
	expectSignal(t, s.quoteRefresh)
	replacementTrace = append(replacementTrace, lifiFillTrace{
		Stage: "replacement-after-release", Capacities: lifiCapacityEntries(s.capacity.Snapshot()),
	})
	wantReplacementTrace := []lifiFillTrace{
		{Stage: "retry-planned-before-release", Amount: "2000000", Capacities: []string{"capacity-1=1000000"}},
		{Stage: "replacement-after-release", Capacities: []string{"capacity-1=2000000"}},
	}
	if !reflect.DeepEqual(replacementTrace, wantReplacementTrace) {
		t.Fatalf("replacement trace = %#v, want %#v", replacementTrace, wantReplacementTrace)
	}
	select {
	case <-s.quoteRefresh:
		t.Fatal("reservation replacement requested more than one quote refresh")
	case <-time.After(50 * time.Millisecond):
	}
	secondResult <- txm.fillResult()
	thirdResult <- txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after receipt")
	}
}

func TestOrderWorkerRecoveryBarrierRetainsTransientCapacityRetry(t *testing.T) {
	fixture := immediateTestSetup(t)
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID:           "route-1",
		CapacityID:        "capacity-1",
		Adapter:           fixture.adapter,
		AmountIn:          big.NewInt(1_000_000),
		ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut:      big.NewInt(990_001),
		ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	events := make(chan string, 3)
	strategy := &recoveryBarrierRetryStrategy{plan: plan, failNextRetry: true, events: events}
	submitted := make(chan chan<- txmanager.Result, 1)
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
	s.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote {
			return profitableFillSnapshots(fixture.tokenIn, fixture.tokenOut, fixture.adapter, 1_000_000)
		},
	}

	pending := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	pending.OrderID = "pending"
	blockedValue := *pending
	blockedValue.OrderID = "blocked"
	blockedValue.Order.Nonce = new(big.Int).Add(pending.Order.Nonce, big.NewInt(1))
	blockedKey, err := localOrderKey(blockedValue.Order)
	if err != nil {
		t.Fatalf("blocked order key: %v", err)
	}
	blockedValue.dedupeKey = blockedKey
	blocked := &blockedValue
	barrier := &submittedOrder{processed: make(chan struct{})}
	orders := make(chan *submittedOrder)
	barrierDelivered := make(chan struct{})
	go func() {
		orders <- pending
		orders <- blocked
		orders <- barrier
		close(barrierDelivered)
		close(orders)
	}()

	inbox := newOrderInbox(4)
	inbox.beginRecovery()
	defer inbox.endRecovery()
	done := make(chan error, 1)
	go func() {
		done <- s.newOrderWorker(
			t.Context(),
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			inbox.markRecoveryRetry,
			nil,
		).run()
	}()

	pendingResult := receiveFillSubmission(t, submitted)
	expectRetryEvent(t, events, "blocked")
	expectRetryEvent(t, events, "probe")
	expectSignal(t, barrierDelivered)
	select {
	case <-barrier.processed:
		t.Fatal("recovery barrier passed while a recovered order was waiting on capacity")
	case <-time.After(100 * time.Millisecond):
	}

	pendingResult <- txm.fillResult()
	expectRetryEvent(t, events, "transient")
	select {
	case <-barrier.processed:
	case <-time.After(3 * time.Second):
		t.Fatal("recovery barrier did not pass after the capacity retry returned to recovery")
	}
	retries := inbox.takeRecoveryRetries()
	if len(retries) != 1 || retries[0] != blocked {
		t.Fatalf("recovery retries = %+v, want blocked order", retries)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after retaining the capacity retry")
	}
}

func TestOrderWorkerRequeuesReroutedOrderWithoutBlockingNewOrders(t *testing.T) {
	fixture := immediateTestSetup(t)
	capacities := []liquidlane.CapacityID{"capacity-a", "capacity-b", "capacity-c"}
	routes := make([]route, 0, len(capacities))
	quotes := make([]liquidlane.FillQuote, 0, len(capacities))
	plans := make(map[liquidlane.CapacityID]*types.FillPlan, len(capacities))
	for index, capacityID := range capacities {
		routeID := liquidlane.RouteID("route-" + string(rune('a'+index)))
		adapter := common.BigToAddress(big.NewInt(int64(index + 1)))
		routes = append(routes, route{
			ID: routeID, CapacityID: capacityID, Adapter: adapter,
			TokenIn: fixture.tokenIn, TokenOut: fixture.tokenOut,
			TokenInDecimals: 6, TokenOutDecimals: 6,
		})
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{
				Route: liquidlane.Route{
					ID: routeID, CapacityID: capacityID, Adapter: adapter,
					TokenIn: fixture.tokenIn, TokenOut: fixture.tokenOut,
				},
				MaxAssets: big.NewInt(2_000_000),
			},
			AmountIn: big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
		})
		plans[capacityID] = &types.FillPlan{Routes: []types.FillRoute{{
			RouteID: routeID, CapacityID: capacityID, Adapter: adapter,
			AmountIn: big.NewInt(1_000_000), ExpectedAmountOut: big.NewInt(1_000_000),
			MinAmountOut: big.NewInt(990_001), ReservedAmountOut: big.NewInt(1_000_000),
		}}}
	}

	events := make(chan string, 8)
	strategy := &reroutingFillStrategy{plans: plans, events: events}
	submitted := make(chan chan<- txmanager.Result, 4)
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
	s.reader = fakeLifiReader{
		status: lifiOrderStatusDeposited,
		orderIDFn: func(order inputsettler.StandardOrder) common.Hash {
			return common.BigToHash(order.Nonce)
		},
		fillSnapshotsFn: func() []liquidlane.FillQuote { return quotes },
	}

	base := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	orders := make(chan *submittedOrder, 4)
	for index, orderID := range []string{"pending-a", "pending-b", "blocked", "unrelated"} {
		order := *base
		order.OrderID = orderID
		order.Order.Nonce = new(big.Int).Add(base.Order.Nonce, big.NewInt(int64(index)))
		orders <- &order
	}
	close(orders)
	done := make(chan error, 1)
	go func() { done <- s.newOrderWorker(t.Context(), routes, orders, nil, nil).run() }()

	results := []chan<- txmanager.Result{
		receiveFillSubmission(t, submitted),
		receiveFillSubmission(t, submitted),
		receiveFillSubmission(t, submitted),
	}
	expectRetryEvent(t, events, "blocked-capacity-a")
	expectRetryEvent(t, events, "probe-capacity-a")

	results[0] <- txm.fillResult()
	expectRetryEvent(t, events, "blocked-capacity-b")
	expectRetryEvent(t, events, "probe-capacity-b")
	results[1] <- txm.fillResult()
	results = append(results, receiveFillSubmission(t, submitted))
	expectRetryEvent(t, events, "fill-capacity-b")

	results[2] <- txm.fillResult()
	results[3] <- txm.fillResult()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runOrderWorker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after rerouted retry")
	}
}

func TestOrderWorkerDrainsAcceptedFillAfterCancellation(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatalf("New strategy: %v", err)
	}
	submitted := make(chan chan<- txmanager.Result, 1)
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
	ctx, cancel := context.WithCancel(t.Context())
	orders := make(chan *submittedOrder, 1)
	orders <- testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	close(orders)
	done := make(chan error, 1)
	go func() {
		done <- s.newOrderWorker(
			ctx,
			testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
			orders,
			nil,
			nil,
		).run()
	}()

	result := receiveFillSubmission(t, submitted)
	cancel()
	select {
	case err := <-done:
		t.Fatalf("worker returned before accepted fill completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	result <- txm.fillResult()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runOrderWorker error = %v, want context cancellation after drain", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish after draining accepted fill")
	}
	if s.capacity.Len() != 0 {
		t.Fatalf("capacity reservations after drain = %d, want 0", s.capacity.Len())
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

func expectRetryEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("retry event = %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for retry event %q", want)
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
		wallNow:      func() time.Time { return time.Unix(1_700_000_000, 0) },
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
