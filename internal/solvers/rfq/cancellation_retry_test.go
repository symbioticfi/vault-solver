package rfq

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func confirmedCancellation() txmanager.Result {
	hash := common.HexToHash("0x1234")
	return txmanager.Result{
		Outcome: txmanager.OutcomeCancelled, Hash: hash,
		Receipt: &ethtypes.Receipt{TxHash: hash, Status: ethtypes.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1)},
		Err:     errors.New("pending transaction cancelled"),
	}
}

func TestExecutionCancellationRetryRequiresSafeOutcome(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*executionService, *fakeTxm)
		want   orderStatus
	}{
		{name: "disabled", mutate: func(e *executionService, _ *fakeTxm) { e.maxCancellationRetries = 0 }, want: statusFailed},
		{name: "reverted", mutate: func(_ *executionService, txm *fakeTxm) { txm.result.Outcome = txmanager.OutcomeReverted }, want: statusFailed},
		{name: "unconfirmed cancellation", mutate: func(_ *executionService, txm *fakeTxm) { txm.result.Outcome = txmanager.OutcomeCancelledUnconfirmed }, want: statusSubmitted},
		{name: "tracking stopped", mutate: func(_ *executionService, txm *fakeTxm) { txm.result.Outcome = txmanager.OutcomeTrackingStopped }, want: statusSubmitted},
		{name: "no receipt", mutate: func(_ *executionService, txm *fakeTxm) { txm.result.Receipt = nil }, want: statusFailed},
		{name: "failed cancellation", mutate: func(_ *executionService, txm *fakeTxm) { txm.result.Receipt.Status = ethtypes.ReceiptStatusFailed }, want: statusFailed},
		{name: "order expires before next poll", mutate: func(e *executionService, _ *fakeTxm) {
			e.reader.(*fakeRecoveryReader).chainTime = time.Unix(4_102_444_797, 0)
		}, want: statusFailed},
		{name: "order expires while cancellation confirms", mutate: func(e *executionService, txm *fakeTxm) {
			txm.onResult = func() { e.now = func() time.Time { return time.Unix(4_102_444_800, 0) } }
		}, want: statusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: confirmedCancellation()}
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

func TestExecutionCancellationRetryRevalidatesBackendAndDeadline(t *testing.T) {
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
			txm := &fakeTxm{result: confirmedCancellation()}
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

func TestExecutionCancellationRetryRefreshesDiscountCalldata(t *testing.T) {
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
	txm := &fakeTxm{result: confirmedCancellation()}
	e := newExec(t, st, be, txm)
	e.now = st.now
	offerDiscountCandidate(e, be, id)
	builds := 0
	e.strategy = fixedFillStrategy{plan: discountFillPlan(id), onBuild: func() { builds++ }}
	syncCycle(t.Context(), e)
	if !txm.lastReq.CancelAt.Equal(time.Unix(90, 0)) {
		t.Fatalf("first deadline = %v, want original discount deadline", txm.lastReq.CancelAt)
	}

	// The cancellation is settled and the original protocol signature is now expired.
	// A retry must resolve another one and rebuild the fill using the fresh backend order too.
	now = time.Unix(100, 0)
	e.reader.(*fakeRecoveryReader).chainTime = now
	be.discount.ProtocolDeadline = 190
	be.discount.ProtocolSignature = "0xcc"
	be.executable.ProtocolSignature = strPtr("0x1234")
	be.order.OrderStatus = "filled"
	txm.result = confirmedTxResult()
	syncCycle(t.Context(), e)
	// Plans: the award reservation, the first submission and the retry rebuild. The retry keeps its
	// reservation while waiting, so it is not reserved again.
	if txm.calls != 2 || be.resolveCalls != 2 || builds != 3 || st.order("o1").Status != statusFilled {
		t.Fatalf("retry did not rebuild and fill: sends=%d resolves=%d plans=%d order=%+v", txm.calls, be.resolveCalls, builds, st.order("o1"))
	}
	if !txm.lastReq.CancelAt.Equal(time.Unix(190, 0)) {
		t.Fatalf("retry deadline = %v, want refreshed protocol deadline", txm.lastReq.CancelAt)
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

func TestExecutionDoesNotScheduleCancellationRetryDuringShutdown(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedCancellation()}
	e := newExec(t, st, be, txm)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	txm.onResult = cancel
	syncCycle(ctx, e)
	if st.order("o1").Status != statusFailed || st.order("o1").CancellationRetries != 0 {
		t.Fatalf("shutdown scheduled a new retry: %+v", st.order("o1"))
	}
}

func TestExecutionCancellationRetryExpiresWithoutBackendReconciliation(t *testing.T) {
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
			txm := &fakeTxm{result: confirmedCancellation()}
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
				t.Fatal("expired cancellation retry was not evicted")
			}
		})
	}
}

func TestExecutionRetryDeadlineDoesNotExpireUnknownInclusion(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: confirmedCancellation()}
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
