package rfq

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestQuoteDeclinesBeforePlanningWhileLaneUnfundable(t *testing.T) {
	srv := testServer()
	reader := &countingQuoteReader{quoteCandidateReader: srv.quotes.reader}
	strategy := &countingStrategy{Strategy: srv.quotes.strategy}
	srv.quotes.laneFundable = func() bool { return false }
	srv.quotes.reader = reader
	srv.quotes.strategy = strategy

	rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
	if rr.Code != http.StatusNoContent {
		t.Fatalf("quote status = %d, want 204 (body %s)", rr.Code, rr.Body.String())
	}
	if reader.calls != 0 || strategy.quoteCalls != 0 {
		t.Fatalf("unfundable lane performed reader=%d strategy=%d calls, want none", reader.calls, strategy.quoteCalls)
	}
}

// TestQuoteDeclinesWhenLaneBecomesUnfundableDuringPlanning closes the funding gate while the strategy
// decides, as a close between the pre- and post-planning checks would.
func TestQuoteDeclinesWhenLaneBecomesUnfundableDuringPlanning(t *testing.T) {
	var fundable atomic.Bool
	fundable.Store(true)
	srv := testServer()
	srv.quotes.laneFundable = fundable.Load
	srv.quotes.strategy = &laneFlippingStrategy{Strategy: srv.quotes.strategy, ready: &fundable}

	rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
	if rr.Code != http.StatusNoContent {
		t.Fatalf("quote status = %d, want 204 after the lane became unfundable (body %s)", rr.Code, rr.Body.String())
	}
}

func TestQuoteLaneDeclineReasons(t *testing.T) {
	yes, no := func() bool { return true }, func() bool { return false }
	for _, tc := range []struct {
		name      string
		available func() bool
		fundable  func() bool
		want      quoteDecisionOutcome
	}{
		{name: "open lane", available: yes, fundable: yes, want: ""},
		{name: "nonce conflict wins", available: no, fundable: no, want: quoteDecisionLaneUnavailable},
		{name: "unfundable", available: yes, fundable: no, want: quoteDecisionLaneUnfundable},
		{name: "unwired availability fails closed", fundable: yes, want: quoteDecisionLaneUnavailable},
		{name: "unwired funding fails closed", available: yes, want: quoteDecisionLaneUnfundable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qs := &quoteService{laneAvailable: tc.available, laneFundable: tc.fundable}
			if got := qs.laneDecline(); got != tc.want {
				t.Fatalf("laneDecline = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExecutionCancellationRetrySuppressedWhileUnfundable(t *testing.T) {
	log, lines := tracetest.CaptureLogs(t, 0)
	st, be := fillFixtures(t)
	now := time.Unix(0, 0)
	st.now = func() time.Time { return now }
	be.order.OrderStatus = "open"
	txm := &fakeTxm{result: confirmedCancellation()}
	e := newExec(t, st, be, txm)
	e.now = st.now
	e.log = log
	e.laneFundable = func() bool { return false }

	for range 3 {
		syncCycle(t.Context(), e)
		now = now.Add(10 * time.Second)
	}
	if txm.calls != 1 {
		t.Fatalf("sends = %d, want no retry while the lane is unfundable", txm.calls)
	}
	if got := st.order("o1"); got.Status != statusFailed || got.CancellationRetries != 0 {
		t.Fatalf("order = %+v, want failed without a scheduled retry", got)
	}
	if !strings.Contains(strings.Join(lines(), "\n"), "retry suppressed while the signer balance cannot fund a fill") {
		t.Fatalf("suppressed retry not logged: %s", strings.Join(lines(), "\n"))
	}
}

// TestExecutionNotAdmittedFillIsAnExpectedSkip covers a fill the balance guard refused: the order fails
// with no retry loop, the refusal is logged at Info rather than as a failed fill, and the order and submit
// spans record a declined event instead of an error status.
func TestExecutionNotAdmittedFillIsAnExpectedSkip(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantReason string
	}{
		{name: "unaffordable", err: errors.Errorf("send %q: %w", "rfq-fill", txmanager.ErrUnaffordable), wantReason: "unaffordable"},
		{name: "stale head", err: errors.Errorf("send %q: %w", "rfq-fill", txmanager.ErrStaleHead), wantReason: "stale_head"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := tracetest.Install(t)
			log, lines := tracetest.CaptureLogs(t, 0)
			st, be := fillFixtures(t)
			txm := &fakeTxm{result: txmanager.Result{
				Outcome: txmanager.OutcomeSubmissionError, Err: tc.err, NotAdmitted: true,
			}}
			e := newExec(t, st, be, txm)
			e.log = log

			syncCycle(t.Context(), e)

			if got := st.order("o1"); got.Status != statusFailed || got.CancellationRetries != 0 {
				t.Fatalf("order = %+v, want failed without a retry", got)
			}
			if txm.calls != 1 {
				t.Fatalf("sends = %d, want 1", txm.calls)
			}
			var notAdmitted, failed int
			for _, line := range lines() {
				switch {
				case strings.Contains(line, `"msg":"fill not admitted"`):
					if !strings.Contains(line, `"level":0`) || !strings.Contains(line, `"reason":"`+tc.wantReason+`"`) {
						t.Fatalf("refusal log = %s, want Info with reason %s", line, tc.wantReason)
					}
					notAdmitted++
				case strings.Contains(line, `"msg":"fill failed"`):
					failed++
				}
			}
			if notAdmitted != 1 || failed != 0 {
				t.Fatalf("logged %d refusals and %d failed fills, want 1/0:\n%s", notAdmitted, failed, strings.Join(lines(), "\n"))
			}
			for _, name := range []string{"rfq.order", "rfq.order.submit"} {
				span := tracetest.Ended(t, rec, name)
				if span.Status().Code == codes.Error {
					t.Fatalf("%s span ended with error status %q", name, span.Status().Description)
				}
				if reason, count := tracetest.EventAttr(span, "declined", "reason"); count != 1 || reason != tc.wantReason {
					t.Fatalf("%s declined events = %d with reason %q, want 1 with %q", name, count, reason, tc.wantReason)
				}
			}
		})
	}
}

func TestExecutionRealSubmissionErrorStillLogsError(t *testing.T) {
	log, lines := tracetest.CaptureLogs(t, 0)
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: txmanager.Result{
		Outcome: txmanager.OutcomeSubmissionError, Err: errors.New("estimate gas: execution reverted"),
	}}
	e := newExec(t, st, be, txm)
	e.log = log

	syncCycle(t.Context(), e)

	failed := 0
	for _, line := range lines() {
		if strings.Contains(line, `"msg":"fill failed"`) {
			if strings.Contains(line, `"level":`) {
				t.Fatalf("admitted submission failure logged below error: %s", line)
			}
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("fill failed logged %d times, want 1", failed)
	}
}

// TestFillRequestObsoleteFollowsBackendStatus drives the Obsolete check the fill request carries: the
// backend moving the order to filled, cancelled or expired makes the manager cancel the pending fill
// before its CancelAt, while any other status or a failed read keeps it alive.
func TestFillRequestObsoleteFollowsBackendStatus(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)
	syncCycle(t.Context(), e)
	if txm.calls != 1 || txm.lastReq.Obsolete == nil {
		t.Fatalf("sends = %d with Obsolete wired %v, want one fill carrying an Obsolete check",
			txm.calls, txm.calls == 1 && txm.lastReq.Obsolete != nil)
	}
	check := txm.lastReq.Obsolete

	for _, tc := range []struct {
		name         string
		order        *backendOrder
		readErr      error
		wantObsolete bool
		wantErr      bool
	}{
		{name: "open", order: &backendOrder{OrderStatus: "open"}},
		{name: "filled elsewhere", order: &backendOrder{OrderStatus: "filled"}, wantObsolete: true},
		{name: "cancelled", order: &backendOrder{OrderStatus: "cancelled"}, wantObsolete: true},
		{name: "expired", order: &backendOrder{OrderStatus: "expired"}, wantObsolete: true},
		{name: "error keeps the fill", order: &backendOrder{OrderStatus: "error"}},
		{name: "unverified keeps the fill", order: &backendOrder{OrderStatus: "unverified"}},
		{name: "insufficient funds keeps the fill", order: &backendOrder{OrderStatus: "insufficient-funds"}},
		{name: "unknown status", order: &backendOrder{OrderStatus: "renamed"}, wantErr: true},
		{name: "dropped status", order: &backendOrder{}, wantErr: true},
		{name: "order not found", wantErr: true},
		{name: "backend unavailable", readErr: errors.New("backend down"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			be.order, be.orderErr = tc.order, tc.readErr
			obsolete, err := check(t.Context())
			if obsolete != tc.wantObsolete || (err != nil) != tc.wantErr {
				t.Fatalf("Obsolete = (%v, %v), want (%v, error %v)", obsolete, err, tc.wantObsolete, tc.wantErr)
			}
			if err != nil && obsolete {
				t.Fatal("a failed check reported the fill obsolete")
			}
		})
	}
}
