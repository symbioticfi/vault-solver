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
)

func TestMetrics(t *testing.T) {
	submissionErrorBackend := newMockBackend()
	submissionErrorBackend.sendErrs = []error{errors.New("send failed")}
	for _, test := range []struct {
		name    string
		label   string
		backend Backend
		outcome Outcome
	}{
		{"confirmed", "rfq-fill", newMockBackend(), OutcomeConfirmed},
		{"reverted", "uniswapx-fill", &revertingBackend{mockBackend: newMockBackend()}, OutcomeReverted},
		{"submission error", "lifi-fill", submissionErrorBackend, OutcomeSubmissionError},
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
			wantGas := float64(21_000)
			if test.outcome == OutcomeSubmissionError {
				wantGas = 0
			}
			assertMetric(t, metrics.gasUsed.WithLabelValues(test.label, string(test.outcome)), wantGas)
		})
	}

	t.Run("included unconfirmed", func(t *testing.T) {
		backend := newMockBackend()
		metrics := newTestMetrics(t)
		ctx, cancel := context.WithCancel(context.Background())
		manager := NewWithMetrics(
			backend, mustSigner(t), big.NewInt(11155111),
			Config{Confirmations: 2, PollInterval: time.Millisecond}, metrics, logr.Discard(),
		)
		go manager.Start(ctx)

		result, accepted := manager.SendAsync(ctx, Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "lifi-fill",
		})
		if !accepted {
			t.Fatal("transaction was not accepted")
		}
		waitForSentTransactions(t, backend, 1)
		cancel()
		completed := <-result
		if completed.Outcome != OutcomeIncludedUnconfirmed {
			t.Fatalf("outcome = %q, want %q", completed.Outcome, OutcomeIncludedUnconfirmed)
		}
		assertMetric(t, metrics.requests.WithLabelValues(
			"lifi-fill",
			string(OutcomeIncludedUnconfirmed),
		), 1)
	})

	t.Run("replacement", func(t *testing.T) {
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
			Config{PollInterval: time.Millisecond}, metrics, logr.Discard(),
		)
		go manager.Start(ctx)

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
		assertMetric(t, metrics.requests.WithLabelValues(
			"rfq-fill",
			string(OutcomeTrackingStopped),
		), 1)
		assertMetric(t, metrics.inflight.WithLabelValues("rfq-fill"), 0)
	})
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
