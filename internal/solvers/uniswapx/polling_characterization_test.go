package uniswapx

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type pollCharacterizationTrace struct {
	mu     sync.Mutex
	events []string
}

func (t *pollCharacterizationTrace) add(event string) {
	t.mu.Lock()
	t.events = append(t.events, event)
	t.mu.Unlock()
}

func (t *pollCharacterizationTrace) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.events...)
}

type pollCharacterizationOrders struct {
	trace *pollCharacterizationTrace

	exclusiveEntries []orderEntry
	exclusiveErr     error
	publicEntries    []orderEntry
	publicErr        error
	recentEntries    []orderEntry
	recentErr        error
	terminals        map[common.Hash]orderTerminal
	terminalsErr     error
}

func (p *pollCharacterizationOrders) openOrders(
	_ context.Context,
	_ int64,
	filler *common.Address,
) ([]orderEntry, error) {
	if filler != nil {
		p.trace.add("api.open.exclusive")
		return p.exclusiveEntries, p.exclusiveErr
	}
	p.trace.add("api.open.public")
	return p.publicEntries, p.publicErr
}

func (p *pollCharacterizationOrders) recentOrders(
	context.Context,
	int64,
	common.Address,
	time.Time,
) ([]orderEntry, error) {
	p.trace.add("api.history")
	return p.recentEntries, p.recentErr
}

func (p *pollCharacterizationOrders) ordersByHash(
	context.Context,
	int64,
	[]common.Hash,
) (map[common.Hash]orderTerminal, error) {
	p.trace.add("api.terminals")
	return p.terminals, p.terminalsErr
}

type pollCharacterizationReader struct {
	chainReader

	trace      *pollCharacterizationTrace
	now        time.Time
	receiptAt  time.Time
	receiptErr error
}

func (r *pollCharacterizationReader) latestBlockTime(context.Context) (time.Time, error) {
	r.trace.add("reader.latest")
	return r.now, nil
}

func (r *pollCharacterizationReader) transactionBlockTimeConfirmed(
	context.Context,
	common.Hash,
	uint64,
) (time.Time, error) {
	r.trace.add("reader.receipt")
	return r.receiptAt, r.receiptErr
}

type pollCharacterizationCase struct {
	name              string
	unknown           bool
	configure         func(*pollCharacterizationOrders, *pollCharacterizationReader)
	wantError         string
	wantEvents        []string
	wantSuccess       bool
	wantTracked       common.Hash
	wantSettledMetric float64
}

func TestPollOrdersConditionalSequenceAndEffects(t *testing.T) {
	chainNow := time.Unix(1_001, 0)
	deadline := time.Unix(1_000, 0)
	hash := common.HexToHash("0x1234")
	txHash := common.HexToHash("0xabcd")
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	partialEntry, partialConfig := testExclusiveOrderEntry(t, executor)

	tests := []pollCharacterizationCase{
		{
			name:    "successful exclusive history terminal receipt then public",
			unknown: true,
			configure: func(orders *pollCharacterizationOrders, reader *pollCharacterizationReader) {
				orders.terminals = map[common.Hash]orderTerminal{
					hash: {Status: orderStatusFilled, TxHash: txHash},
				}
				reader.receiptAt = deadline
			},
			wantEvents: []string{
				"api.open.exclusive", "reader.latest", "api.history", "api.terminals",
				"reader.receipt", "api.open.public", "reader.latest",
			},
			wantSuccess: true, wantSettledMetric: 1,
		},
		{
			name: "exclusive partial processes entries but skips recovery and sweep",
			configure: func(orders *pollCharacterizationOrders, _ *pollCharacterizationReader) {
				orders.exclusiveEntries = []orderEntry{partialEntry}
				orders.exclusiveErr = errors.New("exclusive partial")
			},
			wantError: "poll exclusive-v2 orders: exclusive partial",
			wantEvents: []string{
				"api.open.exclusive", "reader.latest", "api.open.public", "reader.latest",
			},
			wantTracked: common.HexToHash(partialEntry.OrderHash),
		},
		{
			name:    "history recovery failure still polls public",
			unknown: true,
			configure: func(orders *pollCharacterizationOrders, _ *pollCharacterizationReader) {
				orders.recentErr = errors.New("history unavailable")
			},
			wantError: "poll recent exclusive orders: history unavailable",
			wantEvents: []string{
				"api.open.exclusive", "reader.latest", "api.history", "api.open.public", "reader.latest",
			},
		},
		{
			name: "terminal sweep failure still polls public",
			configure: func(orders *pollCharacterizationOrders, _ *pollCharacterizationReader) {
				orders.terminals = map[common.Hash]orderTerminal{}
			},
			wantError: "reconcile exclusive orders: lookup expired obligation " + hash.Hex() + ": missing result",
			wantEvents: []string{
				"api.open.exclusive", "reader.latest", "api.terminals", "api.open.public", "reader.latest",
			},
		},
		{
			name: "receipt reconciliation failure still polls public",
			configure: func(orders *pollCharacterizationOrders, reader *pollCharacterizationReader) {
				orders.terminals = map[common.Hash]orderTerminal{
					hash: {Status: orderStatusFilled, TxHash: txHash},
				}
				reader.receiptErr = errors.New("receipt uncertain")
			},
			wantError: "reconcile exclusive orders: lookup expired obligation " + hash.Hex() + " fill time: receipt uncertain",
			wantEvents: []string{
				"api.open.exclusive", "reader.latest", "api.terminals", "reader.receipt",
				"api.open.public", "reader.latest",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runPollCharacterizationCase(t, tc, partialConfig, chainNow, deadline, hash, executor)
		})
	}
}

func runPollCharacterizationCase(
	t *testing.T,
	tc pollCharacterizationCase,
	partialConfig *Config,
	chainNow, deadline time.Time,
	hash common.Hash,
	executor common.Address,
) {
	t.Helper()
	trace := new(pollCharacterizationTrace)
	orders := &pollCharacterizationOrders{trace: trace}
	reader := &pollCharacterizationReader{trace: trace, now: chainNow}
	tc.configure(orders, reader)
	cfg := pollCharacterizationConfig(executor)
	if tc.wantTracked != (common.Hash{}) {
		cfg = partialConfig
		cfg.OrderServer = OrderServerConfig{
			PollInterval: time.Second, Sources: OrderSourcesConfig{PublicV2: true},
		}
		cfg.Breaker = BreakerConfig{Window: time.Minute}
	}
	solver := &Solver{
		cfg: cfg, chainID: 1, reader: reader, orders: orders, log: logr.Discard(),
		orderState:     make(map[common.Hash]trackedOrder),
		exclusiveState: map[common.Hash]trackedExclusive{hash: {deadline: deadline}},
	}
	solver.exclusiveStateUnknown.Store(tc.unknown)
	solver.quoteEpoch.Store(5)
	quote := &quoteState{
		epoch: 5, inventory: []liquidlane.Inventory{{}}, expiresAt: time.Now().Add(time.Hour),
	}
	solver.quoteState.Store(quote)
	metrics, err := newUniswapXMetrics(prometheus.NewRegistry(), solver.ready)
	if err != nil {
		t.Fatal(err)
	}
	solver.metrics = metrics

	err = solver.pollOrders(t.Context(), make(chan *resolvedOrder, 2))
	assertPollCharacterizationError(t, err, tc.wantError)
	if events := trace.snapshot(); !reflect.DeepEqual(events, tc.wantEvents) {
		t.Fatalf("poll event order = %v, want %v", events, tc.wantEvents)
	}
	assertPollCharacterizationMetrics(t, metrics, tc)
	if tc.wantSuccess {
		assertSuccessfulPollEffects(t, solver, metrics, quote, hash)
	} else {
		assertFailedPollEffects(t, solver, metrics, hash)
	}
	assertPartialPollEntry(t, solver, tc.wantTracked, deadline)
}

func pollCharacterizationConfig(executor common.Address) *Config {
	return &Config{
		Executor: executor,
		OrderServer: OrderServerConfig{
			PollInterval: time.Second, Sources: OrderSourcesConfig{PublicV2: true},
		},
		Breaker: BreakerConfig{Window: time.Minute},
	}
}

func assertPollCharacterizationError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" && err != nil {
		t.Fatalf("pollOrders() error = %v", err)
	}
	if want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
		t.Fatalf("pollOrders() error = %v, want containing %q", err, want)
	}
}

func assertPollCharacterizationMetrics(
	t *testing.T,
	metrics *uniswapXMetrics,
	tc pollCharacterizationCase,
) {
	t.Helper()
	wantExclusiveOutcome := "failed"
	if tc.wantSuccess {
		wantExclusiveOutcome = "ok"
	}
	if got := testutil.ToFloat64(metrics.polls.WithLabelValues(string(orderSourceExclusiveV2), wantExclusiveOutcome)); got != 1 {
		t.Fatalf("exclusive %s poll metric = %v, want 1", wantExclusiveOutcome, got)
	}
	if got := testutil.ToFloat64(metrics.polls.WithLabelValues(string(orderSourcePublicV2), "ok")); got != 1 {
		t.Fatalf("public ok poll metric = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.fills.WithLabelValues("exclusive-settled-in-time")); got != tc.wantSettledMetric {
		t.Fatalf("settled fill metric = %v, want %v", got, tc.wantSettledMetric)
	}
}

func assertSuccessfulPollEffects(
	t *testing.T,
	solver *Solver,
	metrics *uniswapXMetrics,
	quote *quoteState,
	hash common.Hash,
) {
	t.Helper()
	if solver.exclusiveStateUnknown.Load() || solver.lastExclusivePoll.Load() == 0 {
		t.Fatal("successful reconciliation did not make exclusive delivery known")
	}
	if got := int64(testutil.ToFloat64(metrics.exclusivePoll)); got != solver.lastExclusivePoll.Load() {
		t.Fatalf("exclusive poll gauge = %d, want %d", got, solver.lastExclusivePoll.Load())
	}
	if solver.quoteState.Load() != quote || solver.quoteEpoch.Load() != 5 {
		t.Fatal("successful poll unexpectedly invalidated quotes")
	}
	if !solver.ready() || testutil.ToFloat64(metrics.ready) != 1 {
		t.Fatal("successful poll did not restore readiness")
	}
	if tracked := solver.exclusiveState[hash]; !tracked.terminal() || tracked.pending() {
		t.Fatalf("settled obligation state = %+v", tracked)
	}
}

func assertFailedPollEffects(
	t *testing.T,
	solver *Solver,
	metrics *uniswapXMetrics,
	hash common.Hash,
) {
	t.Helper()
	if !solver.exclusiveStateUnknown.Load() || solver.lastExclusivePoll.Load() != 0 {
		t.Fatal("failed exclusive reconciliation was recorded as known/successful")
	}
	if solver.quoteState.Load() != nil || solver.quoteEpoch.Load() != 6 {
		t.Fatalf("failure quote state/epoch = %p/%d, want nil/6", solver.quoteState.Load(), solver.quoteEpoch.Load())
	}
	if solver.ready() || testutil.ToFloat64(metrics.ready) != 0 {
		t.Fatal("failed exclusive reconciliation remained ready")
	}
	if tracked := solver.exclusiveState[hash]; !tracked.pending() {
		t.Fatalf("failed reconciliation terminalized obligation: %+v", tracked)
	}
}

func assertPartialPollEntry(t *testing.T, solver *Solver, hash common.Hash, deadline time.Time) {
	t.Helper()
	if hash == (common.Hash{}) {
		return
	}
	if tracked, ok := solver.exclusiveState[hash]; !ok || !tracked.pending() || !tracked.deadline.Equal(deadline) {
		t.Fatalf("partial exclusive entry was not processed: tracked=%+v ok=%v", tracked, ok)
	}
}
