package txmanager

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

func TestInitialFeeCeilingsAreTypedAndCountedOnlyAtSubmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		global  float64
		request *big.Int
	}{
		{name: "market above global cap", global: 10},
		{name: "market above request cap", global: 100, request: gweiToWei(10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newMockBackend()
			metrics := newTestMetrics(t)
			logs, log := newLogCapture(1)
			m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: tc.global}, metrics, log)
			_, _ = m.MaxFeePerGas(t.Context()) // Profitability reads are not rejected submissions.
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseInitial), 0)
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
				Label: "rfq-fill", To: common.Address{1}, GasLimit: 21_000, MaxFeePerGas: tc.request,
			})
			if pending != nil || !errors.Is(err, ErrFeeLimitReached) || b.sendCalls != 0 {
				t.Fatalf("fee ceiling signed or sent a transaction: pending=%+v err=%v sends=%d", pending, err, b.sendCalls)
			}
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseInitial), 1)
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseReplacement), 0)
			if errorLevel, info := countLogs(*logs, "transaction fee limit reached"); errorLevel != 0 || info != 1 {
				t.Fatalf("expected cap rejection Info, got Error=%d Info=%d", errorLevel, info)
			}
		})
	}
}

func TestReplacementFeeCeilingsAreCountedWithoutFailureAlerts(t *testing.T) {
	for _, marketAboveCap := range []bool{false, true} {
		name := "required fee bump exceeds cap"
		if marketAboveCap {
			name = "market base fee exceeds cap"
		}
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			metrics := newTestMetrics(t)
			logs, log := newLogCapture(1)
			m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, metrics, log)
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{
				Label: "rfq-fill", To: common.Address{1}, GasLimit: 21_000,
			})
			if err != nil {
				t.Fatal(err)
			}
			pending.req.MaxFeePerGas = new(big.Int).Set(pending.fees.maxFee)
			if marketAboveCap {
				b.mine(gweiToWei(200))
			}
			for range 2 {
				delete(b.receipts, pending.originalHash)
				deadline, err := m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{})
				if deadline || err != nil {
					t.Fatalf("capped tracking did not retain ordinary rebroadcast: deadline=%v err=%v", deadline, err)
				}
			}
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseInitial), 0)
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseReplacement), 2)
			if len(pending.attempts) != 1 || b.sendCalls != 3 {
				t.Fatalf("ceiling changed the signed transaction instead of exact rebroadcast: attempts=%d sends=%d", len(pending.attempts), b.sendCalls)
			}
			if errorLevel, info := countLogs(*logs, "replacement fee limit reached"); errorLevel != 0 || info != 2 {
				t.Fatalf("expected replacement ceilings Info, got Error=%d Info=%d", errorLevel, info)
			}
			if errorLevel, _ := countLogs(*logs, "cannot replace pending transaction"); errorLevel != 0 {
				t.Fatal("expected fee ceiling emitted a replacement failure alert")
			}
		})
	}
}

func TestBroadcastTransportErrorDoesNotBecomeFeeCeiling(t *testing.T) {
	b := newMockBackend()
	cause := errors.New("RPC connection reset")
	b.sendErrs = []error{cause}
	metrics := newTestMetrics(t)
	logs, log := newLogCapture(1)
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, metrics, log)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{Label: "rfq-fill", To: common.Address{1}, GasLimit: 21_000})
	if pending == nil || err != nil {
		t.Fatalf("transport ambiguity did not retain signed tracking: pending=%+v err=%v", pending, err)
	}
	assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseInitial), 0)
	if errorLevel, _ := countLogs(*logs, "transaction broadcast uncertain; tracking signed hash"); errorLevel != 1 {
		t.Fatal("transport failure stopped paging")
	}
}

type feeLimitEstimateTransportBackend struct {
	*mockBackend

	cause error
}

func (b *feeLimitEstimateTransportBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 0, b.cause
}

func (b *feeLimitEstimateTransportBackend) EstimateGasNextBlock(
	context.Context, ethereum.CallMsg, *types.Header, time.Duration,
) (uint64, error) {
	return 0, b.cause
}

func TestEstimateTransportErrorDoesNotBecomeFeeCeiling(t *testing.T) {
	cause := errors.New("gas estimate RPC connection reset")
	b := &feeLimitEstimateTransportBackend{mockBackend: newMockBackend(), cause: cause}
	metrics := newTestMetrics(t)
	logs, log := newLogCapture(1)
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, metrics, log)
	pending, err := m.broadcast(managerCtx(t.Context(), m), Request{Label: "rfq-fill", To: common.Address{1}})
	if pending != nil || !errors.Is(err, cause) || errors.Is(err, ErrFeeLimitReached) {
		t.Fatalf("estimate transport failure lost its error category: pending=%+v err=%v", pending, err)
	}
	assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseInitial), 0)
	if errorLevel, _ := countLogs(*logs, "gas estimation failed"); errorLevel != 1 {
		t.Fatal("estimate transport failure stopped paging")
	}
}

func TestCappedRebroadcastKnownAcknowledgementStaysInfo(t *testing.T) {
	for _, known := range []bool{true, false} {
		name := "transport failure"
		cause := errors.New("RPC connection reset")
		if known {
			name, cause = "already known", errors.New("already known")
		}
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			b.sendErrs = []error{nil, cause}
			metrics := newTestMetrics(t)
			logs, log := newLogCapture(1)
			m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, metrics, log)
			pending, err := m.broadcast(managerCtx(t.Context(), m), Request{Label: "rfq-fill", To: common.Address{1}, GasLimit: 21_000})
			if err != nil {
				t.Fatal(err)
			}
			pending.req.MaxFeePerGas = new(big.Int).Set(pending.fees.maxFee)
			delete(b.receipts, pending.originalHash)
			if deadline, err := m.tryReplace(managerCtx(t.Context(), m), pending, replaceIntent{}); deadline || err != nil {
				t.Fatalf("cap rebroadcast stopped ordinary tracking: deadline=%v err=%v", deadline, err)
			}
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseReplacement), 1)
			wantError := 1
			if known {
				wantError = 0
				assertMetric(t, metrics.replacements.WithLabelValues("rfq-fill", replacementKindRebroadcast, rebroadcastReasonCapped), 1)
			}
			if errorLevel, _ := countLogs(*logs, "capped transaction rebroadcast failed"); errorLevel != wantError {
				t.Fatalf("known cap acknowledgement paged or transport stopped paging: got %d errors, want %d", errorLevel, wantError)
			}
			if known {
				if errorLevel, info := countLogs(*logs, "capped transaction already known by write RPC"); errorLevel != 0 || info != 1 {
					t.Fatalf("known cap acknowledgement not reported at Info: errors=%d info=%d", errorLevel, info)
				}
			}
		})
	}
}
