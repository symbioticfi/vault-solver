package rfq

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

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
