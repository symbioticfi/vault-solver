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
)

type characterizationLogRecord struct {
	level     string
	verbosity int
	message   string
	err       string
	fields    []any
}

type characterizationLogState struct {
	mu      sync.Mutex
	records []characterizationLogRecord
}

type characterizationLogSink struct {
	state  *characterizationLogState
	values []any
}

func (s *characterizationLogSink) Init(logr.RuntimeInfo) {}

func (s *characterizationLogSink) Enabled(int) bool { return true }

func (s *characterizationLogSink) Info(level int, message string, fields ...any) {
	s.append(characterizationLogRecord{
		level: "info", verbosity: level, message: message,
		fields: append(append([]any(nil), s.values...), fields...),
	})
}

func (s *characterizationLogSink) Error(err error, message string, fields ...any) {
	s.append(characterizationLogRecord{
		level: "error", message: message, err: err.Error(),
		fields: append(append([]any(nil), s.values...), fields...),
	})
}

func (s *characterizationLogSink) WithValues(fields ...any) logr.LogSink {
	return &characterizationLogSink{
		state:  s.state,
		values: append(append([]any(nil), s.values...), fields...),
	}
}

func (s *characterizationLogSink) WithName(string) logr.LogSink {
	return &characterizationLogSink{state: s.state, values: append([]any(nil), s.values...)}
}

func (s *characterizationLogSink) append(record characterizationLogRecord) {
	s.state.mu.Lock()
	s.state.records = append(s.state.records, record)
	s.state.mu.Unlock()
}

func newCharacterizationLogger() (logr.Logger, *characterizationLogState) {
	state := new(characterizationLogState)
	return logr.New(&characterizationLogSink{state: state}), state
}

func (s *characterizationLogState) recordsFor(message string) []characterizationLogRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []characterizationLogRecord
	for _, record := range s.records {
		if record.message == message {
			records = append(records, record)
		}
	}
	return records
}

type characterizationOrderPoller struct {
	terminals map[common.Hash]orderTerminal
	recent    []orderEntry
	err       error
	recentErr error
	calls     int
	started   chan struct{}
	release   chan struct{}
}

func (p *characterizationOrderPoller) openOrders(
	context.Context,
	int64,
	*common.Address,
) ([]orderEntry, error) {
	return nil, nil
}

func (p *characterizationOrderPoller) recentOrders(
	context.Context,
	int64,
	common.Address,
	time.Time,
) ([]orderEntry, error) {
	return p.recent, p.recentErr
}

func (p *characterizationOrderPoller) ordersByHash(
	_ context.Context,
	_ int64,
	_ []common.Hash,
) (map[common.Hash]orderTerminal, error) {
	p.calls++
	if p.started != nil {
		close(p.started)
		<-p.release
	}
	return p.terminals, p.err
}

type characterizationReceiptReader struct {
	chainReader

	mu            sync.Mutex
	times         map[common.Hash]time.Time
	err           error
	failAfter     int
	calls         int
	confirmations []uint64
	started       chan struct{}
	release       chan struct{}
}

func (r *characterizationReceiptReader) transactionBlockTimeConfirmed(
	_ context.Context,
	hash common.Hash,
	confirmations uint64,
) (time.Time, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.confirmations = append(r.confirmations, confirmations)
	started, release := r.started, r.release
	r.mu.Unlock()
	if started != nil {
		close(started)
		<-release
	}
	if r.err != nil && (r.failAfter == 0 || call >= r.failAfter) {
		return time.Time{}, r.err
	}
	return r.times[hash], nil
}

func (r *characterizationReceiptReader) snapshot() (int, []uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]uint64(nil), r.confirmations...)
}

type sweepCharacterizationOutcome string

const (
	sweepSettled   sweepCharacterizationOutcome = "settled"
	sweepMissed    sweepCharacterizationOutcome = "missed"
	sweepUncertain sweepCharacterizationOutcome = "uncertain"
)

type sweepCharacterizationCase struct {
	name            string
	terminalPresent bool
	terminal        orderTerminal
	filledAt        time.Time
	lookupErr       error
	receiptErr      error
	wantOutcome     sweepCharacterizationOutcome
	wantError       string
}

func TestSweepExclusiveCharacterization(t *testing.T) {
	deadline := time.Unix(1_000, 0)
	now := deadline.Add(10 * time.Second)
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	trackingEntry, _ := testExclusiveOrderEntry(t, executor)
	hash := common.HexToHash(trackingEntry.OrderHash)
	txHash := common.HexToHash("0xabcd")
	lookupErr := errors.New("order API unavailable")
	receiptErr := errors.New("receipt unavailable")

	cases := []sweepCharacterizationCase{
		{
			name: "filled before deadline", terminalPresent: true,
			terminal: orderTerminal{Status: orderStatusFilled, TxHash: txHash},
			filledAt: deadline.Add(-time.Second), wantOutcome: sweepSettled,
		},
		{
			name: "filled exactly at deadline", terminalPresent: true,
			terminal: orderTerminal{Status: orderStatusFilled, TxHash: txHash},
			filledAt: deadline, wantOutcome: sweepSettled,
		},
		{
			name: "filled after deadline", terminalPresent: true,
			terminal: orderTerminal{Status: orderStatusFilled, TxHash: txHash},
			filledAt: deadline.Add(time.Second), wantOutcome: sweepMissed,
		},
	}
	for _, status := range []string{
		orderStatusExpired,
		orderStatusError,
		orderStatusCancelled,
		orderStatusInsufficientFunds,
	} {
		cases = append(cases, sweepCharacterizationCase{
			name: status, terminalPresent: true,
			terminal: orderTerminal{Status: status}, wantOutcome: sweepMissed,
		})
	}
	cases = append(cases,
		sweepCharacterizationCase{
			name: "open", terminalPresent: true,
			terminal: orderTerminal{Status: orderStatusOpen}, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + ": order is still open",
		},
		sweepCharacterizationCase{
			name: "empty status", terminalPresent: true,
			terminal: orderTerminal{}, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + ": unknown status \"\"",
		},
		sweepCharacterizationCase{
			name: "unknown status", terminalPresent: true,
			terminal: orderTerminal{Status: "refunded"}, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + ": unknown status \"refunded\"",
		},
		sweepCharacterizationCase{
			name: "missing terminal", wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + ": missing result",
		},
		sweepCharacterizationCase{
			name: "filled without transaction", terminalPresent: true,
			terminal: orderTerminal{Status: orderStatusFilled}, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + ": filled order has no transaction",
		},
		sweepCharacterizationCase{
			name: "non-fill with transaction", terminalPresent: true,
			terminal: orderTerminal{Status: orderStatusExpired, TxHash: txHash}, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + ": status \"expired\" unexpectedly has transaction " + txHash.Hex(),
		},
		sweepCharacterizationCase{
			name: "receipt reader error", terminalPresent: true,
			terminal:   orderTerminal{Status: orderStatusFilled, TxHash: txHash},
			receiptErr: receiptErr, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligation " + hash.Hex() + " fill time: receipt unavailable",
		},
		sweepCharacterizationCase{
			name: "terminal lookup error", lookupErr: lookupErr, wantOutcome: sweepUncertain,
			wantError: "lookup expired obligations: order API unavailable",
		},
	)

	for _, startupOnly := range []bool{false, true} {
		originName := "runtime-live"
		if startupOnly {
			originName = "startup-only"
		}
		for _, tc := range cases {
			t.Run(originName+"/"+tc.name, func(t *testing.T) {
				runSweepCharacterizationCase(t, tc, startupOnly, deadline, now, hash, txHash)
			})
		}
	}
}

func runSweepCharacterizationCase(
	t *testing.T,
	tc sweepCharacterizationCase,
	startupOnly bool,
	deadline, now time.Time,
	hash, txHash common.Hash,
) {
	t.Helper()
	terminals := make(map[common.Hash]orderTerminal)
	if tc.terminalPresent {
		terminals[hash] = tc.terminal
	}
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	trackingEntry, cfg := testExclusiveOrderEntry(t, executor)
	trackingEntry.OrderStatus = orderStatusExpired
	poller := &characterizationOrderPoller{terminals: terminals, err: tc.lookupErr}
	if startupOnly {
		poller.recent = []orderEntry{trackingEntry}
	}
	reader := &characterizationReceiptReader{
		times: map[common.Hash]time.Time{txHash: tc.filledAt}, err: tc.receiptErr,
	}
	log, logs := newCharacterizationLogger()
	metrics := &uniswapXMetrics{
		fills: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_uniswapx_fills_total"}, []string{"outcome"},
		),
		blockUntil: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_uniswapx_block_until"}),
	}
	failureAt := deadline.Add(-time.Minute)
	cfg.Breaker = BreakerConfig{Window: 15 * time.Minute}
	solver := &Solver{
		cfg: cfg, chainID: 1,
		orders: poller, reader: reader, confirmations: 3, log: log, metrics: metrics,
		failureTimes: []time.Time{failureAt},
	}
	solver.localBlockUntil.Store(321)
	solver.quoteEpoch.Store(41)
	quote := &quoteState{expiresAt: now.Add(time.Hour)}
	solver.quoteState.Store(quote)
	if startupOnly {
		if err := solver.recoverRecentExclusive(t.Context(), deadline.Add(-time.Minute)); err != nil {
			t.Fatalf("recoverRecentExclusive() error = %v", err)
		}
	} else {
		solver.trackExclusive(&resolvedOrder{
			Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
		}, deadline.Add(-time.Minute))
	}

	err := solver.sweepExclusive(t.Context(), now)
	assertSweepCharacterizationError(t, err, tc.wantError)
	assertSweepCharacterizationState(t, solver, hash, deadline, now, tc.wantOutcome)
	wantRuntimeMiss := tc.wantOutcome == sweepMissed && !startupOnly
	wantSettled := tc.wantOutcome == sweepSettled
	wantHistorical := tc.wantOutcome == sweepMissed && startupOnly
	assertSweepCharacterizationEffects(t, solver, metrics, quote, failureAt, now, wantSettled, wantRuntimeMiss)
	assertSweepReceiptCalls(t, reader, tc)
	assertExclusiveOutcomeLog(t, logs, hash, txHash, tc.terminal.Status, deadline, tc.filledAt, now,
		wantSettled, wantRuntimeMiss, wantHistorical)
}

func assertSweepCharacterizationError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" && err != nil {
		t.Fatalf("sweepExclusive() error = %v", err)
	}
	if want != "" && (err == nil || err.Error() != want) {
		t.Fatalf("sweepExclusive() error = %v, want exact %q", err, want)
	}
}

func assertSweepCharacterizationState(
	t *testing.T,
	solver *Solver,
	hash common.Hash,
	deadline, now time.Time,
	outcome sweepCharacterizationOutcome,
) {
	t.Helper()
	tracked, exists := solver.exclusiveState[hash]
	if !exists {
		t.Fatal("exclusive obligation disappeared")
	}
	if !tracked.deadline.Equal(deadline) {
		t.Fatalf("tracked obligation deadline = %v, want %v", tracked.deadline, deadline)
	}
	terminalOutcome := outcome != sweepUncertain
	if got := tracked.terminal(); got != terminalOutcome || tracked.pending() == terminalOutcome {
		t.Fatalf("tracked terminal/pending = %v/%v, want %v/%v", got, tracked.pending(), terminalOutcome, !terminalOutcome)
	}
	if terminalOutcome && !tracked.terminalAt.Equal(now) {
		t.Fatalf("terminalAt = %v, want %v", tracked.terminalAt, now)
	}
}

func assertSweepCharacterizationEffects(
	t *testing.T,
	solver *Solver,
	metrics *uniswapXMetrics,
	quote *quoteState,
	failureAt, now time.Time,
	settled, runtimeMiss bool,
) {
	t.Helper()
	wantBlockUntil := int64(0)
	if runtimeMiss {
		wantBlockUntil = now.Add(15 * time.Minute).Unix()
	}
	if got := solver.exclusiveBlockUntil.Load(); got != wantBlockUntil {
		t.Fatalf("exclusiveBlockUntil = %d, want %d", got, wantBlockUntil)
	}
	if solver.localBlockUntil.Load() != 321 || !reflect.DeepEqual(solver.failureTimes, []time.Time{failureAt}) {
		t.Fatalf("ordinary breaker history changed: until=%d failures=%v", solver.localBlockUntil.Load(), solver.failureTimes)
	}
	if runtimeMiss {
		if solver.quoteState.Load() != nil || solver.quoteEpoch.Load() != 42 {
			t.Fatalf("runtime miss quote state/epoch = %p/%d, want nil/42", solver.quoteState.Load(), solver.quoteEpoch.Load())
		}
	} else if solver.quoteState.Load() != quote || solver.quoteEpoch.Load() != 41 {
		t.Fatalf("non-runtime outcome changed quote state/epoch = %p/%d", solver.quoteState.Load(), solver.quoteEpoch.Load())
	}
	assertFillMetric(t, metrics, "exclusive-settled-in-time", settled)
	assertFillMetric(t, metrics, "missed-exclusive", runtimeMiss)
}

func assertFillMetric(t *testing.T, metrics *uniswapXMetrics, outcome string, incremented bool) {
	t.Helper()
	want := float64(0)
	if incremented {
		want = 1
	}
	if got := testutil.ToFloat64(metrics.fills.WithLabelValues(outcome)); got != want {
		t.Fatalf("%s metric = %v, want %v", outcome, got, want)
	}
}

func assertSweepReceiptCalls(
	t *testing.T,
	reader *characterizationReceiptReader,
	tc sweepCharacterizationCase,
) {
	t.Helper()
	wantCalls := 0
	if tc.terminalPresent && tc.terminal.Status == orderStatusFilled &&
		tc.terminal.TxHash != (common.Hash{}) && tc.lookupErr == nil {
		wantCalls = 1
	}
	calls, confirmations := reader.snapshot()
	if calls != wantCalls {
		t.Fatalf("receipt calls = %d, want %d", calls, wantCalls)
	}
	if wantCalls == 1 && !reflect.DeepEqual(confirmations, []uint64{3}) {
		t.Fatalf("receipt confirmations = %v, want [3]", confirmations)
	}
}

func assertExclusiveOutcomeLog(
	t *testing.T,
	logs *characterizationLogState,
	hash, txHash common.Hash,
	status string,
	deadline, filledAt, now time.Time,
	settled, runtimeMiss, historicalMiss bool,
) {
	t.Helper()
	type expectedLog struct {
		message string
		record  characterizationLogRecord
	}
	var expected *expectedLog
	switch {
	case settled:
		expected = &expectedLog{
			message: "exclusive order settled before exclusivity ended",
			record: characterizationLogRecord{
				level: "info", message: "exclusive order settled before exclusivity ended",
				fields: []any{
					"orderHash", hash.Hex(), "tx", txHash.Hex(), "filledAt", filledAt.Unix(),
					"exclusiveUntil", deadline.Unix(),
				},
			},
		}
	case runtimeMiss:
		fields := []any{
			"orderHash", hash.Hex(), "status", status, "exclusiveUntil", deadline.Unix(),
			"blockUntil", now.Add(15 * time.Minute).Unix(),
		}
		if txHash != (common.Hash{}) && status == orderStatusFilled {
			fields = append(fields, "tx", txHash.Hex(), "filledAt", filledAt.Unix())
		}
		expected = &expectedLog{
			message: "exclusive obligation missed",
			record: characterizationLogRecord{
				level: "error", message: "exclusive obligation missed",
				err: "exclusive fill missed decay start", fields: fields,
			},
		}
	case historicalMiss:
		fields := []any{
			"orderHash", hash.Hex(), "status", status, "exclusiveUntil", deadline.Unix(),
			"origin", "startup-recovery",
		}
		if txHash != (common.Hash{}) && status == orderStatusFilled {
			fields = append(fields, "tx", txHash.Hex(), "filledAt", filledAt.Unix())
		}
		expected = &expectedLog{
			message: "historical exclusive obligation missed",
			record: characterizationLogRecord{
				level: "info", message: "historical exclusive obligation missed", fields: fields,
			},
		}
	}
	outcomeMessages := []string{
		"exclusive order settled before exclusivity ended",
		"exclusive obligation missed",
		"historical exclusive obligation missed",
	}
	for _, message := range outcomeMessages {
		records := logs.recordsFor(message)
		if expected != nil && message == expected.message {
			if len(records) != 1 || !reflect.DeepEqual(records[0], expected.record) {
				t.Fatalf("%q records = %#v, want exact %#v", message, records, expected.record)
			}
		} else if len(records) != 0 {
			t.Fatalf("unexpected %q records: %#v", message, records)
		}
	}
}

func TestSweepExclusiveStrictDeadlineHasNoLookupOrEffect(t *testing.T) {
	deadline := time.Unix(2_000, 0)
	hash := common.HexToHash("0x1234")
	poller := &characterizationOrderPoller{terminals: map[common.Hash]orderTerminal{
		hash: {Status: orderStatusExpired},
	}}
	solver := &Solver{
		cfg:    &Config{Breaker: BreakerConfig{Window: time.Minute}},
		orders: poller, log: logr.Discard(),
	}
	solver.trackExclusive(&resolvedOrder{
		Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
	}, deadline.Add(-time.Minute))
	solver.quoteEpoch.Store(9)
	quote := &quoteState{expiresAt: deadline.Add(time.Hour)}
	solver.quoteState.Store(quote)

	if err := solver.sweepExclusive(t.Context(), deadline); err != nil {
		t.Fatal(err)
	}
	if poller.calls != 0 {
		t.Fatalf("terminal lookups = %d, want 0 at exact deadline", poller.calls)
	}
	if tracked := solver.exclusiveState[hash]; !tracked.pending() || tracked.terminal() {
		t.Fatalf("exact-deadline obligation changed: %+v", tracked)
	}
	if solver.exclusiveBlockUntil.Load() != 0 || solver.quoteState.Load() != quote || solver.quoteEpoch.Load() != 9 {
		t.Fatal("exact deadline changed breaker or quote state")
	}
}

func TestSweepExclusiveDecisionBatchIsAtomicOnLaterUncertainty(t *testing.T) {
	deadline := time.Unix(2_000, 0)
	now := deadline.Add(time.Second)
	hashes := []common.Hash{common.HexToHash("0x1001"), common.HexToHash("0x1002")}
	txHashes := []common.Hash{common.HexToHash("0x2001"), common.HexToHash("0x2002")}
	poller := &characterizationOrderPoller{terminals: map[common.Hash]orderTerminal{
		hashes[0]: {Status: orderStatusFilled, TxHash: txHashes[0]},
		hashes[1]: {Status: orderStatusFilled, TxHash: txHashes[1]},
	}}
	reader := &characterizationReceiptReader{
		times: map[common.Hash]time.Time{txHashes[0]: deadline, txHashes[1]: deadline},
		err:   errors.New("second receipt uncertain"), failAfter: 2,
	}
	solver := &Solver{
		cfg:    &Config{Breaker: BreakerConfig{Window: time.Minute}},
		orders: poller, reader: reader, log: logr.Discard(),
	}
	for _, hash := range hashes {
		solver.trackExclusive(&resolvedOrder{
			Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
		}, deadline.Add(-time.Minute))
	}
	quote := &quoteState{expiresAt: now.Add(time.Hour)}
	solver.quoteState.Store(quote)

	err := solver.sweepExclusive(t.Context(), now)
	if err == nil || !strings.Contains(err.Error(), "fill time: second receipt uncertain") {
		t.Fatalf("sweepExclusive() error = %v, want second receipt uncertainty", err)
	}
	calls, _ := reader.snapshot()
	if calls != 2 {
		t.Fatalf("receipt reads = %d, want 2 to prove a prior row classified", calls)
	}
	for _, hash := range hashes {
		if tracked := solver.exclusiveState[hash]; !tracked.pending() || tracked.terminal() {
			t.Fatalf("batch partially committed %s: %+v", hash.Hex(), tracked)
		}
	}
	if solver.exclusiveBlockUntil.Load() != 0 || solver.quoteState.Load() != quote {
		t.Fatal("uncertain batch changed breaker or quote state")
	}
}

func TestSweepExclusiveRevalidatesAfterUnlockedIO(t *testing.T) {
	deadline := time.Unix(2_000, 0)
	now := deadline.Add(time.Second)
	hash := common.HexToHash("0x1234")
	txHash := common.HexToHash("0xabcd")

	tests := []struct {
		name       string
		useReceipt bool
		mutate     func(*Solver)
		assert     func(*testing.T, *Solver)
	}{
		{
			name:   "record disappears during terminal lookup",
			mutate: func(s *Solver) { delete(s.exclusiveState, hash) },
			assert: func(t *testing.T, s *Solver) {
				t.Helper()
				if _, ok := s.exclusiveState[hash]; ok {
					t.Fatal("disappeared record was recreated")
				}
			},
		},
		{
			name: "record terminalizes during terminal lookup",
			mutate: func(s *Solver) {
				tracked := s.exclusiveState[hash]
				tracked.terminalAt = deadline.Add(500 * time.Millisecond)
				s.exclusiveState[hash] = tracked
			},
			assert: func(t *testing.T, s *Solver) {
				t.Helper()
				if got := s.exclusiveState[hash].terminalAt; !got.Equal(deadline.Add(500 * time.Millisecond)) {
					t.Fatalf("terminalAt overwritten = %v", got)
				}
			},
		},
		{
			name: "deadline changes during terminal lookup",
			mutate: func(s *Solver) {
				tracked := s.exclusiveState[hash]
				tracked.deadline = deadline.Add(time.Minute)
				s.exclusiveState[hash] = tracked
			},
			assert: func(t *testing.T, s *Solver) {
				t.Helper()
				tracked := s.exclusiveState[hash]
				if !tracked.pending() || !tracked.deadline.Equal(deadline.Add(time.Minute)) {
					t.Fatalf("changed deadline was not preserved: %+v", tracked)
				}
			},
		},
		{
			name: "record disappears during receipt read", useReceipt: true,
			mutate: func(s *Solver) { delete(s.exclusiveState, hash) },
			assert: func(t *testing.T, s *Solver) {
				t.Helper()
				if _, ok := s.exclusiveState[hash]; ok {
					t.Fatal("record removed during receipt I/O was recreated")
				}
			},
		},
		{
			name: "record terminalizes during receipt read", useReceipt: true,
			mutate: func(s *Solver) {
				tracked := s.exclusiveState[hash]
				tracked.terminalAt = deadline.Add(500 * time.Millisecond)
				s.exclusiveState[hash] = tracked
			},
			assert: func(t *testing.T, s *Solver) {
				t.Helper()
				if got := s.exclusiveState[hash].terminalAt; !got.Equal(deadline.Add(500 * time.Millisecond)) {
					t.Fatalf("terminalAt overwritten after receipt I/O = %v", got)
				}
			},
		},
		{
			name: "deadline changes during receipt read", useReceipt: true,
			mutate: func(s *Solver) {
				tracked := s.exclusiveState[hash]
				tracked.deadline = deadline.Add(time.Minute)
				s.exclusiveState[hash] = tracked
			},
			assert: func(t *testing.T, s *Solver) {
				t.Helper()
				tracked := s.exclusiveState[hash]
				if !tracked.pending() || !tracked.deadline.Equal(deadline.Add(time.Minute)) {
					t.Fatalf("changed deadline after receipt I/O was not preserved: %+v", tracked)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			terminal := orderTerminal{Status: orderStatusExpired}
			poller := &characterizationOrderPoller{terminals: map[common.Hash]orderTerminal{hash: terminal}}
			reader := &characterizationReceiptReader{times: map[common.Hash]time.Time{txHash: deadline}}
			if tc.useReceipt {
				poller.terminals[hash] = orderTerminal{Status: orderStatusFilled, TxHash: txHash}
				reader.started, reader.release = started, release
			} else {
				poller.started, poller.release = started, release
			}
			solver := &Solver{
				cfg:    &Config{Breaker: BreakerConfig{Window: time.Minute}},
				orders: poller, reader: reader, log: logr.Discard(),
			}
			solver.trackExclusive(&resolvedOrder{
				Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
			}, deadline.Add(-time.Minute))
			quote := &quoteState{expiresAt: now.Add(time.Hour)}
			solver.quoteState.Store(quote)
			done := make(chan error, 1)
			go func() { done <- solver.sweepExclusive(t.Context(), now) }()

			<-started
			solver.exclusiveMu.Lock()
			tc.mutate(solver)
			solver.exclusiveMu.Unlock()
			close(release)
			if err := <-done; err != nil {
				t.Fatalf("sweepExclusive() error = %v", err)
			}
			tc.assert(t, solver)
			if solver.exclusiveBlockUntil.Load() != 0 || solver.quoteState.Load() != quote {
				t.Fatal("stale I/O result changed breaker or quote state")
			}
		})
	}
}

type exclusiveTrackingCharacterization struct {
	solver   *Solver
	hash     common.Hash
	deadline time.Time
}

func TestExclusiveTrackingOriginDeadlineTerminalAndTTL(t *testing.T) {
	t.Run("later live observation overrides startup without replacing deadline", func(t *testing.T) {
		fixture := newExclusiveTrackingCharacterization(t, true, logr.Discard())
		solver, hash, deadline := fixture.solver, fixture.hash, fixture.deadline
		solver.trackExclusive(&resolvedOrder{
			Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Add(2 * time.Minute).Unix()),
		}, deadline.Add(-time.Minute))
		tracked := solver.exclusiveState[hash]
		if !tracked.deadline.Equal(deadline) || tracked.recoveredAtStart {
			t.Fatalf("later live observation state = %+v, want original deadline and runtime origin", tracked)
		}
		if err := solver.sweepExclusive(t.Context(), deadline.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if solver.exclusiveBlockUntil.Load() == 0 {
			t.Fatal("later live observation did not produce runtime miss effects")
		}
	})

	t.Run("earlier live deadline wins", func(t *testing.T) {
		fixture := newExclusiveTrackingCharacterization(t, true, logr.Discard())
		solver, hash, deadline := fixture.solver, fixture.hash, fixture.deadline
		earlier := deadline.Add(-30 * time.Second)
		solver.trackExclusive(&resolvedOrder{
			Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(earlier.Unix()),
		}, deadline.Add(-time.Minute))
		tracked := solver.exclusiveState[hash]
		if !tracked.deadline.Equal(earlier) || tracked.recoveredAtStart {
			t.Fatalf("earlier live observation state = %+v, want earlier deadline and runtime origin", tracked)
		}
	})

	t.Run("startup retry retains startup origin", func(t *testing.T) {
		log, logs := newCharacterizationLogger()
		fixture := newExclusiveTrackingCharacterization(t, true, log)
		solver, hash, deadline := fixture.solver, fixture.hash, fixture.deadline
		if err := solver.recoverRecentExclusive(t.Context(), deadline.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := solver.sweepExclusive(t.Context(), deadline.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if solver.exclusiveBlockUntil.Load() != 0 {
			t.Fatal("startup retry changed to runtime origin")
		}
		if records := logs.recordsFor("historical exclusive obligation missed"); len(records) != 1 {
			t.Fatalf("historical miss records = %d, want 1 for %s", len(records), hash.Hex())
		}
	})

	t.Run("terminal record cannot reopen", func(t *testing.T) {
		fixture := newExclusiveTrackingCharacterization(t, false, logr.Discard())
		solver, hash, deadline := fixture.solver, fixture.hash, fixture.deadline
		terminalAt := deadline.Add(time.Second)
		if err := solver.sweepExclusive(t.Context(), terminalAt); err != nil {
			t.Fatal(err)
		}
		terminal := solver.exclusiveState[hash]
		solver.trackExclusive(&resolvedOrder{
			Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Add(-time.Minute).Unix()),
		}, terminalAt)
		if got := solver.exclusiveState[hash]; !reflect.DeepEqual(got, terminal) {
			t.Fatalf("terminal record reopened: got %+v, want %+v", got, terminal)
		}
	})

	t.Run("terminal TTL retains exactly one hour and deletes only after", func(t *testing.T) {
		fixture := newExclusiveTrackingCharacterization(t, false, logr.Discard())
		solver, hash, deadline := fixture.solver, fixture.hash, fixture.deadline
		terminalAt := deadline.Add(time.Second)
		if err := solver.sweepExclusive(t.Context(), terminalAt); err != nil {
			t.Fatal(err)
		}
		solver.exclusiveMu.Lock()
		solver.cleanupExclusiveLocked(terminalAt.Add(time.Hour))
		_, retained := solver.exclusiveState[hash]
		solver.cleanupExclusiveLocked(terminalAt.Add(time.Hour + time.Nanosecond))
		_, deleted := solver.exclusiveState[hash]
		solver.exclusiveMu.Unlock()
		if !retained || deleted {
			t.Fatalf("TTL boundary retained/deleted = %v/%v, want true/false", retained, deleted)
		}
	})
}

func newExclusiveTrackingCharacterization(
	t *testing.T,
	startup bool,
	log logr.Logger,
) exclusiveTrackingCharacterization {
	t.Helper()
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	entry, cfg := testExclusiveOrderEntry(t, executor)
	entry.OrderStatus = orderStatusExpired
	hash := common.HexToHash(entry.OrderHash)
	deadline := time.Unix(1_000, 0)
	poller := &characterizationOrderPoller{
		terminals: map[common.Hash]orderTerminal{hash: {Status: orderStatusExpired}},
		recent:    []orderEntry{entry},
	}
	cfg.Breaker = BreakerConfig{Window: time.Minute}
	solver := &Solver{cfg: cfg, chainID: 1, orders: poller, log: log}
	if startup {
		if err := solver.recoverRecentExclusive(t.Context(), deadline.Add(-time.Minute)); err != nil {
			t.Fatalf("recoverRecentExclusive() error = %v", err)
		}
	} else {
		solver.trackExclusive(&resolvedOrder{
			Hash: hash, Source: orderSourceExclusiveV2, ExclusiveUntil: uint64(deadline.Unix()),
		}, deadline.Add(-time.Minute))
	}
	return exclusiveTrackingCharacterization{solver: solver, hash: hash, deadline: deadline}
}
