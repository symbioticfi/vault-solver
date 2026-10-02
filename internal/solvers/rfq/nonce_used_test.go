package rfq

import (
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func nonceUsedResult(data []byte) txmanager.Result {
	return txmanager.Result{Outcome: txmanager.OutcomeSubmissionError,
		Err: errors.Errorf("estimate: %w", &txmanager.ExecutionRevertError{Data: data, Cause: errors.New("execution reverted")})}
}

func TestExecutionNonceUsedRetiresSendingBeforeBackendCatchesUp(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: nonceUsedResult([]byte{0x1f, 0x6d, 0x5a, 0xef})}
	e := newExec(t, st, be, txm)
	e.now = st.now
	reg := prometheus.NewRegistry()
	var err error
	e.metrics, err = newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		syncCycle(t.Context(), e)
		now = now.Add(e.pollInterval)
	}
	if rec := st.order("o1"); rec.Status != statusNonceUsed || rec.NonceRetries != 0 || txm.calls != 1 || st.reserved("o1") {
		t.Fatalf("known consumed order re-armed or retained liquidity: order=%+v sends=%d", rec, txm.calls)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
	// NonceUsed is also returned for explicit invalidation. Backend evidence refines the outcome.
	be.order.OrderStatus = "cancelled"
	syncCycle(t.Context(), e)
	if st.order("o1").Status != statusObsolete || txm.calls != 1 {
		t.Fatalf("invalidation was reported as a successful fill: %+v", st.order("o1"))
	}
}

func TestExecutionNonceUsedRecognitionIsExactAndLengthSafe(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "absent"},
		{name: "short", data: []byte{0x1f, 0x6d}},
		{name: "other selector", data: []byte{0xde, 0xad, 0xbe, 0xef}},
		{name: "trailing payload", data: []byte{0x1f, 0x6d, 0x5a, 0xef, 0x01}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: nonceUsedResult(tc.data)}
			e := newExec(t, st, be, txm)
			syncCycle(t.Context(), e)
			if st.order("o1").Status != statusRetryWaiting {
				t.Fatalf("unrecognized revert falsely retired an open order: %+v", st.order("o1"))
			}
		})
	}
}

func TestExecutionNonceUsedStopsEvenWhenBackendIsMissing(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order = nil
	txm := &fakeTxm{result: nonceUsedResult([]byte{0x1f, 0x6d, 0x5a, 0xef})}
	e := newExec(t, st, be, txm)
	e.now = st.now
	syncCycle(t.Context(), e)
	if st.order("o1").Status != statusNonceUsed || st.reserved("o1") {
		t.Fatalf("known used nonce waited for backend before retiring: %+v", st.order("o1"))
	}
	now = time.Unix(4_102_444_800, 0)
	syncCycle(t.Context(), e)
	if txm.calls != 1 || st.order("o1").Status != statusExpired {
		t.Fatalf("nonce-used backend observation exceeded its deadline: %+v", st.order("o1"))
	}
}
