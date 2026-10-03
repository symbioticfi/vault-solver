package rfq

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

// Literal selectors and ABI words come from the vendored Reactor/Executor interfaces. These
// fixtures exercise the submission consumer, including severity and stale-listing re-arm guards.
func TestExecutionKnownEstimateErrorsRetireUnsignedOrder(t *testing.T) {
	word := strings.Repeat("0", 63) + "1"
	for _, tc := range []struct {
		name       string
		data       string
		want       orderStatus
		wantErrors int
	}{
		{name: "expired request", data: "dd9cfdb6", want: statusExpired},
		{name: "invalid filler", data: "02f5c732", want: statusFailed, wantErrors: 1},
		{name: "invalid adapter", data: "fbf66df1", want: statusFailed, wantErrors: 1},
		{name: "insufficient balance", data: "cf479181" + word + word, want: statusFailed, wantErrors: 1},
		{name: "executor caller", data: "16c618d8", want: statusFailed, wantErrors: 1},
		{name: "executor reactor", data: "73f7fe5f", want: statusFailed, wantErrors: 1},
		{name: "missing code", data: "9996b315" + word, want: statusFailed, wantErrors: 1},
		{name: "invalid input amount", data: "cae33fc2", want: statusFailed, wantErrors: 1},
		{name: "invalid output", data: "98f73609", want: statusFailed, wantErrors: 1},
		{name: "invalid input token", data: "d70f29d2", want: statusFailed, wantErrors: 1},
		{name: "invalid protocol signature", data: "2e90f060", want: statusFailed, wantErrors: 1},
		{name: "invalid short string", data: "b3512b0c", want: statusFailed, wantErrors: 1},
		{name: "string too long", data: "305a27a9" + strings.Repeat("0", 62) + "20" + strings.Repeat("0", 63) + "3" + "616263" + strings.Repeat("0", 58), want: statusFailed, wantErrors: 1},
		{name: "invalid owner", data: "1e4fbdf7" + word, want: statusFailed, wantErrors: 1},
		{name: "unauthorized owner", data: "118cdaa7" + word, want: statusFailed, wantErrors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
			e := newExec(t, st, be, txm)
			e.now = st.now
			reg := prometheus.NewRegistry()
			var err error
			e.metrics, err = newRFQMetrics(reg, st, "")
			if err != nil {
				t.Fatal(err)
			}
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			for range 6 {
				syncCycle(t.Context(), e)
				now = now.Add(e.pollInterval)
			}
			if rec := st.order("o1"); rec.Status != tc.want || txm.calls != 1 || st.reserved("o1") {
				t.Fatalf("persistent estimate reverted and re-armed: order=%+v estimates=%d", rec, txm.calls)
			}
			if got := errorLogCount(logs); got != tc.wantErrors {
				t.Fatalf("error logs=%d, want %d: %v", got, tc.wantErrors, logs)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, float64(tc.wantErrors))
		})
	}
}

func TestExecutionUnknownEstimateErrorsHaveIndependentRetryBudget(t *testing.T) {
	for _, limit := range []int{0, 2} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: estimateRevertResult()}
			e := newExec(t, st, be, txm)
			e.now, e.maxNonceRetries = st.now, limit
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			for range 7 {
				syncCycle(t.Context(), e)
				now = now.Add(e.pollInterval)
			}
			if rec := st.order("o1"); rec.Status != statusFailed || rec.NonceRetries != 0 || txm.calls != 1+limit || st.reserved("o1") {
				t.Fatalf("estimate retry budget did not bound simulation independently: order=%+v estimates=%d", rec, txm.calls)
			}
			if got := errorLogCount(logs); got != 1 {
				t.Fatalf("persistent unknown estimate error logs=%d: %v", got, logs)
			}
		})
	}
}

func TestExecutionEstimateErrorsPreferBackendPeerCompletion(t *testing.T) {
	for _, data := range []string{"02f5c732", "dd9cfdb6", "deadbeef"} {
		t.Run(data, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.TxHash = strPtr("0x" + strings.Repeat("11", 32))
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + data))}
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
			if st.order("o1").Status != statusFilled || txm.calls != 1 || errorLogCount(logs) != 0 {
				t.Fatalf("backend peer lost to estimate classification: order=%+v estimates=%d logs=%v", st.order("o1"), txm.calls, logs)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", fillOutcomePeer, 1)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, 0)
		})
	}
}

func TestExecutionFatalEstimateWaitsForBackendReconciliation(t *testing.T) {
	for _, terminal := range []string{"filled", "open"} {
		t.Run(terminal, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order = nil
			txm := &fakeTxm{result: nonceUsedResult([]byte{0x02, 0xf5, 0xc7, 0x32})}
			e := newExec(t, st, be, txm)
			var logs []string
			e.log = funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
			for range 3 {
				syncCycle(t.Context(), e)
			}
			if !st.order("o1").Status.active() || txm.calls != 1 || errorLogCount(logs) != 0 {
				t.Fatalf("fatal estimate failed before backend was available: order=%+v logs=%v", st.order("o1"), logs)
			}
			be.order = &backendOrder{OrderID: "o1", OrderStatus: terminal, TxHash: strPtr("0x" + strings.Repeat("11", 32))}
			syncCycle(t.Context(), e)
			want, wantErrors := statusFilled, 0
			if terminal == "open" {
				want, wantErrors = statusFailed, 1
			}
			if st.order("o1").Status != want || txm.calls != 1 || errorLogCount(logs) != wantErrors {
				t.Fatalf("backend recovery did not classify retained estimate: order=%+v logs=%v", st.order("o1"), logs)
			}
		})
	}
}

func TestExecutionMalformedEstimateErrorsUseBoundedUnknownRetry(t *testing.T) {
	for _, data := range []string{"02", "02f5c73201", "dd9cfdb601", "cf479181", "5274afe7", "5274afe7" + strings.Repeat("00", 33), "5274afe7" + "ff" + strings.Repeat("00", 31), "305a27a9", "305a27a9" + strings.Repeat("00", 64)} {
		t.Run(data, func(t *testing.T) {
			st, be := fillFixtures(t)
			now := time.Unix(0, 0)
			st.now = func() time.Time { return now }
			be.order.OrderStatus = "open"
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + data))}
			e := newExec(t, st, be, txm)
			e.now, e.maxNonceRetries = st.now, 1
			syncCycle(t.Context(), e)
			if st.order("o1").Status != statusRetryWaiting {
				t.Fatalf("malformed payload was trusted: %+v", st.order("o1"))
			}
			now = now.Add(e.pollInterval)
			syncCycle(t.Context(), e)
			if st.order("o1").Status != statusFailed || txm.calls != 2 {
				t.Fatalf("malformed revert escaped bounded retries: %+v", st.order("o1"))
			}
		})
	}
}

func TestExecutionEstimateReconciliationSpanReportsPermanentError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		backend string
		limit   int
		want    codes.Code
	}{
		{name: "setup failure", data: "02f5c732", backend: "open", want: codes.Error},
		{name: "unknown exhausted", data: "deadbeef", backend: "open", want: codes.Error},
		{name: "unknown retry", data: "deadbeef", backend: "open", limit: 1},
		{name: "expired", data: "dd9cfdb6", backend: "open"},
		{name: "peer completion", data: "02f5c732", backend: "filled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.Install(t)
			st, be := fillFixtures(t)
			be.order.OrderStatus = tc.backend
			txm := &fakeTxm{result: nonceUsedResult(hexutil.MustDecode("0x" + tc.data))}
			e := newExec(t, st, be, txm)
			e.maxNonceRetries = tc.limit
			syncCycle(t.Context(), e)
			if got := tracetest.Ended(t, recorder, "rfq.order.report").Status().Code; got != tc.want {
				t.Fatalf("reconciliation span status=%s, want %s", got, tc.want)
			}
		})
	}
}

func errorLogCount(logs []string) int {
	count := 0
	for _, entry := range logs {
		if strings.Contains(entry, `"error"`) {
			count++
		}
	}
	return count
}
