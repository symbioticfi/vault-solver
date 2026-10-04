package rfq

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestUnresolvedFatalEstimateExpiryPreservesDiagnosticWithoutFillFailure(t *testing.T) {
	for _, tc := range []struct {
		name           string
		data           string
		backendErr     error
		finalStatus    string
		wantStatus     orderStatus
		wantDiagnostic int
	}{
		{name: "missing backend", data: "02f5c732", wantStatus: statusExpired, wantDiagnostic: 1},
		{name: "unavailable backend", data: "02f5c732", backendErr: errors.New("backend unavailable"), wantStatus: statusExpired, wantDiagnostic: 1},
		{name: "final peer fill", data: "02f5c732", finalStatus: "filled", wantStatus: statusFilled},
		{name: "final backend expiry", data: "02f5c732", finalStatus: "expired", wantStatus: statusExpired},
		{name: "unknown revert", data: "deadbeef", wantStatus: statusExpired},
		{name: "nonce used", data: "1f6d5aef", wantStatus: statusExpired},
		{name: "expired request", data: "dd9cfdb6", wantStatus: statusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order, be.orderErr = nil, tc.backendErr
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
			e := newExec(t, st, be, txm)
			e.now = st.now
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			syncCycle(t.Context(), e)
			before := st.order("o1")
			if !before.Status.active() || before.RetryDeadline.IsZero() {
				t.Fatalf("missing backend did not retain bounded observation: %+v", before)
			}
			now = before.RetryDeadline.Add(-time.Second)
			syncCycle(t.Context(), e)
			if unresolvedEstimateDiagnosticCount(logs) != 0 {
				t.Fatal("diagnostic emitted before final backend reconciliation")
			}
			now = before.RetryDeadline
			if tc.finalStatus != "" {
				be.orderErr = nil
				be.order = &backendOrder{OrderID: "o1", OrderStatus: tc.finalStatus, TxHash: strPtr("0x" + strings.Repeat("11", 32))}
			}
			for range 3 {
				syncCycle(t.Context(), e)
				now = now.Add(time.Second)
			}
			after := st.order("o1")
			if after.Status != tc.wantStatus || txm.calls != 1 || st.reserved("o1") {
				t.Fatalf("expiry changed execution/state: sends=%d order=%+v", txm.calls, after)
			}
			if got := unresolvedEstimateDiagnosticCount(logs); got != tc.wantDiagnostic {
				t.Fatalf("fatal expiry diagnostics=%d, want %d: %v", got, tc.wantDiagnostic, logs)
			}
			if tc.wantDiagnostic != 0 && after.LastError != before.LastError {
				t.Fatalf("fatal cause overwritten at expiry: before=%q after=%q", before.LastError, after.LastError)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
			wantPeer := float64(0)
			if tc.finalStatus == "filled" {
				wantPeer = 1
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", fillOutcomePeer, wantPeer)
		})
	}
}

func unresolvedEstimateDiagnosticCount(logs []string) int {
	count := 0
	for _, entry := range logs {
		if strings.Contains(entry, "fatal fill estimate unresolved at order deadline") &&
			strings.Contains(entry, `"error"`) && strings.Contains(entry, `"revert":"InvalidFiller"`) {
			count++
		}
	}
	return count
}

func TestFatalEstimateExpiryDoesNotOverwriteTerminalStateOrRepeatDiagnostic(t *testing.T) {
	for _, peerAlreadyFilled := range []bool{false, true} {
		t.Run(map[bool]string{false: "unresolved", true: "terminal peer"}[peerAlreadyFilled], func(t *testing.T) {
			st := newStore(func() time.Time { return time.Unix(0, 0) })
			st.upsertQueued(queuedOrder{OrderID: "o1"})
			st.markEstimateReverted("o1", estimateRevertFatal, "InvalidFiller", "original estimate cause")
			if peerAlreadyFilled {
				st.markFilled("o1", common.HexToHash("0x1234"))
			}
			revert, cause, expired := st.expireFatalEstimate("o1")
			if peerAlreadyFilled {
				if expired || st.order("o1").Status != statusFilled {
					t.Fatal("stale expiry replaced terminal backend evidence")
				}
			} else if !expired || revert != "InvalidFiller" || cause != "original estimate cause" ||
				st.order("o1").Status != statusExpired || st.order("o1").LastError != cause {
				t.Fatal("unresolved expiry lost its original diagnostic")
			}
			if _, _, repeated := st.expireFatalEstimate("o1"); repeated {
				t.Fatal("terminal observation repeated its diagnostic")
			}
		})
	}
}
