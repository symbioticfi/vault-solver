package rfq

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestObsoleteFillWithOwnedAttemptsWaitsForReceiptMetrics(t *testing.T) {
	for _, tc := range []struct {
		name          string
		retry         bool
		receiptFirst  bool
		attemptsOnly  bool
		receiptStatus uint64
	}{
		{name: "pending success", receiptStatus: types.ReceiptStatusSuccessful},
		{name: "pending revert", receiptStatus: types.ReceiptStatusFailed},
		{name: "attempt snapshot only", attemptsOnly: true, receiptStatus: types.ReceiptStatusSuccessful},
		{name: "unsigned retry before late receipt", retry: true, receiptStatus: types.ReceiptStatusSuccessful},
		{name: "unsigned retry after late receipt", retry: true, receiptFirst: true, receiptStatus: types.ReceiptStatusSuccessful},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			owned := abandonedTxResult()
			ownedHash := owned.Hash
			if !tc.retry {
				owned.Err = txmanager.ErrRequestObsolete
			}
			if tc.attemptsOnly {
				owned.Hash = common.Hash{}
				owned.Attempts = []common.Hash{ownedHash}
			}
			txm := &fakeTxm{result: owned}
			e := newExec(t, st, be, txm)
			e.now = st.now
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			markFilled := func() {
				be.order.OrderStatus = "filled"
				hash := ownedHash.Hex()
				be.order.TxHash = &hash
			}
			if !tc.retry {
				txm.onResult = markFilled
			}
			syncCycle(t.Context(), e)
			observe := txm.lastReq.ObserveReceipt
			if observe == nil {
				t.Fatal("owned fill has no receipt observer")
			}
			receipt := txmanager.Result{
				Hash: ownedHash, Outcome: txmanager.OutcomeConfirmed,
				Receipt: &types.Receipt{Status: tc.receiptStatus},
			}
			if tc.receiptFirst {
				observe(t.Context(), receipt)
			}
			wantSends := 1
			if tc.retry {
				if st.order("o1").Status != statusRetryWaiting {
					t.Fatalf("owned abandonment did not schedule a retry: %+v", st.order("o1"))
				}
				now = now.Add(3 * time.Second)
				txm.result = txmanager.Result{Outcome: txmanager.OutcomeSubmissionError, Err: txmanager.ErrRequestObsolete}
				txm.onResult = markFilled
				syncCycle(t.Context(), e)
				wantSends++
			}
			before := st.order("o1")
			if before.Status != statusFilled || st.reserved("o1") || txm.calls != wantSends {
				t.Fatalf("obsolete fill was not retired: sends=%d order=%+v", txm.calls, before)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", "peer", 0)
			if !tc.receiptFirst {
				observe(t.Context(), receipt)
			}
			if !reflect.DeepEqual(before, st.order("o1")) || st.reserved("o1") || txm.calls != wantSends {
				t.Fatal("late receipt changed retired business state")
			}
			wantSuccess := float64(0)
			if tc.receiptStatus == types.ReceiptStatusSuccessful {
				wantSuccess = 1
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, wantSuccess)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeObsolete, 0)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
			if wantSuccess != 0 {
				metricstest.RequireWorkflowAmount(t, reg, Name, "fill", tIn.Hex(), liquidlane.FillAmountInput, 1e18)
			}
		})
	}
}

func TestLateFillReceiptRecordsMetricsWithoutReopeningOrder(t *testing.T) {
	for _, successful := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "revert"}[successful], func(t *testing.T) {
			st, be := fillFixtures(t)
			txm := &fakeTxm{result: uncertainNonceResult(txmanager.OutcomeAbandoned)}
			e := newExec(t, st, be, txm)
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			plan := baseFillPlan()
			plan.QuotedAmountOut = big.NewInt(950_000)
			plan.Legs[0].AmountOut = big.NewInt(950_000)
			e.strategy = fixedFillStrategy{plan: plan}
			syncCycle(t.Context(), e)
			before := st.order("o1")
			if before.Status != statusFilled || st.reserved("o1") {
				t.Fatal("backend fill reconciliation did not retire the abandoned order")
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 0)
			if txm.lastReq.ObserveReceipt == nil {
				t.Fatal("abandoned fill has no receipt metrics observer")
			}
			plan.QuotedAmountOut.SetInt64(1)
			receipt := &types.Receipt{Status: types.ReceiptStatusFailed}
			want := float64(0)
			if successful {
				receipt.Status = types.ReceiptStatusSuccessful
				want = 1
			}
			txm.lastReq.ObserveReceipt(t.Context(), txmanager.Result{
				Hash: txm.result.Hash, Outcome: txmanager.OutcomeConfirmed, Receipt: receipt,
			})
			if !reflect.DeepEqual(before, st.order("o1")) || st.reserved("o1") || txm.calls != 1 {
				t.Fatal("receipt metrics observation changed retired order state or capacity")
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, want)
			if !successful {
				return
			}
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", tIn.Hex(), liquidlane.FillAmountInput, want*1e18)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", tOut.Hex(), liquidlane.FillAmountOutput, want*900_000)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", tOut.Hex(), liquidlane.FillAmountPlannedSurplus, want*50_000)
		})
	}
}

func TestOrdinaryFillReceiptRecordsSuccessOnce(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			st, be := fillFixtures(t)
			result := confirmedTxResult()
			result.Outcome = outcome
			e := newExec(t, st, be, &fakeTxm{result: result})
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			syncCycle(t.Context(), e)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", tIn.Hex(), liquidlane.FillAmountInput, 1e18)
		})
	}
}
