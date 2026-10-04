package rfq

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

// These shared Reactor/Executor error signatures describe a failed external operation, not proof
// that the order is permanently invalid. The selectors and address word are literal ABI fixtures.
var transientEstimateFixtures = []struct{ name, data string }{
	{name: "FailedCall", data: "d6bda275"},
	{name: "SafeERC20FailedOperation", data: "5274afe7" + strings.Repeat("0", 63) + "1"},
}

func TestExecutionTransientEstimateErrorsRetryFreshOpenOrder(t *testing.T) {
	for _, tc := range transientEstimateFixtures {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
			e := newExec(t, st, be, txm)
			e.now, e.maxNonceRetries = st.now, 1
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != statusRetryWaiting || rec.EstimateRetries != 1 ||
				rec.EstimateErrorName != tc.name || !st.reserved("o1") || errorLogCount(logs) != 0 {
				t.Fatalf("external operation failed permanently before a fresh retry: order=%+v logs=%v", rec, logs)
			}
			firstCalldata := bytes.Clone(txm.lastReq.Data)
			now = now.Add(e.pollInterval)
			be.open = nil
			syncCycle(t.Context(), e)
			if txm.calls != 1 {
				t.Fatal("retried without a fresh open-order listing")
			}
			be.open = []backendOrder{{OrderID: "o1", OrderStatus: "open", QuoteID: "q1", Filler: be.executable.Filler}}
			be.executable.ProtocolSignature = strPtr("0x1234")
			txm.result = confirmedTxResult()
			txm.onResult = func() { be.order.OrderStatus = "filled" }
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != statusFilled || rec.NonceRetries != 0 || txm.calls != 2 ||
				bytes.Equal(firstCalldata, txm.lastReq.Data) || errorLogCount(logs) != 0 {
				t.Fatalf("fresh planning did not recover the external operation: order=%+v sends=%d logs=%v", rec, txm.calls, logs)
			}
		})
	}
}

func TestExecutionTransientEstimateErrorsUseCountedRetryBudget(t *testing.T) {
	for _, tc := range transientEstimateFixtures {
		t.Run(tc.name, func(t *testing.T) {
			for _, limit := range []int{0, 2} {
				t.Run(strconv.Itoa(limit), func(t *testing.T) {
					st, be := fillFixtures(t)
					now := time.Unix(0, 0)
					st.now = func() time.Time { return now }
					be.order.OrderStatus = "open"
					txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
					e := newExec(t, st, be, txm)
					e.now, e.maxNonceRetries = st.now, limit
					reg := prometheus.NewRegistry()
					var err error
					e.metrics, err = newRFQMetrics(reg, st, "")
					if err != nil {
						t.Fatal(err)
					}
					var logs []string
					e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
					for range 7 {
						syncCycle(t.Context(), e)
						now = now.Add(e.pollInterval)
					}
					if rec := st.order("o1"); rec.Status != statusFailed || rec.EstimateRetries != limit ||
						rec.NonceRetries != 0 || rec.EstimateErrorName != tc.name || !rec.RetryRetired ||
						txm.calls != 1+limit || st.reserved("o1") {
						t.Fatalf("retry budget did not retire persistent external failure: order=%+v sends=%d", rec, txm.calls)
					}
					if errorLogCount(logs) != 1 {
						t.Fatalf("persistent operation must report Error once: %v", logs)
					}
					for _, entry := range logs {
						if strings.Contains(entry, `"error"`) && !strings.Contains(entry, tc.name) {
							t.Fatalf("permanent failure lost the decoded revert name: %s", entry)
						}
					}
					metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 1)
				})
			}
		})
	}
}

func TestExecutionTransientEstimateErrorsRespectOrderDeadline(t *testing.T) {
	for _, tc := range transientEstimateFixtures {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
			e := newExec(t, st, be, txm)
			e.now = st.now
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			syncCycle(t.Context(), e)
			now = st.order("o1").RetryDeadline.Add(-e.pollInterval / 2)
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != statusEstimateReverted || rec.EstimateRetries != 1 || txm.calls != 2 {
				t.Fatalf("scheduled a retry without deadline headroom: order=%+v sends=%d", rec, txm.calls)
			}
			now = st.order("o1").RetryDeadline
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != statusExpired || txm.calls != 2 || st.reserved("o1") || errorLogCount(logs) != 0 {
				t.Fatalf("operation retry escaped its order deadline: order=%+v sends=%d logs=%v", rec, txm.calls, logs)
			}
		})
	}
}

func TestExecutionTransientEstimateErrorsPreferBackendTerminalStatus(t *testing.T) {
	for _, tc := range transientEstimateFixtures {
		t.Run(tc.name, func(t *testing.T) {
			for _, terminal := range []struct {
				status string
				want   orderStatus
			}{
				{status: "filled", want: statusFilled},
				{status: "expired", want: statusExpired},
				{status: "cancelled", want: statusObsolete},
			} {
				t.Run(terminal.status, func(t *testing.T) {
					st, be := fillFixtures(t)
					be.order.OrderStatus = terminal.status
					be.order.TxHash = strPtr("0x" + strings.Repeat("11", 32))
					txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
					e := newExec(t, st, be, txm)
					e.maxNonceRetries = 0
					reg := prometheus.NewRegistry()
					var err error
					e.metrics, err = newRFQMetrics(reg, st, "")
					if err != nil {
						t.Fatal(err)
					}
					var logs []string
					e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
					for range 3 {
						syncCycle(t.Context(), e)
					}
					if rec := st.order("o1"); rec.Status != terminal.want || rec.EstimateRetries != 0 ||
						txm.calls != 1 || errorLogCount(logs) != 0 {
						t.Fatalf("terminal backend lost to operation failure: order=%+v sends=%d logs=%v", rec, txm.calls, logs)
					}
					wantPeer := float64(0)
					if terminal.status == "filled" {
						wantPeer = 1
					}
					metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", fillOutcomePeer, wantPeer)
					metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
				})
			}
		})
	}
}
