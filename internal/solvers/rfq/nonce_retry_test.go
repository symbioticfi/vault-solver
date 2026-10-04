package rfq

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func abandonedTxResult() txmanager.Result {
	return txmanager.Result{
		Outcome: txmanager.OutcomeAbandoned, Hash: common.HexToHash("0x1234"),
		Err: txmanager.ErrAbandoned,
	}
}

func TestExecutionNonceRetryRequiresSafeOutcome(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*executionService, *fakeTxm)
		want   orderStatus
	}{
		{name: "disabled", mutate: func(e *executionService, _ *fakeTxm) { e.maxNonceRetries = 0 }, want: statusNonceUncertain},
		{name: "tracking stopped", mutate: func(_ *executionService, txm *fakeTxm) { txm.result.Outcome = txmanager.OutcomeTrackingStopped }, want: statusSubmitted},
		{name: "order expires before next poll", mutate: func(e *executionService, _ *fakeTxm) {
			e.reader.(*fakeRecoveryReader).chainTime = time.Unix(4_102_444_797, 0)
		}, want: statusExpired},
		{name: "order expires while abandonment returns", mutate: func(e *executionService, txm *fakeTxm) {
			txm.onResult = func() { e.now = func() time.Time { return time.Unix(4_102_444_800, 0) } }
		}, want: statusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: abandonedTxResult()}
			e := newExec(t, st, be, txm)
			e.now = st.now
			tc.mutate(e, txm)
			for range 5 {
				syncCycle(t.Context(), e)
				now = now.Add(10 * time.Second)
			}
			if txm.calls != 1 || st.order("o1").Status != tc.want {
				t.Fatalf("sends = %d, order = %+v; want one send and %s", txm.calls, st.order("o1"), tc.want)
			}
		})
	}
}

func TestExecutionNonceRetryRevalidatesBackendAndDeadline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fakeBackend, *executionService)
	}{
		{name: "open-order poll fails", change: func(be *fakeBackend, _ *executionService) { be.orderListErr = errors.New("backend unavailable") }},
		{name: "no longer listed open", change: func(be *fakeBackend, _ *executionService) { be.open = nil }},
		{name: "no longer executable", change: func(be *fakeBackend, _ *executionService) {
			be.executable = nil
			be.order.OrderStatus = "expired"
		}},
		{name: "expired on chain", change: func(_ *fakeBackend, e *executionService) {
			e.reader.(*fakeRecoveryReader).chainTime = time.Unix(4_102_444_800, 0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: abandonedTxResult()}
			e := newExec(t, st, be, txm)
			e.now = st.now
			syncCycle(t.Context(), e)
			if st.order("o1").Status != statusRetryWaiting {
				t.Fatalf("order = %+v, want scheduled retry", st.order("o1"))
			}
			tc.change(be, e)
			now = now.Add(3 * time.Second)
			syncCycle(t.Context(), e)
			if txm.calls != 1 {
				t.Fatalf("sends after order became unavailable = %d, want 1", txm.calls)
			}
		})
	}
}

func TestExecutionNonceRetryRefreshesDiscountCalldata(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	id := common.HexToHash("0xab")
	be.discount = &resolveDiscountResponse{
		DiscountID: id.Hex(),
		Discount: discountTerms{
			Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Discount: "500",
			Signer:   "0x00000000000000000000000000000000000000a1",
			Protocol: "0x00000000000000000000000000000000000000a2",
			Nonce:    "1", Deadline: 4_102_444_700,
		},
		SignerSignature: "0xaa", ProtocolDeadline: 90, ProtocolSignature: "0xbb",
	}
	txm := &fakeTxm{result: abandonedTxResult()}
	e := newExec(t, st, be, txm)
	e.now = st.now
	offerDiscountCandidate(e, be, id)
	builds := 0
	e.strategy = fixedFillStrategy{plan: discountFillPlan(id), onBuild: func() { builds++ }}
	syncCycle(t.Context(), e)
	if !txm.lastReq.Deadline.Equal(time.Unix(90, 0)) {
		t.Fatalf("first deadline = %v, want original discount deadline", txm.lastReq.Deadline)
	}

	// The old attempt was abandoned and its original protocol signature is now expired.
	// A retry must resolve another one and rebuild the fill using the fresh backend order too.
	now = time.Unix(100, 0)
	e.reader.(*fakeRecoveryReader).chainTime = now
	be.discount.ProtocolDeadline = 190
	be.discount.ProtocolSignature = "0xcc"
	be.executable.ProtocolSignature = strPtr("0x1234")
	be.order.OrderStatus = "filled"
	txm.result = confirmedTxResult()
	syncCycle(t.Context(), e)
	if txm.calls != 2 || be.resolveCalls != 2 || builds != 2 || st.order("o1").Status != statusFilled {
		t.Fatalf("retry did not rebuild and fill: sends=%d resolves=%d plans=%d order=%+v", txm.calls, be.resolveCalls, builds, st.order("o1"))
	}
	if !txm.lastReq.Deadline.Equal(time.Unix(190, 0)) {
		t.Fatalf("retry deadline = %v, want refreshed protocol deadline", txm.lastReq.Deadline)
	}
	args, err := executorABI.Methods["fill"].Inputs.Unpack(txm.lastReq.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if sig := args[1].([]byte); !bytes.Equal(sig, []byte{0x12, 0x34}) {
		t.Fatalf("retry order signature = %x, want fresh 1234", sig)
	}
	swaps := *abi.ConvertType(args[3], new([]executor.IReactorDiscountSwapInput)).(*[]executor.IReactorDiscountSwapInput)
	if len(swaps) != 1 || !bytes.Equal(swaps[0].ProtocolSignature, []byte{0xcc}) {
		t.Fatalf("retry discount swaps = %+v, want fresh protocol signature cc", swaps)
	}
}

func TestExecutionDoesNotScheduleNonceRetryDuringShutdown(t *testing.T) {
	st, be := fillFixtures(t)
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: abandonedTxResult()}
	e := newExec(t, st, be, txm)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	txm.onResult = cancel
	syncCycle(ctx, e)
	if st.order("o1").Status != statusNonceUncertain || st.order("o1").NonceRetries != 0 {
		t.Fatalf("shutdown scheduled a new retry: %+v", st.order("o1"))
	}
}

func TestExecutionNonceRetryExpiresWithoutBackendReconciliation(t *testing.T) {
	for _, listed := range []bool{false, true} {
		name := "backend forgets order"
		if listed {
			name = "backend still lists expired order open"
		}
		t.Run(name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: abandonedTxResult()}
			e := newExec(t, st, be, txm)
			e.now = st.now
			syncCycle(t.Context(), e)
			if !listed {
				be.open = nil
			}
			be.order = nil
			be.executable = nil
			if listed {
				now = now.Add(3 * time.Second)
				syncCycle(t.Context(), e)
				if st.order("o1").Status != statusSubmitting {
					t.Fatalf("order = %+v, want unsigned retry awaiting an executable order", st.order("o1"))
				}
			}
			now = time.Unix(4_102_444_800, 0)
			syncCycle(t.Context(), e)
			if txm.calls != 1 || st.order("o1").Status != statusExpired {
				t.Fatalf("expired retry state = %+v, sends = %d; want expired and one send", st.order("o1"), txm.calls)
			}
			if active, _ := st.activeOrderMetrics(); active != 0 {
				t.Fatalf("active obligations after expiry = %d, want 0", active)
			}
			be.open = nil
			now = now.Add(3*time.Hour + time.Second)
			syncCycle(t.Context(), e)
			if st.order("o1") != nil {
				t.Fatal("expired nonce retry was not evicted")
			}
		})
	}
}

func TestExecutionRetryDeadlineDoesNotExpireUnknownInclusion(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: abandonedTxResult()}
	e := newExec(t, st, be, txm)
	e.now = st.now
	syncCycle(t.Context(), e)
	txm.result = txmanager.Result{
		Hash: common.HexToHash("0x5678"), Outcome: txmanager.OutcomeTrackingStopped,
		Err: errors.New("tracking stopped"),
	}
	now = now.Add(3 * time.Second)
	syncCycle(t.Context(), e)
	be.open = nil
	be.order = nil
	now = time.Unix(4_102_444_800, 0)
	syncCycle(t.Context(), e)
	if txm.calls != 2 || st.order("o1").Status != statusSubmitted || st.order("o1").TxHash != txm.result.Hash {
		t.Fatalf("unknown retry inclusion lost tracking: sends=%d order=%+v", txm.calls, st.order("o1"))
	}
}

func TestExecutionObsoleteHookReadsBackendOrderStatus(t *testing.T) {
	for _, tc := range []struct {
		status   string
		order    bool
		readErr  error
		obsolete bool
		wantErr  bool
	}{
		{status: "open", order: true},
		{status: "filled", order: true, obsolete: true},
		{status: "expired", order: true, obsolete: true},
		{status: "cancelled", order: true, obsolete: true},
		{status: "error", order: true, obsolete: true},
		{status: "unverified", order: true, obsolete: true},
		{status: "insufficient-funds", order: true, obsolete: true},
		{status: "renamed-status", order: true, wantErr: true},
		{status: "", order: true, wantErr: true},
		{status: "missing order", wantErr: true},
		{status: "backend unavailable", order: true, readErr: errors.New("backend unavailable"), wantErr: true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			st, be := fillFixtures(t)
			txm := &fakeTxm{result: confirmedTxResult()}
			e := newExec(t, st, be, txm)
			syncCycle(t.Context(), e)
			if txm.lastReq.Obsolete == nil {
				t.Fatal("fill request has no Obsolete hook")
			}

			be.order = nil
			if tc.order {
				be.order = &backendOrder{OrderID: "o1", OrderStatus: tc.status, QuoteID: "q1"}
			}
			be.orderErr = tc.readErr
			obsolete, err := txm.lastReq.Obsolete(t.Context())
			if (err != nil) != tc.wantErr || obsolete != tc.obsolete {
				t.Fatalf("Obsolete() = %v, %v; want obsolete %v, error %v", obsolete, err, tc.obsolete, tc.wantErr)
			}
		})
	}
}

func TestExecutionRetiresObsoleteOrderWithoutRetry(t *testing.T) {
	obsoleteAbandonment := abandonedTxResult()
	obsoleteAbandonment.Err = errors.Errorf("pending transaction abandoned: %w", txmanager.ErrRequestObsolete)
	unconfirmed := obsoleteAbandonment
	unconfirmed.Outcome = txmanager.OutcomeTrackingStopped
	for _, tc := range []struct {
		name          string
		result        txmanager.Result
		backendStatus string
		want          orderStatus
		wantObsolete  float64
	}{
		{name: "abandoned obsolete fill", result: obsoleteAbandonment, backendStatus: "open", want: statusObsolete},
		{name: "tracking stopped after obsolescence", result: unconfirmed, backendStatus: "open", want: statusObsolete},
		{name: "dropped before signing", result: txmanager.Result{
			Outcome: txmanager.OutcomeSubmissionError,
			Err:     errors.Errorf("send %q: %w", "rfq-fill", txmanager.ErrRequestObsolete),
		}, backendStatus: "open", want: statusObsolete, wantObsolete: 1},
		{name: "backend already reports the fill", result: obsoleteAbandonment, backendStatus: "filled", want: statusFilled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = tc.backendStatus
			reg := prometheus.NewRegistry()
			metrics, err := newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			txm := &fakeTxm{result: tc.result}
			e := newExec(t, st, be, txm)
			e.now = st.now
			e.metrics = metrics

			for range 5 {
				syncCycle(t.Context(), e)
				now = now.Add(10 * time.Second)
			}

			if txm.calls != 1 || st.order("o1").Status != tc.want {
				t.Fatalf("sends = %d, order = %+v; want one send and %s", txm.calls, st.order("o1"), tc.want)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, tc.wantObsolete)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
		})
	}
}

// An expired abandoned attempt must release its business reservation so a different awarded order
// can be freshly planned. Nonce reuse itself is exercised by txmanager's real-chain tests.
func TestExecutionAbandonedOrderExpiryAllowsFreshDifferentOrder(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: abandonedTxResult()}
	e := newExec(t, st, be, txm)
	e.now = st.now
	syncCycle(t.Context(), e)
	original := append([]byte(nil), txm.lastReq.Data...)
	if !st.reserved("o1") || st.order("o1").Status != statusRetryWaiting {
		t.Fatalf("valid abandoned fill lost its commitment: %+v", st.order("o1"))
	}
	be.open, be.order = nil, nil
	now = st.order("o1").RetryDeadline
	syncCycle(t.Context(), e)
	if st.order("o1").Status != statusExpired || st.reserved("o1") {
		t.Fatalf("expired abandoned fill retained capacity: %+v", st.order("o1"))
	}

	fresh := sampleOrder()
	fresh.Request.Nonce = big.NewInt(2)
	fresh.Request.Deadline = big.NewInt(4_102_444_900)
	encoded, err := orderTupleArgs.Pack(fresh)
	if err != nil {
		t.Fatal(err)
	}
	be.executable.OrderID, be.executable.QuoteID = "o2", "q2"
	be.executable.EncodedOrder = strPtr(hexutil.Encode(encoded))
	be.executable.Deadline = i64Ptr(4_102_444_900)
	be.executable.ProtocolSignature = strPtr("0x1234")
	be.open = []backendOrder{{OrderID: "o2", QuoteID: "q2", OrderStatus: "open", Filler: be.executable.Filler}}
	be.order = &backendOrder{OrderID: "o2", QuoteID: "q2", OrderStatus: "filled"}
	e.reader.(*fakeRecoveryReader).chainTime = now
	plan := baseFillPlan()
	plan.QuoteID = "q2"
	e.strategy = fixedFillStrategy{plan: plan}
	txm.result = confirmedTxResult()
	syncCycle(t.Context(), e)
	if txm.calls != 2 || st.order("o2") == nil || st.order("o2").Status != statusFilled {
		t.Fatalf("fresh different order did not fill: sends=%d order=%+v", txm.calls, st.order("o2"))
	}
	if bytes.Equal(original, txm.lastReq.Data) || st.order("o1").Status != statusExpired {
		t.Fatal("new order replayed the abandoned fill or reopened its expired business order")
	}
}
