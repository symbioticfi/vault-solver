package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// rpcCodeError is a JSON-RPC error with a code, as the rpc client returns one.
type rpcCodeError struct {
	code    int
	message string
}

func (e rpcCodeError) Error() string  { return e.message }
func (e rpcCodeError) ErrorCode() int { return e.code }

// withoutNextBlockEstimates hides a backend's next-block estimate and pinned balance capabilities.
type withoutNextBlockEstimates struct{ Backend }

func TestHorizonGasEstimateModes(t *testing.T) {
	// A node without the pinned parent block yet: a lagging upstream behind the one the snapshot came from.
	notFound := rpcCodeError{code: -32000, message: "header not found"}
	for _, tc := range []struct {
		name          string
		cfg           func(cfg *Config)
		backend       func(b *horizonBackend) Backend
		verdict       blockOverridesSupport
		nextBlockErr  error
		nextBlockErrs []error
		wantGas       uint64
		wantMode      string
		wantErr       bool
		wantStaleHead bool // the failed estimate refuses the send as stale_head (NotAdmitted)
		wantErrorLog  bool // "gas estimation failed" is logged at Error
		wantNextCall  bool
		wantCounts    map[[2]string]float64 // {mode, outcome} -> count
	}{
		{
			name: "a confirmed next-block estimate", verdict: blockOverridesSupported,
			wantGas: 105_000, wantMode: estimateModeNextBlock, wantNextCall: true,
			wantCounts: map[[2]string]float64{{estimateModeNextBlock, gasEstimateOK}: 1},
		},
		{
			name: "mainnet headroom", verdict: blockOverridesSupported,
			cfg:     func(cfg *Config) { cfg.Gas.HeadroomBps = new(uint64(800)) },
			wantGas: 108_000, wantMode: estimateModeNextBlock, wantNextCall: true,
		},
		{
			// Until a probe confirms the overrides, an upstream that ignores them cannot be told apart. That
			// is not an upstream fault, so it is counted apart from the fallback the alert follows.
			name:    "unconfirmed overrides estimate at latest with the fallback headroom",
			wantGas: 99_000, wantMode: estimateModeUnconfirmed,
			wantCounts: map[[2]string]float64{
				{estimateModeUnconfirmed, gasEstimateOK}: 1, {estimateModeFallback, gasEstimateOK}: 0,
			},
		},
		{
			name: "overrides the probe found ignored", verdict: blockOverridesUnsupported,
			wantGas: 99_000, wantMode: estimateModeFallback,
			wantCounts: map[[2]string]float64{{estimateModeFallback, gasEstimateOK}: 1},
		},
		{
			name: "a per-call rejection falls back", verdict: blockOverridesSupported,
			nextBlockErr: rpcCodeError{code: -32602, message: "too many arguments, want at most 3"},
			wantGas:      99_000, wantMode: estimateModeFallback, wantNextCall: true,
			wantCounts: map[[2]string]float64{
				{estimateModeNextBlock, gasEstimateUnsupported}: 1, {estimateModeFallback, gasEstimateOK}: 1,
			},
		},
		{
			name: "a next-block revert fails the send", verdict: blockOverridesSupported,
			nextBlockErr: rpcCodeError{code: 3, message: "execution reverted"},
			wantErr:      true, wantErrorLog: true, wantNextCall: true,
			wantCounts: map[[2]string]float64{
				{estimateModeNextBlock, gasEstimateReverted}: 1, {estimateModeFallback, gasEstimateOK}: 0,
			},
		},
		{
			// The estimate is pinned to the snapshot's header, like the balance read, and is retried the same
			// way when the upstream it lands on has not imported that block yet.
			name: "a lagging upstream is retried until it has the parent block", verdict: blockOverridesSupported,
			nextBlockErrs: []error{notFound, notFound},
			wantGas:       105_000, wantMode: estimateModeNextBlock, wantNextCall: true,
			wantCounts: map[[2]string]float64{
				{estimateModeNextBlock, gasEstimateOK}: 1, {estimateModeNextBlock, gasEstimateBlockNotFound}: 0,
				{estimateModeFallback, gasEstimateOK}: 0,
			},
		},
		{
			name: "a parent block that never arrives refuses the send as a stale head", verdict: blockOverridesSupported,
			cfg:          func(cfg *Config) { cfg.Gas.EstimateTimeout = 30 * time.Millisecond },
			nextBlockErr: notFound,
			wantErr:      true, wantStaleHead: true, wantNextCall: true,
			wantCounts: map[[2]string]float64{
				{estimateModeNextBlock, gasEstimateBlockNotFound}: 1, {estimateModeNextBlock, gasEstimateError}: 0,
				{estimateModeFallback, gasEstimateOK}: 0,
			},
		},
		{
			name: "next-block estimates off", verdict: blockOverridesSupported,
			cfg:     func(cfg *Config) { cfg.Gas.NextBlockEstimateDisabled = true },
			wantGas: 94_500, wantMode: estimateModeLatest,
			wantCounts: map[[2]string]float64{{estimateModeLatest, gasEstimateOK}: 1},
		},
		{
			name: "a backend without the capability", verdict: blockOverridesSupported,
			backend: func(b *horizonBackend) Backend { return withoutNextBlockEstimates{b} },
			wantGas: 99_000, wantMode: estimateModeFallback,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newHorizonBackend(big.NewInt(1e18))
			b.nextBlockErr, b.nextBlockErrs = tc.nextBlockErr, tc.nextBlockErrs
			var backend Backend = b
			if tc.backend != nil {
				backend = tc.backend(b)
			}
			cfg := horizonConfig()
			if tc.cfg != nil {
				tc.cfg(&cfg)
			}
			metrics := newTestMetrics(t)
			logs, log := newLogCapture(0)
			var mu sync.Mutex
			m := NewWithMetrics(backend, mustSigner(t), big.NewInt(11155111), cfg, metrics,
				logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
			m.overrides.swap(tc.verdict)
			pending, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(0))
			if tc.wantErr {
				if err == nil || pending != nil || len(b.attemptedTransactions()) != 0 {
					t.Fatalf("broadcast = (%v, %v), want a failed estimate and nothing sent", pending, err)
				}
				if reason, refused := guardRefusalReason(err); refused != tc.wantStaleHead ||
					(refused && reason != admissionRejectionStaleHead) {
					t.Fatalf("broadcast error %v refused as %q (%t), want stale_head %t", err, reason, refused, tc.wantStaleHead)
				}
			} else {
				if err != nil {
					t.Fatalf("broadcast: %v", err)
				}
				if pending.gas != tc.wantGas {
					t.Fatalf("gas limit = %d, want %d", pending.gas, tc.wantGas)
				}
			}
			calls := b.nextBlockEstimates()
			if (len(calls) > 0) != tc.wantNextCall {
				t.Fatalf("next-block estimates = %d, want called %t", len(calls), tc.wantNextCall)
			}
			for _, call := range calls {
				// On top of the snapshot's header 100: block 101, one 12 s block time later.
				if call.parent.Uint64() != 100 || call.overrides.Number.Uint64() != 101 {
					t.Fatalf("next-block estimate on parent %s with number %s, want 100 and 101", call.parent, call.overrides.Number)
				}
				if call.overrides.Time < uint64(time.Now().Unix())+12 {
					t.Fatalf("next-block estimate time %d, want the header time plus 12 s", call.overrides.Time)
				}
			}
			for key, want := range tc.wantCounts {
				assertMetric(t, metrics.gasEstimates.WithLabelValues("fill", key[0], key[1]), want)
			}
			mu.Lock()
			defer mu.Unlock()
			if errorLevel, _ := countLogs(*logs, "gas estimation failed"); (errorLevel > 0) != tc.wantErrorLog {
				t.Fatalf("gas estimation failed logged %d times at Error, want logged %t: %s",
					errorLevel, tc.wantErrorLog, strings.Join(*logs, "\n"))
			}
		})
	}
}

func TestHorizonGasEstimateTimeout(t *testing.T) {
	b := newHorizonBackend(big.NewInt(1e18))
	b.blockEstimates = make(chan struct{})
	cfg := horizonConfig()
	cfg.Gas.EstimateTimeout = 20 * time.Millisecond
	metrics := newTestMetrics(t)
	m := newHorizonManager(t, b, cfg, metrics)
	started := time.Now()
	if _, err := m.broadcast(managerCtx(t.Context(), m), fillRequest(0)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("broadcast error = %v, want the estimate deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("estimate took %s despite a 20 ms budget", elapsed)
	}
	assertMetric(t, metrics.gasEstimates.WithLabelValues("fill", estimateModeUnconfirmed, gasEstimateError), 1)
}

func TestProbeBlockOverrides(t *testing.T) {
	for _, tc := range []struct {
		name        string
		result      bool
		err         error
		before      blockOverridesSupport
		wantVerdict blockOverridesSupport
		wantDelay   time.Duration
	}{
		{name: "honoured", result: true, wantVerdict: blockOverridesSupported, wantDelay: blockOverridesProbeInterval},
		{name: "ignored or rejected", before: blockOverridesSupported, wantVerdict: blockOverridesUnsupported, wantDelay: blockOverridesProbeInterval},
		{
			name: "inconclusive keeps the verdict", err: errors.New("header unavailable"), before: blockOverridesSupported,
			wantVerdict: blockOverridesSupported, wantDelay: blockOverridesProbeRetry,
		},
		{name: "inconclusive at startup stays unknown", err: errors.New("header unavailable"), wantDelay: blockOverridesProbeRetry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newHorizonBackend(big.NewInt(1e18))
			b.probeResult, b.probeErr = tc.result, tc.err
			m := newHorizonManager(t, b, horizonConfig(), nil)
			m.overrides.swap(tc.before)
			if delay := m.probeBlockOverridesOnce(managerCtx(t.Context(), m)); delay != tc.wantDelay {
				t.Fatalf("next probe in %s, want %s", delay, tc.wantDelay)
			}
			if got := m.overrides.load(); got != tc.wantVerdict {
				t.Fatalf("verdict = %d, want %d", got, tc.wantVerdict)
			}
		})
	}
}

func TestStartProbesBlockOverridesUnderHorizonOnly(t *testing.T) {
	t.Run("horizon", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		m := newHorizonManager(t, b, horizonConfig(), nil)
		startManagerForTest(t, m)
		deadline := time.Now().Add(5 * time.Second)
		for m.overrides.load() != blockOverridesSupported {
			if time.Now().After(deadline) {
				t.Fatal("the startup probe did not confirm block overrides")
			}
			time.Sleep(time.Millisecond)
		}
		if res := m.Send(t.Context(), fillRequest(0)); res.Err != nil {
			t.Fatalf("send: %v", res.Err)
		}
		if calls := b.nextBlockEstimates(); len(calls) != 1 {
			t.Fatalf("next-block estimates = %d after the probe confirmed them, want 1", len(calls))
		}
	})
	t.Run("legacy", func(t *testing.T) {
		b := newHorizonBackend(big.NewInt(1e18))
		m := New(b, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
		startManagerForTest(t, m)
		if res := m.Send(t.Context(), fillRequest(21_000)); res.Err != nil {
			t.Fatalf("send: %v", res.Err)
		}
		if calls := b.probeCalls.Load(); calls != 0 || m.nextBlock != nil {
			t.Fatalf("legacy policy probed %d times (next-block estimator %v)", calls, m.nextBlock)
		}
	})
}
