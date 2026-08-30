package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

type lifecycleTrace struct {
	mu     sync.Mutex
	events []string
}

func (t *lifecycleTrace) add(event string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *lifecycleTrace) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.events)
}

type characterizationManager struct {
	trace            *lifecycleTrace
	validateErr      error
	initializeErr    error
	started          chan struct{}
	stopped          chan struct{}
	laneChanges      chan struct{}
	laneReady        atomic.Bool
	startOnce        sync.Once
	stopOnce         sync.Once
	validateCalls    atomic.Int64
	initCalls        atomic.Int64
	startCalls       atomic.Int64
	subscribeCalls   atomic.Int64
	laneCalls        atomic.Int64
	unsubscribeCalls atomic.Int64
}

func newCharacterizationManager(trace *lifecycleTrace) *characterizationManager {
	manager := &characterizationManager{
		trace:       trace,
		started:     make(chan struct{}),
		stopped:     make(chan struct{}),
		laneChanges: make(chan struct{}, 1),
	}
	manager.laneReady.Store(true)
	return manager
}

func (m *characterizationManager) ValidateFeeHeadroom() error {
	m.validateCalls.Add(1)
	m.trace.add("manager validate")
	return m.validateErr
}

func (m *characterizationManager) Initialize(context.Context) error {
	m.initCalls.Add(1)
	m.trace.add("manager initialize")
	return m.initializeErr
}

func (m *characterizationManager) Start(ctx context.Context) {
	m.startCalls.Add(1)
	m.trace.add("manager start")
	m.startOnce.Do(func() { close(m.started) })
	<-ctx.Done()
	m.trace.add("manager stop")
	m.stopOnce.Do(func() { close(m.stopped) })
}

func (m *characterizationManager) SubscribeLaneState() (<-chan struct{}, func()) {
	m.subscribeCalls.Add(1)
	m.trace.add("watcher subscribe")
	return m.laneChanges, func() {
		m.unsubscribeCalls.Add(1)
		m.trace.add("watcher joined")
	}
}

func (m *characterizationManager) LaneReady() bool {
	m.laneCalls.Add(1)
	ready := m.laneReady.Load()
	m.trace.add(fmt.Sprintf("lane ready %t", ready))
	return ready
}

type characterizationHealth struct {
	trace   *lifecycleTrace
	updates chan bool
	calls   atomic.Int64
}

func newCharacterizationHealth(trace *lifecycleTrace) *characterizationHealth {
	return &characterizationHealth{trace: trace, updates: make(chan bool, 16)}
}

func (h *characterizationHealth) SetReady(ready bool) {
	h.calls.Add(1)
	h.trace.add(fmt.Sprintf("readiness %t", ready))
	h.updates <- ready
}

type characterizationSolver struct {
	name string
	run  func(context.Context) error
}

func (s characterizationSolver) Name() string { return s.name }
func (s characterizationSolver) Run(ctx context.Context) error {
	return s.run(ctx)
}

func TestSolverLifecycleSelectsTransactionManagerFromCommandRequirement(t *testing.T) {
	tests := []struct {
		name            string
		solverNames     []string
		requiresManager bool
	}{
		{
			name:            "OEV only",
			solverNames:     []string{"redstone-oev"},
			requiresManager: false,
		},
		{
			name:            "transaction solver",
			solverNames:     []string{"rfq-filler"},
			requiresManager: true,
		},
		{
			name:            "mixed OEV and transaction solver",
			solverNames:     []string{"redstone-oev", "lifi-samechain"},
			requiresManager: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testSolverLifecycleSelection(t, test.solverNames, test.requiresManager)
		})
	}
}

func testSolverLifecycleSelection(t *testing.T, solverNames []string, requiresManager bool) {
	t.Helper()
	trace := new(lifecycleTrace)
	health := newCharacterizationHealth(trace)
	manager := newCharacterizationManager(trace)
	solvers := make([]solver.Solver, 0, len(solverNames))
	solverStarted := make(chan struct{}, len(solverNames))
	for _, name := range solverNames {
		solvers = append(solvers, characterizationSolver{name: name, run: func(ctx context.Context) error {
			if requiresManager {
				<-manager.started
			}
			trace.add("solver start")
			solverStarted <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		}})
	}

	var runtimeManager runtimeTransactionManager
	if requiresManager {
		runtimeManager = manager
	}
	ctx, cancel := context.WithCancel(t.Context())
	returned := make(chan error, 1)
	go func() {
		returned <- runSolverLifecycle(
			ctx, "characterization.yaml", lifecycleConfig(), runtimeManager,
			solvers, requiresManager, health, logr.Discard(),
		)
	}()
	expectReadyState(t, health.updates, true)
	for range solverNames {
		waitForSignal(t, solverStarted, "solver start")
	}
	if requiresManager {
		manager.laneReady.Store(false)
		manager.laneChanges <- struct{}{}
		expectReadyState(t, health.updates, false)
		manager.laneReady.Store(true)
		manager.laneChanges <- struct{}{}
		expectReadyState(t, health.updates, true)
	}
	cancel()
	if requiresManager {
		expectReadyState(t, health.updates, false)
	}
	if err := waitForError(t, returned, "lifecycle return"); err != nil {
		t.Fatalf("runSolverLifecycle: %v", err)
	}

	wantCalls := int64(0)
	if requiresManager {
		wantCalls = 1
	}
	for name, got := range map[string]int64{
		"validate":    manager.validateCalls.Load(),
		"initialize":  manager.initCalls.Load(),
		"start":       manager.startCalls.Load(),
		"subscribe":   manager.subscribeCalls.Load(),
		"unsubscribe": manager.unsubscribeCalls.Load(),
	} {
		if got != wantCalls {
			t.Fatalf("manager %s calls = %d, want %d; trace %v", name, got, wantCalls, trace.snapshot())
		}
	}
	if requiresManager {
		if manager.laneCalls.Load() != 2 {
			t.Fatalf("LaneReady calls = %d, want 2; trace %v", manager.laneCalls.Load(), trace.snapshot())
		}
		assertTraceOrder(t, trace.snapshot(),
			"manager validate", "manager initialize", "manager start", "solver start",
		)
	}
}

func TestSolverLifecycleShutdownTrace(t *testing.T) {
	trace := new(lifecycleTrace)
	manager := newCharacterizationManager(trace)
	health := newCharacterizationHealth(trace)
	acceptedStarted := make(chan struct{})
	intakeStopped := make(chan struct{})
	releaseAccepted := make(chan struct{})
	fatalErr := errors.New("fatal solver failure")

	accepted := characterizationSolver{name: "accepted", run: func(ctx context.Context) error {
		<-manager.started
		trace.add("accepted solver start")
		close(acceptedStarted)
		<-ctx.Done()
		trace.add("accepted solver intake stopped")
		close(intakeStopped)
		<-releaseAccepted
		trace.add("accepted solver complete")
		return ctx.Err()
	}}
	fatal := characterizationSolver{name: "fatal", run: func(context.Context) error {
		<-acceptedStarted
		trace.add("fatal solver error")
		return fatalErr
	}}

	returned := make(chan error, 1)
	go func() {
		err := runSolverLifecycle(
			t.Context(), "characterization.yaml", lifecycleConfig(), manager,
			[]solver.Solver{accepted, fatal}, true, health, logr.Discard(),
		)
		trace.add("command return")
		returned <- err
	}()
	expectReadyState(t, health.updates, true)
	waitForSignal(t, intakeStopped, "solver intake stop")
	expectReadyState(t, health.updates, false)
	assertNotSignaled(t, manager.stopped, "transaction manager stopped before accepted solver completed")
	assertNotSignaled(t, returned, "command returned before accepted solver completed")
	close(releaseAccepted)
	err := waitForError(t, returned, "command return")
	if err == nil || !errors.Is(err, fatalErr) || err.Error() != `solver "fatal": fatal solver failure` {
		t.Fatalf("lifecycle error = %v, want wrapped fatal solver error", err)
	}

	events := trace.snapshot()
	assertTraceOrder(t, events,
		"manager start",
		"accepted solver start",
		"fatal solver error",
		"accepted solver intake stopped",
		"accepted solver complete",
		"manager stop",
		"command return",
	)
	assertEventBefore(t, events, "watcher joined", "command return")
}

func TestSolverLifecycleTimeoutStopsManagerWithoutAbandoningSolverJoin(t *testing.T) {
	trace := new(lifecycleTrace)
	manager := newCharacterizationManager(trace)
	health := newCharacterizationHealth(trace)
	solverStarted := make(chan struct{})
	intakeStopped := make(chan struct{})
	releaseSolver := make(chan struct{})
	var (
		logsMu sync.Mutex
		logs   []string
	)
	log := funcr.NewJSON(func(entry string) {
		logsMu.Lock()
		defer logsMu.Unlock()
		logs = append(logs, entry)
	}, funcr.Options{})
	blocking := characterizationSolver{name: "accepted", run: func(ctx context.Context) error {
		<-manager.started
		close(solverStarted)
		<-ctx.Done()
		trace.add("accepted solver intake stopped")
		close(intakeStopped)
		<-releaseSolver
		trace.add("accepted solver complete")
		return ctx.Err()
	}}

	ctx, cancel := context.WithCancel(t.Context())
	returned := make(chan error, 1)
	go func() {
		returned <- runSolverLifecycle(
			ctx, "characterization.yaml", timeoutLifecycleConfig(), manager,
			[]solver.Solver{blocking}, true, health, log,
		)
	}()
	expectReadyState(t, health.updates, true)
	waitForSignal(t, solverStarted, "solver start")
	cancel()
	waitForSignal(t, intakeStopped, "solver intake stop")
	waitForSignal(t, manager.stopped, "forced transaction manager stop")
	assertNotSignaled(t, returned, "command returned before timed-out solver joined")
	close(releaseSolver)
	if err := waitForError(t, returned, "command return"); err != nil {
		t.Fatalf("runSolverLifecycle: %v", err)
	}

	logsMu.Lock()
	captured := slices.Clone(logs)
	logsMu.Unlock()
	found := false
	for _, entry := range captured {
		var record map[string]any
		if err := json.Unmarshal([]byte(entry), &record); err != nil {
			t.Fatalf("decode log %q: %v", entry, err)
		}
		if record["msg"] == "solver shutdown timed out; stopping tx manager" {
			found = true
			if record["timeout"] != "2ms" {
				t.Fatalf("timeout log = %v, want 2ms", record["timeout"])
			}
		}
	}
	if !found {
		t.Fatalf("missing exact shutdown-timeout log in %v", captured)
	}
	assertTraceOrder(t, trace.snapshot(),
		"accepted solver intake stopped",
		"manager stop",
		"accepted solver complete",
	)
}

func TestSolverLifecycleManagerStartupFailuresDoNotStartRuntime(t *testing.T) {
	validateErr := errors.New("invalid fee headroom")
	initializeErr := errors.New("nonce initialization failed")
	tests := []struct {
		name          string
		validateErr   error
		initializeErr error
		want          string
		wantInitCalls int64
	}{
		{
			name:        "fee validation",
			validateErr: validateErr,
			want:        `invalid config "characterization.yaml": txManager: invalid fee headroom`,
		},
		{
			name:          "initialization",
			initializeErr: initializeErr,
			want:          "initialize tx manager: nonce initialization failed",
			wantInitCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace := new(lifecycleTrace)
			manager := newCharacterizationManager(trace)
			manager.validateErr = test.validateErr
			manager.initializeErr = test.initializeErr
			health := newCharacterizationHealth(trace)
			solverCalls := atomic.Int64{}
			err := runSolverLifecycle(
				t.Context(), "characterization.yaml", lifecycleConfig(), manager,
				[]solver.Solver{characterizationSolver{name: "must-not-run", run: func(context.Context) error {
					solverCalls.Add(1)
					return nil
				}}}, true, health, logr.Discard(),
			)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if manager.validateCalls.Load() != 1 || manager.initCalls.Load() != test.wantInitCalls {
				t.Fatalf("manager calls validate=%d initialize=%d, want 1/%d", manager.validateCalls.Load(), manager.initCalls.Load(), test.wantInitCalls)
			}
			if manager.startCalls.Load() != 0 || manager.subscribeCalls.Load() != 0 ||
				health.calls.Load() != 0 || solverCalls.Load() != 0 {
				t.Fatalf("runtime started after startup failure: trace %v", trace.snapshot())
			}
			assertNotSignaled(t, manager.started, "manager goroutine leaked after startup failure")
		})
	}
}

func lifecycleConfig() *config.Config {
	return &config.Config{TxManager: config.TxManagerConfig{
		MaxFeeGwei:            50,
		BroadcastTimeoutMs:    1,
		ReplacementIntervalMs: 1_000,
		PendingTimeoutMs:      2_000,
		ShutdownTimeoutMs:     1,
	}}
}

func timeoutLifecycleConfig() *config.Config {
	cfg := lifecycleConfig()
	cfg.TxManager.ReplacementIntervalMs = 1
	cfg.TxManager.PendingTimeoutMs = 1
	return cfg
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForError(t *testing.T, result <-chan error, description string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
		return nil
	}
}

func assertNotSignaled[T any](t *testing.T, signal <-chan T, description string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(description)
	default:
	}
}

func assertTraceOrder(t *testing.T, events []string, ordered ...string) {
	t.Helper()
	previous := -1
	for _, event := range ordered {
		index := slices.Index(events, event)
		if index < 0 {
			t.Fatalf("trace missing %q: %v", event, events)
		}
		if index <= previous {
			t.Fatalf("trace order for %q = %v, want %v", event, events, ordered)
		}
		previous = index
	}
}

func assertEventBefore(t *testing.T, events []string, before, after string) {
	t.Helper()
	beforeIndex := slices.Index(events, before)
	afterIndex := slices.Index(events, after)
	if beforeIndex < 0 || afterIndex < 0 || beforeIndex >= afterIndex {
		t.Fatalf("trace does not place %q before %q: %v", before, after, events)
	}
}
