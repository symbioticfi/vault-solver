package txmanager

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestMetrics(t *testing.T) {
	const effectiveGasPriceWei = int64(2_000_000_000)
	submissionErrorBackend := &metricsReceiptBackend{mockBackend: newMockBackend()}
	submissionErrorBackend.sendErrs = []error{errors.New("insufficient funds")}
	for _, test := range []struct {
		name       string
		label      string
		backend    Backend
		outcome    Outcome
		wantPhases []lifecyclePhase
		wantFeeWei float64
	}{
		{
			"confirmed", "rfq-fill",
			&metricsReceiptBackend{
				mockBackend: newMockBackend(), effectiveGasPrice: big.NewInt(effectiveGasPriceWei),
			},
			OutcomeConfirmed,
			[]lifecyclePhase{
				lifecyclePhasePrebroadcast, lifecyclePhasePending, lifecyclePhaseConfirming,
			},
			float64(21_000 * effectiveGasPriceWei),
		},
		{
			"reverted", "uniswapx-fill",
			&metricsReceiptBackend{
				mockBackend: newMockBackend(), reverted: true,
				effectiveGasPrice: big.NewInt(effectiveGasPriceWei),
			},
			OutcomeReverted,
			[]lifecyclePhase{
				lifecyclePhasePrebroadcast, lifecyclePhasePending, lifecyclePhaseConfirming,
			},
			float64(21_000 * effectiveGasPriceWei),
		},
		{
			"submission error", "lifi-fill", submissionErrorBackend, OutcomeSubmissionError,
			[]lifecyclePhase{lifecyclePhasePrebroadcast}, 0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			metrics := newTestMetrics(t)
			manager := startTestManager(t, test.backend, Config{}, metrics)
			result := manager.Send(t.Context(), Request{
				To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: test.label,
			})
			if result.Outcome != test.outcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome, test.outcome)
			}
			assertMetric(t, metrics.requests.WithLabelValues(test.label, string(test.outcome)), 1)
			assertMetric(t, metrics.inflight.WithLabelValues(test.label), 0)
			assertHistogramObservedOnce(t, metrics.lifecycleDuration.WithLabelValues(
				test.label,
				string(test.outcome),
			))
			assertHistogramObservedOnce(t, metrics.admissionWait.WithLabelValues(
				test.label,
				string(admissionOutcomeAdmitted),
			))
			for _, phase := range test.wantPhases {
				assertHistogramObservedOnce(t, metrics.phaseDuration.WithLabelValues(
					test.label,
					phase.label(),
					string(test.outcome),
				))
			}
			if got := testutil.CollectAndCount(metrics.phaseDuration); got != len(test.wantPhases) {
				t.Fatalf("phase duration series = %d, want %d", got, len(test.wantPhases))
			}
			wantGas := float64(21_000)
			if test.outcome == OutcomeSubmissionError {
				wantGas = 0
			}
			assertMetric(t, metrics.gasUsed.WithLabelValues(test.label, string(test.outcome)), wantGas)
			assertMetric(
				t,
				metrics.feePaidWei.WithLabelValues(test.label, string(test.outcome)),
				test.wantFeeWei,
			)
		})
	}

	t.Run("included unconfirmed", func(t *testing.T) {
		backend := &cancelOnConfirmationHeadBackend{mockBackend: newMockBackend()}
		metrics := newTestMetrics(t)
		manager := NewWithMetrics(
			backend, mustSigner(t), big.NewInt(11155111),
			Config{Confirmations: 2, PollInterval: time.Millisecond}, metrics, logr.Discard(),
		)
		request := Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "lifi-fill",
		}
		pending, err := manager.broadcast(t.Context(), request)
		if err != nil {
			t.Fatalf("broadcast: %v", err)
		}
		result := make(chan Result, 1)
		pending.result = result
		manager.trackUnminedTransaction(pending)
		pending.lifecycle = metrics.beginLifecycle(request.Label)
		pending.lifecycle.transitionPhase(lifecyclePhasePending)

		lifecycleCtx, cancel := context.WithCancelCause(t.Context())
		backend.cancel = func() { cancel(context.Canceled) }
		backend.armed = true
		manager.complete(lifecycleCtx, pending)
		completed := <-result
		if completed.Outcome != OutcomeIncludedUnconfirmed {
			t.Fatalf("outcome = %q, want %q", completed.Outcome, OutcomeIncludedUnconfirmed)
		}
		assertMetric(t, metrics.requests.WithLabelValues(
			"lifi-fill",
			string(OutcomeIncludedUnconfirmed),
		), 1)
	})

	t.Run("replacement lifecycle", func(t *testing.T) {
		sgnr := mustSigner(t)
		backend := &replacementBackend{mockBackend: newMockBackend(), cancellationTo: sgnr.Address()}
		metrics := newTestMetrics(t)
		manager := NewWithMetrics(
			backend, sgnr, big.NewInt(11155111),
			Config{
				MaxFeeGwei:          100,
				PollInterval:        time.Millisecond,
				ReplacementInterval: 2 * time.Millisecond,
				PendingTimeout:      8 * time.Millisecond,
			},
			metrics,
			logr.Discard(),
		)
		go manager.Start(t.Context())

		result, accepted := manager.SendAsync(t.Context(), Request{
			To: common.HexToAddress("0xabc"), Data: []byte{1}, GasLimit: 21_000,
			MaxFeePerGas: big.NewInt(42_000_000_000), Label: "lifi-fill",
		})
		if !accepted {
			t.Fatal("transaction was not accepted")
		}
		if completed := <-result; completed.Outcome != OutcomeCancelled {
			t.Fatalf("outcome = %q, want %q", completed.Outcome, OutcomeCancelled)
		}
		assertMetric(t, metrics.replacements.WithLabelValues(
			"lifi-fill",
			replacementKindReplacement,
		), 1)
		assertMetric(t, metrics.replacements.WithLabelValues(
			"lifi-fill",
			replacementKindCancellation,
		), 1)
	})

	t.Run("tracking stopped", func(t *testing.T) {
		backend := &pendingMetricsBackend{mockBackend: newMockBackend()}
		metrics := newTestMetrics(t)
		ctx, cancel := context.WithCancel(context.Background())
		manager := NewWithMetrics(
			backend, mustSigner(t), big.NewInt(11155111),
			Config{
				PollInterval:        time.Millisecond,
				ReplacementInterval: time.Millisecond,
				ShutdownTimeout:     20 * time.Millisecond,
			}, metrics, logr.Discard(),
		)
		managerDone := make(chan struct{})
		go func() {
			manager.Start(ctx)
			close(managerDone)
		}()

		result, accepted := manager.SendAsync(ctx, Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill",
		})
		if !accepted {
			t.Fatal("transaction was not accepted")
		}
		waitForSentTransactions(t, backend.mockBackend, 1)
		assertMetric(t, metrics.inflight.WithLabelValues("rfq-fill"), 1)

		cancel()
		completed := <-result
		if completed.Outcome != OutcomeTrackingStopped {
			t.Fatalf("outcome = %q, want %q", completed.Outcome, OutcomeTrackingStopped)
		}
		<-managerDone
		manager.lifecycleWG.Wait()
		assertMetric(t, metrics.requests.WithLabelValues("rfq-fill", string(OutcomeTrackingStopped)), 1)
		assertMetric(t, metrics.inflight.WithLabelValues("rfq-fill"), 0)
		if got := testutil.CollectAndCount(metrics.phaseDuration); got != 2 {
			t.Fatalf("phase duration series = %d, want 2", got)
		}
	})
}

func TestReceiptFeePaidWei(t *testing.T) {
	for _, test := range []struct {
		name    string
		receipt *types.Receipt
		want    float64
		wantOK  bool
	}{
		{name: "missing receipt"},
		{name: "missing effective price", receipt: &types.Receipt{GasUsed: 21_000}},
		{
			name: "negative effective price",
			receipt: &types.Receipt{
				GasUsed: 21_000, EffectiveGasPrice: big.NewInt(-1),
			},
		},
		{
			name: "actual receipt fee",
			receipt: &types.Receipt{
				GasUsed: 21_000, EffectiveGasPrice: big.NewInt(2_000_000_000),
			},
			want: 42_000_000_000_000, wantOK: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := receiptFeePaidWei(test.receipt)
			if got != test.want || ok != test.wantOK {
				t.Fatalf("receiptFeePaidWei() = (%v, %t), want (%v, %t)", got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestUntrustedReconciliationReceiptKeepsPendingPhase(t *testing.T) {
	backend := &transientHeadErrorBackend{mockBackend: newMockBackend()}
	metrics := newTestMetrics(t)
	manager := NewWithMetrics(
		backend,
		mustSigner(t),
		big.NewInt(11155111),
		Config{},
		metrics,
		logr.Discard(),
	)
	pending, err := manager.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "canonical-reconciliation",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	pending.lifecycle = metrics.beginLifecycle(pending.req.Label)
	pending.lifecycle.transitionPhase(lifecyclePhasePending)
	pending.nonceConflictHash = pending.attempts[0].hash
	manager.markNonceConflict(pending.nonce, pending.nonceConflictHash)
	backend.errorMu.Lock()
	backend.blockFailures = 1
	backend.errorMu.Unlock()

	if result, done := manager.receiptResult(t.Context(), pending); done {
		t.Fatalf("untrusted reconciliation receipt completed lifecycle: %+v", result)
	}
	if pending.lifecycle.phase != lifecyclePhasePending {
		t.Fatalf("phase = %q, want pending", pending.lifecycle.phase.label())
	}
	if pending.lifecycle.phaseObserved[lifecyclePhaseConfirming] {
		t.Fatal("untrusted reconciliation receipt recorded confirming phase")
	}

	pending.lifecycle.finish(OutcomeTrackingStopped, nil)
	if got := testutil.CollectAndCount(metrics.phaseDuration); got != 2 {
		t.Fatalf("phase duration series = %d, want prebroadcast and pending only", got)
	}
}

func TestPhaseDurationAccumulatesAcrossReceiptReorg(t *testing.T) {
	backend := newMockBackend()
	reorgBackend := &disappearingReceiptBackend{mockBackend: backend}
	metrics := newTestMetrics(t)
	manager := NewWithMetrics(
		reorgBackend,
		mustSigner(t),
		big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond},
		metrics,
		logr.Discard(),
	)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	backend.receipts[tx.Hash()] = successfulReceipt(tx, backend.head-2)
	pending := &pendingTransaction{
		req:      Request{Label: "reorged-phase"},
		nonce:    tx.Nonce(),
		attempts: []txAttempt{{hash: tx.Hash(), tx: tx}},
	}
	pending.lifecycle = metrics.beginLifecycle(pending.req.Label)
	pending.lifecycle.transitionPhase(lifecyclePhasePending)

	if result, done := manager.receiptResult(t.Context(), pending); done {
		t.Fatalf("reorged receipt completed lifecycle: %+v", result)
	}
	if pending.lifecycle.phase != lifecyclePhasePending {
		t.Fatalf("phase after reorg = %q, want pending", pending.lifecycle.phase.label())
	}
	firstConfirmingDuration := pending.lifecycle.phaseDurations[lifecyclePhaseConfirming]

	// The next receipt remains canonical, so the same lifecycle enters confirming a second time.
	manager.backend = backend
	result, done := manager.receiptResult(t.Context(), pending)
	if !done || result.Outcome != OutcomeConfirmed {
		t.Fatalf("canonical receipt result = (%+v, %t), want confirmed terminal result", result, done)
	}
	if pending.lifecycle.phase != lifecyclePhaseConfirming {
		t.Fatalf("terminal phase = %q, want confirming", pending.lifecycle.phase.label())
	}
	time.Sleep(time.Millisecond)
	pending.lifecycle.finish(result.Outcome, result.Receipt)

	if got := pending.lifecycle.phaseDurations[lifecyclePhaseConfirming]; got <= firstConfirmingDuration {
		t.Fatalf(
			"cumulative confirming duration = %s, want more than first episode %s",
			got,
			firstConfirmingDuration,
		)
	}
	for phase := range lifecyclePhaseCount {
		assertHistogramDuration(t, metrics.phaseDuration.WithLabelValues(
			pending.req.Label,
			phase.label(),
			string(result.Outcome),
		), pending.lifecycle.phaseDurations[phase])
	}
}

func TestAdmissionRejectionMetrics(t *testing.T) {
	t.Run("bounded reasons", func(t *testing.T) {
		for _, test := range []struct {
			name string
			err  error
			want admissionRejectionReason
		}{
			{"manager stopped", errManagerStopped, admissionRejectionManagerStopped},
			{"nonce conflict", errNonceLanePaused, admissionRejectionNonceConflict},
			{"deadline", context.DeadlineExceeded, admissionRejectionDeadline},
			{"caller cancelled", context.Canceled, admissionRejectionCallerCancelled},
			{"other", errors.New("unexpected"), admissionRejectionOther},
		} {
			t.Run(test.name, func(t *testing.T) {
				metrics := newTestMetrics(t)
				metrics.finishAdmission("test", time.Now(), test.err)
				assertMetric(t, metrics.admissionRejections.WithLabelValues("test", string(test.want)), 1)
				assertHistogramObservedOnce(t, metrics.admissionWait.WithLabelValues("test", string(test.want)))
			})
		}
	})

	t.Run("terminal failures before worker lifecycle", func(t *testing.T) {
		metrics := newTestMetrics(t)
		manager := NewWithMetrics(
			newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, metrics, logr.Discard(),
		)
		result, accepted := manager.SendAsync(t.Context(), Request{
			To:       common.HexToAddress("0xabc"),
			CancelAt: time.Now().Add(-time.Second),
			Label:    "expired",
		})
		if !accepted {
			t.Fatal("request deadline did not return its terminal admission result")
		}
		if completed := <-result; !completed.NotAdmitted {
			t.Fatalf("result = %+v, want NotAdmitted", completed)
		}
		assertMetric(t, metrics.admissionRejections.WithLabelValues(
			"expired",
			string(admissionRejectionDeadline),
		), 1)
		assertHistogramObservedOnce(t, metrics.admissionWait.WithLabelValues(
			"expired",
			string(admissionRejectionDeadline),
		))
		if got := testutil.CollectAndCount(metrics.lifecycleDuration); got != 0 {
			t.Fatalf("pre-admission failure recorded %d lifecycle series, want 0", got)
		}
	})
}

type metricsReceiptBackend struct {
	*mockBackend

	reverted          bool
	effectiveGasPrice *big.Int
}

func (b *metricsReceiptBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := b.mockBackend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	receipt := b.receipts[tx.Hash()]
	if b.reverted {
		receipt.Status = types.ReceiptStatusFailed
	}
	if b.effectiveGasPrice != nil {
		receipt.EffectiveGasPrice = new(big.Int).Set(b.effectiveGasPrice)
	}
	return nil
}

type pendingMetricsBackend struct {
	*mockBackend
}

func (b *pendingMetricsBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := b.mockBackend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.mu.Lock()
	delete(b.receipts, tx.Hash())
	b.mu.Unlock()
	return nil
}

func startTestManager(t *testing.T, backend Backend, cfg Config, metrics *Metrics) *Manager {
	t.Helper()
	cfg.PollInterval = time.Millisecond
	manager := NewWithMetrics(backend, mustSigner(t), big.NewInt(11155111), cfg, metrics, logr.Discard())
	go manager.Start(t.Context())
	return manager
}

func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	return metrics
}

func assertMetric(t *testing.T, collector prometheus.Collector, want float64) {
	t.Helper()
	if got := testutil.ToFloat64(collector); got != want {
		t.Fatalf("metric = %v, want %v", got, want)
	}
}

func assertHistogramObservedOnce(t *testing.T, observer prometheus.Observer) {
	t.Helper()
	if got := metricstest.HistogramCount(t, observer); got != 1 {
		t.Fatalf("histogram sample count = %d, want 1", got)
	}
}

func assertHistogramDuration(t *testing.T, observer prometheus.Observer, want time.Duration) {
	t.Helper()
	metricstest.RequireHistogram(t, observer, 1, want.Seconds())
}
