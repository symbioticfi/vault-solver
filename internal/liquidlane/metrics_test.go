package liquidlane

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestFillMetricsObserve(t *testing.T) {
	reg := prometheus.NewRegistry()
	workflow, err := observability.NewWorkflowMetrics(reg, "rfq", FillWorkflowSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observability.NewWorkflowMetrics(reg, "lifi", FillWorkflowSpec()); err != nil {
		t.Fatalf("register second solver: %v", err)
	}
	rfq := NewFillMetrics(workflow)
	tokenIn := common.HexToAddress("0x0000000000000000000000000000000000000011")
	tokenOut := common.HexToAddress("0x0000000000000000000000000000000000000022")
	rfq.now = func() time.Time { return time.Unix(123, 0) }
	rfq.Observe(
		&types.Receipt{Status: types.ReceiptStatusSuccessful},
		tokenIn, big.NewInt(100), tokenOut, big.NewInt(90), big.NewInt(5),
	)
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", "success", 1, 123)
	metricstest.RequireWorkflowAmount(t, reg, "rfq", "fill", tokenIn.Hex(), FillAmountInput, 100)
	metricstest.RequireWorkflowAmount(t, reg, "rfq", "fill", tokenOut.Hex(), FillAmountPlannedSurplus, 5)

	rfq.Observe(
		&types.Receipt{Status: types.ReceiptStatusFailed},
		tokenIn, big.NewInt(100), tokenOut, big.NewInt(90), big.NewInt(5),
	)
	metricstest.RequireWorkflowEvent(t, reg, "rfq", "fill", "success", 1, 123)
}

func TestPlannedSurplus(t *testing.T) {
	if got := PlannedSurplus(big.NewInt(110), big.NewInt(100)); got.String() != "10" {
		t.Fatalf("surplus = %s, want 10", got)
	}
	if got := PlannedSurplus(big.NewInt(100), big.NewInt(100)); got.Sign() != 0 {
		t.Fatalf("zero surplus = %s", got)
	}
}
