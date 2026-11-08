package rfq

import (
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func estimateRevertResult() txmanager.Result {
	return txmanager.Result{Outcome: txmanager.OutcomeSubmissionError, Err: errors.Errorf("estimate fill: %w",
		&txmanager.ExecutionRevertError{Cause: errors.New("execution reverted")})}
}

func TestExecutionEstimateRevertReconcilesBeforeFailure(t *testing.T) {
	for _, tc := range []struct {
		backend string
		want    orderStatus
	}{
		{backend: "filled", want: statusFilled},
		{backend: "expired", want: statusExpired},
		{backend: "cancelled", want: statusObsolete},
		{backend: "open", want: statusRetryWaiting},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus = tc.backend
			txm := &fakeTxm{result: estimateRevertResult()}
			e := newExec(t, st, be, txm)
			e.maxNonceRetries = 1 // Unknown estimates have their own bounded retry allowance.
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			for range 3 {
				syncCycle(t.Context(), e)
			}
			if rec := st.order("o1"); rec.Status != tc.want || rec.NonceRetries != 0 || txm.calls != 1 {
				t.Fatalf("estimate revert reconciliation: order=%+v sends=%d", rec, txm.calls)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
			for _, entry := range logs {
				if strings.Contains(entry, `"error"`) {
					t.Fatalf("expected execution race paged before reconciliation: %s", entry)
				}
			}
		})
	}
}

func TestExecutionEstimateRevertRetriesFreshOpenOrderWithinDeadline(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: estimateRevertResult()}
	e := newExec(t, st, be, txm)
	e.now, e.maxNonceRetries = st.now, 1
	syncCycle(t.Context(), e)
	if rec := st.order("o1"); rec.Status != statusRetryWaiting || !st.reserved("o1") {
		t.Fatalf("unsigned execution race lost the order or reservation: %+v", rec)
	}
	now = now.Add(e.pollInterval)
	be.open = nil
	syncCycle(t.Context(), e)
	if txm.calls != 1 {
		t.Fatal("retried an estimate without a fresh open-order poll")
	}
	be.open = []backendOrder{{OrderID: "o1", OrderStatus: "open", QuoteID: "q1", Filler: be.executable.Filler}}
	be.executable.ProtocolSignature = strPtr("0x1234")
	txm.result = confirmedTxResult()
	be.order.OrderStatus = "filled"
	syncCycle(t.Context(), e)
	if txm.calls != 2 || st.order("o1").Status != statusFilled {
		t.Fatalf("fresh open order did not recover: sends=%d order=%+v", txm.calls, st.order("o1"))
	}
}

func TestExecutionEstimateRevertWithUnavailableBackendExpiresLocally(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order = nil
	txm := &fakeTxm{result: estimateRevertResult()}
	e := newExec(t, st, be, txm)
	e.now = st.now
	syncCycle(t.Context(), e)
	if st.order("o1").Status != statusEstimateReverted {
		t.Fatalf("missing backend should retain estimate reconciliation: %+v", st.order("o1"))
	}
	now = time.Unix(4_102_444_800, 0)
	syncCycle(t.Context(), e)
	if txm.calls != 1 || st.order("o1").Status != statusExpired || st.reserved("o1") {
		t.Fatalf("estimate reconciliation outlived deadline: sends=%d order=%+v", txm.calls, st.order("o1"))
	}
}
