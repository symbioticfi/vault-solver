package rfq

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func nonceConsumedResult() txmanager.Result {
	return txmanager.Result{
		Hash: common.HexToHash("0x1234"),
		Err:  txmanager.ErrNonceConsumed,
	}
}

func TestExecutionConsumedNonceReconcilesTerminalBackendStatus(t *testing.T) {
	for _, outcome := range []error{txmanager.ErrNonceConsumed, context.DeadlineExceeded} {
		t.Run(outcome.Error(), func(t *testing.T) { executionConsumedNonceReconcilesTerminalBackendStatus(t, outcome) })
	}
}

func executionConsumedNonceReconcilesTerminalBackendStatus(t *testing.T, outcome error) {
	t.Helper()
	for _, tc := range []struct {
		backend string
		want    orderStatus
	}{
		{backend: "filled", want: statusFilled},
		{backend: "expired", want: statusExpired},
		{backend: "cancelled", want: statusFailed},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus = tc.backend
			txm := &fakeTxm{result: uncertainNonceResult(outcome)}
			e := newExec(t, st, be, txm)
			reg := prometheus.NewRegistry()
			metrics, err := newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			e.metrics = metrics
			for range 3 {
				syncCycle(t.Context(), e)
			}
			rec := st.order("o1")
			if rec.Status != tc.want || rec.NonceRetries != 0 || txm.calls != 1 {
				t.Fatalf("terminal reconciliation: order=%+v sends=%d", rec, txm.calls)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
		})
	}
}

func TestExecutionConsumedNonceRebuildsOnlyAfterFreshOpenPoll(t *testing.T) {
	for _, tc := range []struct {
		outcome     error
		wantRetries int
	}{
		{outcome: txmanager.ErrNonceConsumed, wantRetries: 1},
		{outcome: context.DeadlineExceeded, wantRetries: 1},
	} {
		t.Run(tc.outcome.Error(), func(t *testing.T) {
			executionConsumedNonceRebuildsOnlyAfterFreshOpenPoll(t, tc.outcome, tc.wantRetries)
		})
	}
}

func executionConsumedNonceRebuildsOnlyAfterFreshOpenPoll(t *testing.T, outcome error, wantRetries int) {
	t.Helper()
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: uncertainNonceResult(outcome)}
	e := newExec(t, st, be, txm)
	e.now = st.now
	builds := 0
	e.strategy = fixedFillStrategy{plan: baseFillPlan(), onBuild: func() { builds++ }}
	syncCycle(t.Context(), e)
	first := append([]byte(nil), txm.lastReq.Data...)
	if rec := st.order("o1"); rec.Status != statusRetryWaiting || rec.TxHash != txm.result.Hash || rec.NonceRetries != wantRetries {
		t.Fatalf("consumed nonce did not retain identity and schedule retry: %+v", rec)
	}
	if txm.lastReq.Obsolete == nil {
		t.Fatal("fresh fills must retain gas estimation and protocol obsolescence checks")
	}
	syncCycle(t.Context(), e)
	if txm.calls != 1 {
		t.Fatal("retry ran before its polling backoff")
	}
	now = now.Add(3 * time.Second)
	be.open = nil
	syncCycle(t.Context(), e)
	if txm.calls != 1 {
		t.Fatal("retry ran without a fresh open-order listing")
	}
	be.open = []backendOrder{{OrderID: "o1", OrderStatus: "open", QuoteID: "q1", Filler: be.executable.Filler}}
	be.executable.ProtocolSignature = strPtr("0x1234")
	txm.result = confirmedTxResult()
	be.order.OrderStatus = "filled"
	syncCycle(t.Context(), e)
	if txm.calls != 2 || builds != 2 || st.order("o1").Status != statusFilled {
		t.Fatalf("fresh retry: sends=%d plans=%d order=%+v", txm.calls, builds, st.order("o1"))
	}
	if bytes.Equal(first, txm.lastReq.Data) {
		t.Fatal("retry replayed old calldata after the executable signature changed")
	}
	args, err := executorABI.Methods["fill"].Inputs.Unpack(txm.lastReq.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(args[1].([]byte), []byte{0x12, 0x34}) {
		t.Fatal("retry omitted the fresh executable signature")
	}
}

func TestExecutionConsumedNonceRetryBudgetIsBounded(t *testing.T) {
	for _, outcome := range []error{txmanager.ErrNonceConsumed, context.DeadlineExceeded, errCanonicalRevert} {
		t.Run(outcome.Error(), func(t *testing.T) { executionConsumedNonceRetryBudgetIsBounded(t, outcome) })
	}
}

func executionConsumedNonceRetryBudgetIsBounded(t *testing.T, outcome error) {
	t.Helper()
	for _, limit := range []int{0, 2} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: uncertainNonceResult(outcome)}
			e := newExec(t, st, be, txm)
			e.now, e.maxNonceRetries = st.now, limit
			for range 6 {
				syncCycle(t.Context(), e)
				now = now.Add(3 * time.Second)
			}
			if rec := st.order("o1"); txm.calls != limit+1 || rec.Status != statusNonceUncertain ||
				rec.NonceRetries != limit || !rec.NonceRetryExhausted {
				t.Fatalf("consumed nonce exceeded retry budget: sends=%d order=%+v", txm.calls, st.order("o1"))
			}
		})
	}
}

func TestExecutionConsumedNonceUnknownBackendRemainsBounded(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*fakeBackend)
	}{
		{name: "missing", apply: func(be *fakeBackend) { be.order = nil }},
		{name: "unavailable", apply: func(be *fakeBackend) { be.orderErr = errors.New("backend unavailable") }},
		{name: "unknown", apply: func(be *fakeBackend) { be.order.OrderStatus = "unknown" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			change.apply(be)
			txm := &fakeTxm{result: nonceConsumedResult()}
			e := newExec(t, st, be, txm)
			e.now = st.now
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != statusNonceUncertain || rec.NonceRetries != 0 {
				t.Fatalf("unknown backend prematurely scheduled a retry: %+v", rec)
			}
			now = time.Unix(4_102_444_800, 0)
			syncCycle(t.Context(), e)
			if txm.calls != 1 || st.order("o1").Status != statusExpired {
				t.Fatalf("unknown backend outlived order deadline: sends=%d order=%+v", txm.calls, st.order("o1"))
			}
		})
	}
}

func TestExecutionConsumedNonceDoesNotScheduleDuringShutdown(t *testing.T) {
	st, be := fillFixtures(t)
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: nonceConsumedResult()}
	e := newExec(t, st, be, txm)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	txm.onResult = cancel
	syncCycle(ctx, e)
	if rec := st.order("o1"); rec.Status != statusNonceUncertain || rec.NonceRetries != 0 {
		t.Fatalf("shutdown scheduled a consumed nonce retry: %+v", rec)
	}
}

var errCanonicalRevert = errors.New("canonical transaction reverted")

func uncertainNonceResult(cause error) txmanager.Result {
	result := nonceConsumedResult()
	result.Err = cause
	if errors.Is(cause, errCanonicalRevert) {
		result.Receipt = &types.Receipt{Status: types.ReceiptStatusFailed}
	}
	return result
}
