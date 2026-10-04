package rfq

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestExecutionReconcilesPeerFillOnceWithoutOwnSuccess(t *testing.T) {
	st, be := fillFixtures(t)
	peerHash := common.HexToHash("0xbeef").Hex()
	be.order.TxHash = &peerHash
	e := newExec(t, st, be, &fakeTxm{result: nonceConsumedResult()})
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	e.metrics = metrics
	log, capture := tracetest.CaptureLogs(t, 0)
	e.log = log
	ctx := observability.WithLogger(t.Context(), log)
	syncCycle(ctx, e)
	for range 3 {
		e.reconcileTerminalStatus(ctx, "o1")
	}
	if rec := st.order("o1"); rec.Status != statusFilled || rec.TxHash.Hex() != peerHash {
		t.Fatalf("peer completion did not retire order: %+v", rec)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", "peer", 1)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "solver_bot_workflow_amount_atomic_units_total" {
			t.Fatalf("peer fill recorded local amounts: %v", family)
		}
	}
	peerLogs := 0
	for _, line := range capture() {
		if strings.Contains(line, `"error"`) {
			t.Fatalf("peer fill emitted an Error log: %s", line)
		}
		if strings.Contains(line, "order filled by peer") {
			peerLogs++
		}
	}
	if peerLogs != 1 {
		t.Fatalf("peer completion log count = %d, want 1", peerLogs)
	}
}

func TestExecutionBackendOwnReplacementIsNotPeer(t *testing.T) {
	st, be := fillFixtures(t)
	first, replacement := common.HexToHash("0x1111"), common.HexToHash("0x2222")
	be.order.TxHash = strPtr(replacement.Hex())
	result := nonceConsumedResult()
	result.Hash = first
	result.Attempts = []common.Hash{first, replacement}
	e := newExec(t, st, be, &fakeTxm{result: result})
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	e.metrics = metrics
	syncCycle(t.Context(), e)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", "peer", 0)
	result.Attempts[0] = common.Hash{}
	if rec := st.order("o1"); len(rec.AttemptHashes) != 2 || rec.AttemptHashes[0] != first {
		t.Fatalf("caller mutated retained attempt history: %+v", rec)
	}
}

func TestBackendFilledWithoutUsableHashDoesNotClaimPeer(t *testing.T) {
	for _, hash := range []*string{nil, strPtr("0xbeef"), strPtr(common.Hash{}.Hex()), strPtr("garbage")} {
		st, be := fillFixtures(t)
		st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
		be.order.TxHash = hash
		e := newExec(t, st, be, &fakeTxm{result: txmanager.Result{}})
		reg := prometheus.NewRegistry()
		metrics, err := newRFQMetrics(reg, st, "")
		if err != nil {
			t.Fatal(err)
		}
		e.metrics = metrics
		e.reconcileTerminalStatus(t.Context(), "o1")
		if rec := st.order("o1"); rec.Status != statusFilled {
			t.Fatalf("unusable hash lost backend completion: %+v", rec)
		}
		metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", "peer", 0)
	}
}

func TestExecutionBackendEarlierOwnRetryIsNotPeer(t *testing.T) {
	st, be := fillFixtures(t)
	be.order.OrderStatus = "open"
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	firstHash, nextHash := common.HexToHash("0x1111"), common.HexToHash("0x2222")
	txm := &fakeTxm{result: nonceConsumedResult()}
	txm.result.Hash = firstHash
	e := newExec(t, st, be, txm)
	e.now = st.now
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	e.metrics = metrics
	syncCycle(t.Context(), e)
	now = now.Add(e.pollInterval)
	txm.result.Hash = nextHash
	be.order.OrderStatus = "filled"
	be.order.TxHash = strPtr(firstHash.Hex())
	syncCycle(t.Context(), e)
	if rec := st.order("o1"); rec.Status != statusFilled || rec.TxHash != firstHash {
		t.Fatalf("earlier local retry did not reconcile as filled: %+v", rec)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", "peer", 0)
}
