package uniswapx

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type orderPollerFunc func(context.Context, int64, *common.Address) ([]orderEntry, error)

func (f orderPollerFunc) openOrders(ctx context.Context, chainID int64, filler *common.Address) ([]orderEntry, error) {
	return f(ctx, chainID, filler)
}

func (f orderPollerFunc) recentOrders(
	context.Context,
	int64,
	common.Address,
	time.Time,
) ([]orderEntry, error) {
	return nil, nil
}

func (f orderPollerFunc) ordersByHash(
	context.Context,
	int64,
	[]common.Hash,
) (map[common.Hash]orderTerminal, error) {
	return nil, errors.New("terminal lookup is not configured")
}

type countingChainReader struct {
	chainReader

	latestCalls int
	now         time.Time
}

type startupChainReader struct {
	chainReader

	routes       []liquidlane.Route
	executorErr  error
	adapterErr   error
	callerErr    error
	unauthorized []common.Address
	executor     common.Address
	caller       common.Address

	authorizationCalls int
}

func (r *startupChainReader) resolveRoutes(context.Context, []common.Address) ([]liquidlane.Route, error) {
	return r.routes, nil
}

func (r *startupChainReader) validateExecutorCode(
	_ context.Context,
	executor common.Address,
) error {
	r.executor = executor
	return r.executorErr
}

func (r *startupChainReader) validateExecutorCaller(
	_ context.Context,
	executor, caller common.Address,
) error {
	r.executor = executor
	r.caller = caller
	return r.callerErr
}

func (r *startupChainReader) unauthorizedAdapters(
	_ context.Context,
	executor common.Address,
	_ []liquidlane.Route,
) ([]common.Address, error) {
	r.authorizationCalls++
	r.executor = executor
	return r.unauthorized, r.adapterErr
}

func (r *startupChainReader) validateGasTokens([]liquidlane.Route) error { return nil }

func TestRunLogsStartupValidationFailures(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")

	tests := []struct {
		name         string
		executorErr  error
		callerErr    error
		adapterErr   error
		unauthorized []common.Address
		wantError    string
		wantMessage  string
	}{
		{
			name:        "executor",
			executorErr: errors.New("executor has no bytecode"),
			wantError:   "validate executor: executor has no bytecode", wantMessage: "executor validation failed",
		},
		{
			name:        "caller",
			callerErr:   errors.New("caller is not authorized"),
			wantError:   "validate executor caller: caller is not authorized",
			wantMessage: "executor caller validation failed",
		},
		{
			name:         "adapter",
			unauthorized: []common.Address{adapter},
			wantError:    "is not authorized as direct filler", wantMessage: "adapter validation failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs []string
			reader := &startupChainReader{
				routes: []liquidlane.Route{{Adapter: adapter}}, executorErr: tc.executorErr,
				callerErr: tc.callerErr, adapterErr: tc.adapterErr, unauthorized: tc.unauthorized,
			}
			s := &Solver{
				cfg: &Config{
					Executor: executor, Adapters: []common.Address{adapter},
					SolverMode: solverModeExternal,
				},
				solverAddress: common.HexToAddress("0x3333333333333333333333333333333333333333"),
				reader:        reader,
				log:           funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
			}

			err := s.Run(t.Context())
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("Run() error = %v, want %q", err, tc.wantError)
			}
			logged := strings.Join(logs, "\n")
			if !strings.Contains(logged, tc.wantMessage) ||
				!strings.Contains(logged, tc.wantError) ||
				!strings.Contains(logged, executor.Hex()) ||
				!strings.Contains(logged, `"error"`) {
				t.Fatalf("startup failure was not logged with its reason: %s", logged)
			}
			if reader.executor != executor {
				t.Fatalf("executor validation address = %s, want %s", reader.executor, executor)
			}
			if tc.callerErr != nil && reader.caller != s.solverAddress {
				t.Fatalf("caller validation address = %s, want %s", reader.caller, s.solverAddress)
			}
			if tc.callerErr != nil && !strings.Contains(logged, s.solverAddress.Hex()) {
				t.Fatalf("caller validation log omitted caller %s: %s", s.solverAddress, logged)
			}
		})
	}
}

func TestRunInternalModeAllowsNoAdaptersAndSkipsDirectAuthorizationGate(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	stop := errors.New("stop after startup authorization")
	reader := &startupChainReader{
		adapterErr: errors.New("direct authorization is unavailable"),
	}
	solver := &Solver{
		cfg: &Config{
			Executor: executor, SolverMode: solverModeInternal, Discounts: &DiscountConfig{},
		},
		solverAddress: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		reader:        reader,
		orders: orderPollerFunc(func(context.Context, int64, *common.Address) ([]orderEntry, error) {
			return nil, stop
		}),
		log: logr.Discard(),
	}

	err := solver.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), stop.Error()) {
		t.Fatalf("Run() error = %v, want later order-service failure", err)
	}
	if reader.authorizationCalls != 0 {
		t.Fatalf("direct authorization calls = %d, want 0 in internal mode", reader.authorizationCalls)
	}
}

func (r *countingChainReader) latestBlockTime(context.Context) (time.Time, error) {
	r.latestCalls++
	if !r.now.IsZero() {
		return r.now, nil
	}
	return time.Unix(1_000, 0), nil
}

func TestPollOrdersProcessesExclusiveBeforePublicFailure(t *testing.T) {
	reader := &countingChainReader{}
	executor := common.HexToAddress("0x1111111111111111111111111111111111111111")
	solver := &Solver{
		cfg: &Config{
			Executor:    executor,
			OrderServer: OrderServerConfig{Sources: OrderSourcesConfig{ExclusiveV2: true, PublicV2: true}},
		},
		chainID: 1,
		reader:  reader,
		orders: orderPollerFunc(func(_ context.Context, _ int64, filler *common.Address) ([]orderEntry, error) {
			if filler != nil {
				return []orderEntry{{OrderHash: "exclusive-was-processed"}}, nil
			}
			return nil, errors.New("public unavailable")
		}),
		log:     logr.Discard(),
		filled:  make(map[common.Hash]time.Time),
		retryAt: make(map[common.Hash]time.Time),
	}
	err := solver.pollOrders(t.Context(), make(chan *resolvedOrder, 1))
	if err == nil || !strings.Contains(err.Error(), "poll public-v2 orders: public unavailable") {
		t.Fatalf("pollOrders() error = %v", err)
	}
	if reader.latestCalls != 1 {
		t.Fatalf("exclusive latestBlockTime calls = %d, want 1", reader.latestCalls)
	}
}

func TestPollOrdersKeepsUnknownExclusivePendingAndStopsQuotes(t *testing.T) {
	hash := common.HexToHash("0x1234")
	now := time.Unix(1_000, 0)
	cfg := &Config{
		OrderServer: OrderServerConfig{PollInterval: time.Second},
		Breaker:     BreakerConfig{Window: time.Minute},
	}
	solver := newPollingTestSolver(
		cfg,
		&stateTestOrderPoller{terminals: map[common.Hash]orderTerminal{}},
	)
	solver.trackExclusive(&resolvedOrder{
		Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(now.Add(-time.Second).Unix()),
	}, now.Add(-2*time.Second))
	solver.quoteState.Store(&quoteState{expiresAt: now.Add(time.Minute)})

	err := solver.pollOrders(t.Context(), make(chan *resolvedOrder, 1))

	if err == nil || !strings.Contains(err.Error(), "missing result") {
		t.Fatalf("pollOrders() error = %v, want terminal lookup failure", err)
	}
	if !solver.exclusiveStateUnknown.Load() || solver.quoteState.Load() != nil {
		t.Fatal("unknown exclusive state did not stop quotes")
	}
	if solver.exclusiveBlockUntil.Load() != 0 {
		t.Fatal("unknown exclusive state was counted as a fade")
	}
	if _, pending := solver.exclusiveUntil[hash]; !pending {
		t.Fatal("unknown exclusive obligation was not retained for retry")
	}
}

func TestPollSourceTracksRejectedExclusiveOrder(t *testing.T) {
	now := time.Unix(1_000, 0)
	entry, cfg := testExclusiveOrderEntry(
		t,
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	)
	solver := newPollingTestSolver(
		cfg,
		orderPollerFunc(func(context.Context, int64, *common.Address) ([]orderEntry, error) {
			return []orderEntry{entry}, nil
		}),
	)
	solver.metrics = newUniswapXTestMetrics(t, solver)

	for range 2 {
		if _, err := solver.pollSource(
			t.Context(),
			orderSourceExclusiveV2,
			&cfg.Executor,
			make(chan *resolvedOrder, 1),
		); err != nil {
			t.Fatal(err)
		}
	}
	hash := common.HexToHash(entry.OrderHash)
	if tracked, ok := solver.exclusiveUntil[hash]; !ok || !tracked.deadline.Equal(now) {
		t.Fatalf("rejected exclusive obligation = %v, tracked=%v", tracked.deadline, ok)
	}
	if got := testutil.ToFloat64(solver.metrics.exclusiveWins); got != 1 {
		t.Fatalf("exclusive wins = %v, want 1", got)
	}
	if got := testutil.ToFloat64(solver.metrics.exclusiveOutstanding); got != 1 {
		t.Fatalf("outstanding obligations = %v, want 1", got)
	}
	if got := testutil.ToFloat64(solver.metrics.exclusiveDeadline); got != float64(now.Unix()) {
		t.Fatalf("nearest deadline = %v, want %d", got, now.Unix())
	}
}

func TestPollOrdersRecoversTerminalExclusiveOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		startup bool
	}{
		{name: "startup logs historical miss", startup: true},
		{name: "runtime recovery opens breaker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
			entry, cfg := testExclusiveOrderEntry(t, executor)
			entry.OrderStatus = orderStatusExpired
			otherFiller, _ := testExclusiveOrderEntry(
				t,
				common.HexToAddress("0x9999999999999999999999999999999999999999"),
			)
			otherFiller.OrderStatus = orderStatusExpired
			cfg.Breaker.Window = time.Minute
			hash := common.HexToHash(entry.OrderHash)
			var logs []string
			solver := newPollingTestSolver(
				cfg,
				&stateTestOrderPoller{
					recent: []orderEntry{otherFiller, entry},
					terminals: map[common.Hash]orderTerminal{
						hash: {Status: orderStatusExpired},
					},
				},
			)
			solver.reader = &countingChainReader{now: time.Unix(1_001, 0)}
			solver.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			solver.metrics = newUniswapXTestMetrics(t, solver)
			solver.quoteState.Store(&quoteState{})
			solver.exclusiveStateUnknown.Store(true)
			if !tc.startup {
				solver.lastExclusivePoll.Store(999)
			}

			if err := solver.pollOrders(t.Context(), make(chan *resolvedOrder, 1)); err != nil {
				t.Fatal(err)
			}
			wantBreaker := !tc.startup
			if got := solver.exclusiveBlockUntil.Load() != 0; got != wantBreaker {
				t.Fatalf("recovered missed exclusive breaker = %v, want %v", got, wantBreaker)
			}
			if got := solver.quoteState.Load() == nil; got != wantBreaker {
				t.Fatalf("recovered missed exclusive invalidated quotes = %v, want %v", got, wantBreaker)
			}
			if _, pending := solver.exclusiveUntil[hash]; pending {
				t.Fatal("recovered terminal obligation remained pending")
			}
			if _, terminal := solver.exclusiveTerminal[hash]; !terminal {
				t.Fatal("recovered terminal obligation was not retained")
			}

			logged := strings.Join(logs, "\n")
			if got := strings.Contains(logged, "historical exclusive obligation missed"); got != tc.startup {
				t.Fatalf("historical miss log = %v, want %v: %s", got, tc.startup, logged)
			}
			if tc.startup && strings.Contains(logged, `"error"`) {
				t.Fatalf("startup-recovered miss was logged as an error: %s", logged)
			}
			if got := testutil.ToFloat64(solver.metrics.exclusiveWins); got != 0 {
				t.Fatalf("recovery replayed wins: %v", got)
			}
			if got := testutil.ToFloat64(
				solver.metrics.exclusiveOutcomes.WithLabelValues(exclusiveOutcomeMissed),
			); got != 0 {
				t.Fatalf("recovery replayed missed outcomes: %v", got)
			}

			initialLogCount := strings.Count(logged, "historical exclusive obligation missed")
			solver.exclusiveStateUnknown.Store(true)
			if err := solver.pollOrders(t.Context(), make(chan *resolvedOrder, 1)); err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(
				strings.Join(logs, "\n"),
				"historical exclusive obligation missed",
			); got != initialLogCount {
				t.Fatalf("historical miss log repeated: got %d entries, want %d", got, initialLogCount)
			}
		})
	}
}

func TestPollOrdersStopsQuotesWhenExclusiveHistoryIsUnknown(t *testing.T) {
	solver := newPollingTestSolver(
		&Config{
			Executor: common.HexToAddress("0x1111111111111111111111111111111111111111"),
			Breaker:  BreakerConfig{Window: time.Minute},
		},
		&stateTestOrderPoller{err: errors.New("history unavailable")},
	)
	solver.exclusiveStateUnknown.Store(true)
	solver.quoteState.Store(&quoteState{expiresAt: time.Now().Add(time.Minute)})

	err := solver.pollOrders(t.Context(), make(chan *resolvedOrder, 1))
	if err == nil || !strings.Contains(err.Error(), "poll recent exclusive orders") {
		t.Fatalf("pollOrders error = %v", err)
	}
	if !solver.exclusiveStateUnknown.Load() || solver.quoteState.Load() != nil {
		t.Fatal("unknown recovery history did not stop quotes")
	}
}

func newPollingTestSolver(cfg *Config, orders orderPoller) *Solver {
	cfg.OrderServer.Sources.ExclusiveV2 = true
	return &Solver{
		cfg: cfg, chainID: 1, reader: &countingChainReader{}, orders: orders, log: logr.Discard(),
	}
}
