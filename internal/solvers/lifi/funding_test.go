package lifi

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestQuoteLaneBlockedReasons(t *testing.T) {
	yes, no := func() bool { return true }, func() bool { return false }
	for _, tc := range []struct {
		name string
		lane transactionLaneState
		want string
	}{
		{name: "ready and funded", lane: &testTransactionLaneState{ready: yes, fundable: yes}},
		{name: "busy lane wins", lane: &testTransactionLaneState{ready: no, fundable: no}, want: "transaction lane is unavailable"},
		{
			name: "unfundable", lane: &testTransactionLaneState{ready: yes, fundable: no},
			want: "signer balance cannot fund a fill at the pricing horizon",
		},
		{name: "unwired lane fails closed", want: "transaction lane is unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Solver{txLaneState: tc.lane}
			if got := s.quoteLaneBlocked(); got != tc.want {
				t.Fatalf("quoteLaneBlocked = %q, want %q", got, tc.want)
			}
			if s.transactionLaneReady() != (tc.want == "") {
				t.Fatalf("transactionLaneReady = %v with block reason %q", s.transactionLaneReady(), tc.want)
			}
		})
	}
}

// TestQuoteLoopWithholdsQuotesWhileLaneUnfundable publishes a curve only while the funding gate is open,
// and follows its lane-state notifications: a close retires the curve, a reopen republishes it.
func TestQuoteLoopWithholdsQuotesWhileLaneUnfundable(t *testing.T) {
	cfg := testLifiConfig()
	cfg.QuoteRefreshMode = quoteRefreshModeInterval
	cfg.QuoteInterval = time.Hour
	cfg.QuoteTTL = 2 * time.Hour
	cfg.OrderServer.HTTPTimeout = time.Second
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	now := time.Unix(1_700_000_000, 0)
	quoteSubmitted := make(chan struct{}, 2)
	quoteExpired := make(chan struct{}, 2)
	orderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quotes/submit" {
			http.NotFound(w, r)
			return
		}
		var request quoteSubmissionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Quotes) != 1 {
			t.Errorf("unexpected quote submission: %v", err)
			http.Error(w, "expected one quote", http.StatusBadRequest)
			return
		}
		signal := quoteSubmitted
		if len(request.Quotes[0].Ranges) == 0 {
			signal = quoteExpired
		}
		select {
		case signal <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","quotesAdded":%d}`, len(request.Quotes[0].Ranges))
	}))
	defer orderServer.Close()

	var fundable atomic.Bool
	laneStateChanges := make(chan struct{}, 1)
	laneStateSubscribed := make(chan struct{})
	solver := &Solver{
		cfg:      cfg,
		reader:   fakeLifiReader{},
		strategy: recoveryGateStrategy{tokenIn: tokenIn, tokenOut: tokenOut},
		orders:   newOrderClient(orderServer.URL, "test-key", time.Second, 11155111),
		log:      logr.Discard(),
		now:      func(context.Context) (time.Time, error) { return now, nil },
		wallNow:  func() time.Time { return now },
		maxFeePerGas: func(context.Context) (*big.Int, error) {
			return big.NewInt(1), nil
		},
		txLaneState: &testTransactionLaneState{
			ready:       func() bool { return true },
			fundable:    fundable.Load,
			changes:     laneStateChanges,
			onSubscribe: func() { close(laneStateSubscribed) },
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	connectionCtx, cancelConnection := context.WithCancel(t.Context())
	defer cancelConnection()
	feedConnections := make(chan context.Context, 1)
	feedConnections <- connectionCtx
	done := make(chan error, 1)
	go func() {
		done <- solver.quoteLoop(ctx, nil, make(chan struct{}), feedConnections)
	}()

	expectSignal(t, laneStateSubscribed)
	select {
	case <-quoteSubmitted:
		t.Fatal("quote was published while the lane was unfundable")
	case <-time.After(50 * time.Millisecond):
	}
	fundable.Store(true)
	laneStateChanges <- struct{}{}
	expectSignal(t, quoteSubmitted)
	fundable.Store(false)
	laneStateChanges <- struct{}{}
	expectSignal(t, quoteExpired)
	select {
	case <-quoteSubmitted:
		t.Fatal("quote was republished while the lane was unfundable")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("quoteLoop error = %v, want context cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quoteLoop did not stop")
	}
}

// TestFillCompletionNotAdmittedIsAnExpectedSkip completes a fill the balance guard refused: the
// completion is declined rather than failed, and logged at Info rather than as a failed fill.
func TestFillCompletionNotAdmittedIsAnExpectedSkip(t *testing.T) {
	rec := tracetest.Install(t)
	log, lines := tracetest.CaptureLogs(t, 0)
	solver := &Solver{log: log}
	baseCtx := observability.WithLogger(t.Context(), log)
	fill := &pendingFill{
		order:          &submittedOrder{OrderID: tracingOrderID, QuoteID: tracingQuoteID},
		orderID:        common.HexToHash("0x1"),
		reservationKey: "order-1",
	}
	pending := &pendingFillState{byOrder: map[string]*pendingFill{"order-1": fill}}

	ctx, end := tracer.Start(baseCtx, "lifi.order.process")
	err := solver.completeFill(ctx, pending, fillCompletion{fill: fill, result: txmanager.Result{
		Outcome:     txmanager.OutcomeSubmissionError,
		Err:         errors.Errorf("send %q: %w", "lifi-fill", txmanager.ErrUnaffordable),
		NotAdmitted: true,
	}})
	end(err)

	if err != nil {
		t.Fatalf("completion error = %v, want an expected skip", err)
	}
	complete := tracetest.Ended(t, rec, "lifi.order.complete")
	if complete.Status().Code == codes.Error {
		t.Fatalf("complete span ended with error status %q", complete.Status().Description)
	}
	if reason, count := tracetest.EventAttr(complete, "declined", "reason"); count != 1 || reason != "unaffordable" {
		t.Fatalf("declined events = %d with reason %q, want 1 with unaffordable", count, reason)
	}
	var skipped, failed int
	for _, line := range lines() {
		switch {
		case strings.Contains(line, `"msg":"order fill not admitted"`) && strings.Contains(line, `"level":0`):
			skipped++
		case strings.Contains(line, `"msg":"order fill failed"`):
			failed++
		}
	}
	if skipped != 1 || failed != 0 {
		t.Fatalf("logged %d skips and %d failed fills, want 1/0: %v", skipped, failed, lines())
	}
}
