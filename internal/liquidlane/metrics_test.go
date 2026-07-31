package liquidlane

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestFillMetricsObserve(t *testing.T) {
	reg := prometheus.NewRegistry()
	rfq, err := NewFillMetrics(reg, "rfq")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFillMetrics(reg, "lifi"); err != nil {
		t.Fatalf("register second solver: %v", err)
	}

	tokenIn := common.HexToAddress("0x0000000000000000000000000000000000000011")
	tokenOut := common.HexToAddress("0x0000000000000000000000000000000000000022")
	rfq.Observe(
		&types.Receipt{Status: types.ReceiptStatusSuccessful},
		tokenIn, big.NewInt(100), tokenOut, big.NewInt(90), big.NewInt(5),
	)

	if got := testutil.ToFloat64(rfq.amounts.WithLabelValues(
		tokenIn.Hex(), FillAmountInput,
	)); got != 100 {
		t.Fatalf("input = %v, want 100", got)
	}
	if got := testutil.ToFloat64(rfq.amounts.WithLabelValues(
		tokenOut.Hex(), FillAmountPlannedSurplus,
	)); got != 5 {
		t.Fatalf("planned surplus = %v, want 5", got)
	}

	rfq.Observe(
		&types.Receipt{Status: types.ReceiptStatusFailed},
		tokenIn, big.NewInt(100), tokenOut, big.NewInt(90), big.NewInt(5),
	)
	if got := testutil.ToFloat64(rfq.amounts.WithLabelValues(tokenIn.Hex(), FillAmountInput)); got != 100 {
		t.Fatalf("input after failed receipt = %v, want 100", got)
	}
}

func TestPlannedSurplus(t *testing.T) {
	if got := PlannedSurplus(big.NewInt(110), big.NewInt(100)); got.String() != "10" {
		t.Fatalf("surplus = %s, want 10", got)
	}
	if got := PlannedSurplus(big.NewInt(100), big.NewInt(100)); got.Sign() != 0 {
		t.Fatalf("zero surplus = %s", got)
	}
}
