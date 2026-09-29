package txmanager

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestTipPaidFollowsTheLandedAttempt: a mined lifecycle counts gas used times the landed attempt's tip cap in
// tip_paid_wei_total, beside the fee it paid in fee_paid_wei_total, so the tip share of spend is their ratio.
func TestTipPaidFollowsTheLandedAttempt(t *testing.T) {
	backend := &metricsReceiptBackend{mockBackend: newMockBackend(), effectiveGasPrice: big.NewInt(21e9)}
	metrics := newTestMetrics(t)
	manager := startTestManager(t, backend, Config{}, metrics)
	result := manager.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rfq-fill"})
	if result.Outcome != OutcomeConfirmed {
		t.Fatalf("outcome = %q, want confirmed", result.Outcome)
	}
	sent := backend.attemptedTransactions()
	if len(sent) != 1 {
		t.Fatalf("sent %d transactions, want 1", len(sent))
	}
	tip := sent[0].GasTipCap()
	if tip.Sign() <= 0 {
		t.Fatalf("tip cap = %s, want positive", tip)
	}
	want := weiFloat(new(big.Int).Mul(big.NewInt(21_000), tip))
	assertMetric(t, metrics.tipPaidWei.WithLabelValues("rfq-fill", string(OutcomeConfirmed)), want)
	assertMetric(t, metrics.feePaidWei.WithLabelValues("rfq-fill", string(OutcomeConfirmed)), 21_000*21e9)
}
