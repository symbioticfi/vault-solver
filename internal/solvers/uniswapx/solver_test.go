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

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type orderPollerFunc func(context.Context, int64, *common.Address) ([]orderEntry, error)

func (f orderPollerFunc) openOrders(ctx context.Context, chainID int64, filler *common.Address) ([]orderEntry, error) {
	return f(ctx, chainID, filler)
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
	solver := &Solver{
		cfg: &Config{
			OrderServer: OrderServerConfig{
				PollInterval: time.Second,
				Sources:      OrderSourcesConfig{ExclusiveV2: true},
			},
			Breaker: BreakerConfig{Window: time.Minute},
		},
		chainID:  1,
		reader:   &countingChainReader{},
		orders:   &stateTestOrderPoller{terminals: map[common.Hash]orderTerminal{}},
		log:      logr.Discard(),
		filled:   make(map[common.Hash]time.Time),
		retryAt:  make(map[common.Hash]time.Time),
		inFlight: make(map[common.Hash]bool),
		attempts: make(map[common.Hash]int),
	}
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
