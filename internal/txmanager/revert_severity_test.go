package txmanager

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestMinedRevertLeavesAlertSeverityToIntegration(t *testing.T) {
	logs, logger := newLogCapture(0)
	b := &metricsReceiptBackend{mockBackend: newMockBackend(), reverted: true}
	m := newStreakManager(t, b, logger)
	startManagerForTest(t, m)
	result := m.Send(t.Context(), Request{To: common.Address{1}, GasLimit: 21_000, Label: "fill"})
	if result.Outcome != OutcomeReverted || result.Err == nil {
		t.Fatalf("revert outcome or cause lost: %+v", result)
	}
	if failures, info := countLogs(*logs, "transaction reverted"); failures != 0 || info != 1 {
		t.Fatalf("manager paged before integration reconciliation: errors=%d info=%d", failures, info)
	}
}
