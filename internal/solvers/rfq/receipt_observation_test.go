package rfq

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestCanonicalFillReceiptRecordsSuccessOnce(t *testing.T) {
	for _, status := range []uint64{types.ReceiptStatusSuccessful, types.ReceiptStatusFailed} {
		t.Run(map[uint64]string{1: "success", 0: "reverted"}[status], func(t *testing.T) {
			st, be := fillFixtures(t)
			result := confirmedTxResult()
			result.Receipt.Status = status
			e := newExec(t, st, be, &fakeTxm{result: result})
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			syncCycle(t.Context(), e)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, float64(status))
			metricstest.RequireNoWorkflowAmounts(t, reg, Name)
		})
	}
}
