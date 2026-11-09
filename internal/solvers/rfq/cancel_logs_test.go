package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// blockingCandidateReader fails with the context's error once it is done.
type blockingCandidateReader struct{}

func (blockingCandidateReader) readQuoteCandidates(
	ctx context.Context, _ []solverInventory, _, _ common.Address, _ *big.Int, _ pendingReservations,
) ([]liquidlane.QuoteCandidate, error) {
	<-ctx.Done()
	return nil, errors.Errorf("multicall: %w", ctx.Err())
}

// lineLogger records every line its logger writes.
func lineLogger() (logr.Logger, *[]string) {
	lines := new([]string)
	return funcr.NewJSON(func(line string) { *lines = append(*lines, line) }, funcr.Options{Verbosity: 1}), lines
}

// errorLogged reports whether msg was logged at Error (funcr omits level there).
func errorLogged(lines []string, msg string) bool {
	for _, line := range lines {
		if !strings.Contains(line, `"level":`) && strings.Contains(line, `"msg":"`+msg+`"`) {
			return true
		}
	}
	return false
}

// assertCancelLogged fails unless msg was logged at V(1) and nothing at Error.
func assertCancelLogged(t *testing.T, lines []string, msg string) {
	t.Helper()
	found := false
	for _, line := range lines {
		if !strings.Contains(line, `"level":`) {
			t.Fatalf("canceled context logged at Error: %s", line)
		}
		found = found || strings.Contains(line, `"msg":"`+msg+`"`)
	}
	if !found {
		t.Fatalf("missing V(1) %q in %v", msg, lines)
	}
}

func TestQuoteCancelLogs(t *testing.T) {
	t.Run("client hung up", func(t *testing.T) {
		log, lines := lineLogger()
		ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), log))
		cancel()
		srv := testServer()
		srv.quotes.reader = blockingCandidateReader{}
		if _, err := srv.handleQuote(ctx, &quoteInput{Secret: testSecret, Body: validQuoteBody()}); err == nil {
			t.Fatal("expected quote failure")
		}
		assertCancelLogged(t, *lines, "quote abandoned: request canceled")
	})
	t.Run("slow dependency", func(t *testing.T) {
		log, lines := lineLogger()
		srv := testServer()
		srv.quotes.reader = blockingCandidateReader{}
		srv.quoteTimeout = time.Millisecond
		ctx := observability.WithLogger(t.Context(), log)
		if _, err := srv.handleQuote(ctx, &quoteInput{Secret: testSecret, Body: validQuoteBody()}); err == nil {
			t.Fatal("expected quote failure")
		}
		if !errorLogged(*lines, "quote failed") {
			t.Fatalf("quote timeout not logged at Error: %v", *lines)
		}
	})
}

func TestPollOpenOrdersCancelLogs(t *testing.T) {
	log, lines := lineLogger()
	ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), log))
	cancel()
	st := newStore(func() time.Time { return time.Unix(0, 0) })
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	st.markStatus("o1", statusSubmitted, common.Hash{1}, "")
	backend := &fakeBackend{
		orderListErr: errors.Errorf("list open orders: %w", context.Canceled),
		orderErr:     errors.Errorf("get order: %w", context.Canceled),
	}
	e := newExec(t, st, backend, &fakeTxm{})
	e.log = log
	e.syncOnce(ctx)
	assertCancelLogged(t, *lines, "poll open orders")
}
