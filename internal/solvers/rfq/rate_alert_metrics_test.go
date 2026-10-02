package rfq

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestExecutionFeeCeilingReconcilesWithoutFailureAlert(t *testing.T) {
	for _, tc := range []struct {
		backend     string
		transport   bool
		notAdmitted bool
		want        orderStatus
		failure     float64
	}{
		{backend: "open", want: statusRetryWaiting},
		{backend: "filled", want: statusFilled},
		{backend: "expired", want: statusExpired},
		{backend: "cancelled", want: statusObsolete},
		{backend: "open", transport: true, want: statusFailed, failure: 1},
		{backend: "open", notAdmitted: true, want: statusFailed},
	} {
		t.Run(tc.backend+"/"+string(tc.want)+"/"+strconv.FormatBool(tc.notAdmitted), func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus = tc.backend
			cause := txmanager.ErrFeeLimitReached
			if tc.transport {
				cause = errors.New("gas estimate RPC connection reset")
			}
			txm := &fakeTxm{result: txmanager.Result{
				Outcome: txmanager.OutcomeSubmissionError, Err: errors.Errorf("estimate fill: %w", cause), NotAdmitted: tc.notAdmitted,
			}}
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
			syncCycle(t.Context(), e)
			if rec := st.order("o1"); rec.Status != tc.want || rec.NonceRetries != 0 || rec.NonceRetryExhausted || txm.calls != 1 {
				t.Fatalf("fee ceiling reconciliation: sends=%d order=%+v", txm.calls, rec)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeFailure, tc.failure)
			admissionFailures := float64(0)
			if tc.notAdmitted {
				admissionFailures = 1
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeNotAdmitted, admissionFailures)
			paged := false
			for _, entry := range logs {
				paged = paged || strings.Contains(entry, `"error"`)
			}
			if paged != (tc.transport || tc.notAdmitted) {
				t.Fatalf("unexpected cap/transport alert classification: logs=%v", logs)
			}
		})
	}
}

func TestExhaustedNonceRetriesObserveBackendUntilTerminal(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{
		txmanager.OutcomeAbandoned, txmanager.OutcomeNonceConsumed, txmanager.OutcomeReverted,
	} {
		for _, terminal := range []string{"filled", "expired", "local deadline"} {
			t.Run(string(outcome)+"/"+terminal, func(t *testing.T) {
				st, be := fillFixtures(t)
				now := time.Unix(0, 0)
				st.now = func() time.Time { return now }
				be.order.OrderStatus = "open"
				txm := &fakeTxm{result: uncertainNonceResult(outcome)}
				e := newExec(t, st, be, txm)
				e.now, e.maxNonceRetries = st.now, 0
				reg := prometheus.NewRegistry()
				var err error
				e.metrics, err = newRFQMetrics(reg, st, "")
				if err != nil {
					t.Fatal(err)
				}
				syncCycle(t.Context(), e)
				for range 3 {
					now = now.Add(2 * time.Second)
					syncCycle(t.Context(), e)
				}
				if rec := st.order("o1"); txm.calls != 1 || !rec.NonceRetryExhausted || rec.Status != statusNonceUncertain {
					t.Fatalf("exhausted retries did not retain observation without resubmitting: sends=%d order=%+v", txm.calls, rec)
				}
				metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "nonce_retry_exhausted", 1)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "expired_after_nonce_retries", 0)

				want := statusExpired
				peer := float64(0)
				switch terminal {
				case "filled":
					// A terminal backend fill takes precedence even on the local deadline poll.
					be.order.OrderStatus = "filled"
					be.order.TxHash = strPtr(common.HexToHash("0xbeef").Hex())
					now = st.order("o1").RetryDeadline
					want, peer = statusFilled, 1
				case "expired":
					be.order.OrderStatus = "expired"
				case "local deadline":
					be.order, be.open = nil, nil
					now = st.order("o1").RetryDeadline
				}
				for range 3 {
					syncCycle(t.Context(), e)
				}
				if txm.calls != 1 || st.order("o1").Status != want || st.reserved("o1") {
					t.Fatalf("exhausted observation failed to retire its terminal order: sends=%d order=%+v", txm.calls, st.order("o1"))
				}
				metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", fillOutcomePeer, peer)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "expired_after_nonce_retries", 1-peer)
				metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "nonce_retry_exhausted", 1)
			})
		}
	}
}

func TestNonceUsedExpiryDoesNotBecomeRetryExhaustion(t *testing.T) {
	st, be := fillFixtures(t)
	be.order.OrderStatus = "open"
	e := newExec(t, st, be, &fakeTxm{})
	reg := prometheus.NewRegistry()
	var err error
	e.metrics, err = newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	st.upsertQueued(queuedOrder{OrderID: "o1"})
	st.boundUnsignedWork("o1", e.now())
	st.markNonceUsed("o1", "nonce used")
	e.handleOrder(t.Context(), st.order("o1"))
	if st.order("o1").Status != statusExpired {
		t.Fatal("nonce-used observation did not expire at its deadline")
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "nonce_retry_exhausted", 0)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "expired_after_nonce_retries", 0)
}

func TestLastPollIntervalKeepsUncertainOrderUntilActualDeadline(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{
		txmanager.OutcomeAbandoned, txmanager.OutcomeNonceConsumed, txmanager.OutcomeReverted,
	} {
		for _, limit := range []int{0, 1} {
			for _, peerFilled := range []bool{false, true} {
				t.Run(string(outcome)+"/"+strconv.Itoa(limit)+"/"+strconv.FormatBool(peerFilled), func(t *testing.T) {
					st, be := fillFixtures(t)
					now := time.Unix(0, 0)
					st.now = func() time.Time { return now }
					be.order.OrderStatus = "open"
					txm := &fakeTxm{result: uncertainNonceResult(outcome)}
					e := newExec(t, st, be, txm)
					e.now, e.maxNonceRetries = st.now, limit
					e.reader.(*fakeRecoveryReader).chainTime = time.Unix(4_102_444_790, 0)
					txm.onResult = func() {
						now = st.order("o1").RetryDeadline.Add(-e.pollInterval / 2)
					}
					reg := prometheus.NewRegistry()
					var err error
					e.metrics, err = newRFQMetrics(reg, st, "")
					if err != nil {
						t.Fatal(err)
					}
					syncCycle(t.Context(), e)
					exhausted := limit == 0
					if rec := st.order("o1"); rec.Status != statusNonceUncertain || rec.NonceRetryExhausted != exhausted ||
						rec.NonceRetries != 0 || txm.calls != 1 {
						t.Fatalf("last-poll observation expired early, resent, or spent an unusable retry: sends=%d order=%+v", txm.calls, rec)
					}
					deadline := st.order("o1").RetryDeadline
					now = deadline.Add(-time.Nanosecond)
					syncCycle(t.Context(), e)
					if rec := st.order("o1"); rec.Status != statusNonceUncertain || rec.NonceRetries != 0 || txm.calls != 1 {
						t.Fatalf("order stopped observing before its actual deadline: sends=%d order=%+v", txm.calls, rec)
					}
					metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "expired_after_nonce_retries", 0)
					now = deadline
					if peerFilled {
						be.order.OrderStatus = "filled"
						be.order.TxHash = strPtr(common.HexToHash("0xbeef").Hex())
					}
					for range 2 {
						syncCycle(t.Context(), e)
					}
					want, peer, expiry, exhaustion := statusExpired, float64(0), float64(0), float64(0)
					if peerFilled {
						want, peer = statusFilled, 1
					}
					if exhausted {
						exhaustion = 1
						if !peerFilled {
							expiry = 1
						}
					}
					if rec := st.order("o1"); rec.Status != want || rec.NonceRetries != 0 || txm.calls != 1 || st.reserved("o1") {
						t.Fatalf("final poll misclassified uncertain order or retried it: sends=%d order=%+v", txm.calls, rec)
					}
					metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", fillOutcomePeer, peer)
					metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "nonce_retry_exhausted", exhaustion)
					metricstest.RequireWorkflowEventCount(t, reg, Name, "order", "expired_after_nonce_retries", expiry)
				})
			}
		}
	}
}
