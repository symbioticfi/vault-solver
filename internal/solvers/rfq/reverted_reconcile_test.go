package rfq

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestExecutionMinedRevertReconcilesBeforeFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend string
		hash    common.Hash
		want    orderStatus
		peer    float64
	}{
		{name: "sibling filled", backend: "filled", hash: common.HexToHash("0xbeef"), want: statusFilled, peer: 1},
		{name: "older own variant filled", backend: "filled", hash: common.HexToHash("0xabcd"), want: statusFilled},
		{name: "expired", backend: "expired", want: statusExpired},
		{name: "still open", backend: "open", want: statusRetryWaiting},
		{name: "cancelled", backend: "cancelled", want: statusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus, be.order.TxHash = tc.backend, strPtr(tc.hash.Hex())
			txm := &fakeTxm{result: txmanager.Result{Outcome: txmanager.OutcomeReverted,
				Hash: common.HexToHash("0xdead"), Attempts: []common.Hash{common.HexToHash("0xabcd")}, Err: errors.New("tx reverted")}}
			e := newExec(t, st, be, txm)
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != tc.want || txm.calls != 1 {
				t.Fatalf("mined revert skipped business reconciliation: sends=%d order=%+v", txm.calls, rec)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", fillOutcomePeer, tc.peer)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
			for _, entry := range logs {
				if strings.Contains(entry, `"error"`) {
					t.Fatalf("mined revert emitted Error before resolving business state: %s", entry)
				}
			}
		})
	}
}

func TestExecutionMinedRevertRetainsUnavailableBackendForPeerFill(t *testing.T) {
	st, be := fillFixtures(t)
	terminal := be.order
	be.order = nil
	txm := &fakeTxm{result: txmanager.Result{Outcome: txmanager.OutcomeReverted,
		Hash: common.HexToHash("0xdead"), Err: errors.New("tx reverted")}}
	e := newExec(t, st, be, txm)
	syncCycle(t.Context(), e)
	if rec := st.order("o1"); rec.Status != statusNonceUncertain {
		t.Fatalf("missing backend discarded unresolved mined revert: %+v", rec)
	}
	terminal.TxHash = strPtr(common.HexToHash("0xbeef").Hex())
	be.order = terminal
	syncCycle(t.Context(), e)
	if txm.calls != 1 || st.order("o1").Status != statusFilled {
		t.Fatalf("late backend peer fill resubmitted or failed: sends=%d order=%+v", txm.calls, st.order("o1"))
	}
}

func TestExecutionBackendCompletionWinsOnDeadlinePoll(t *testing.T) {
	for _, used := range []bool{false, true} {
		st, be := fillFixtures(t)
		st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
		st.boundUnsignedWork("o1", time.Unix(0, 0))
		if used {
			st.markNonceUsed("o1", "nonce used")
		} else {
			st.markNonceUncertain("o1", common.HexToHash("0xdead"), "reverted", false)
		}
		be.order.TxHash = strPtr(common.HexToHash("0xbeef").Hex())
		e := newExec(t, st, be, &fakeTxm{})
		e.handleOrder(t.Context(), st.order("o1"))
		if rec := st.order("o1"); rec.Status != statusFilled {
			t.Fatalf("local deadline replaced known backend completion: %+v", rec)
		}
	}
}
