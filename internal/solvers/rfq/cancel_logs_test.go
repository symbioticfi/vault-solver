package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type canceledCandidateReader struct{}

func (canceledCandidateReader) readQuoteCandidates(
	ctx context.Context, _ []solverInventory, _, _ common.Address, _ *big.Int, _ pendingReservations,
) ([]liquidlane.QuoteCandidate, error) {
	return nil, errors.Errorf("multicall: %w", ctx.Err())
}

// canceledContext returns a canceled context and the lines its logger wrote at Error.
func canceledContext(t *testing.T) (context.Context, func() []string) {
	t.Helper()
	var errorLines []string
	log := funcr.NewJSON(func(line string) {
		if !strings.Contains(line, `"level":`) { // funcr omits level on Error lines
			errorLines = append(errorLines, line)
		}
	}, funcr.Options{Verbosity: 1})
	ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), log))
	cancel()
	return ctx, func() []string { return errorLines }
}

func TestQuoteCancelLogsNoError(t *testing.T) {
	ctx, errorLines := canceledContext(t)
	srv := testServer()
	srv.quotes.reader = canceledCandidateReader{}
	if _, err := srv.handleQuote(ctx, &quoteInput{Secret: testSecret, Body: validQuoteBody()}); err == nil {
		t.Fatal("expected quote failure")
	}
	if got := errorLines(); len(got) != 0 {
		t.Fatalf("canceled request logged at Error: %v", got)
	}
}

func TestPollOpenOrdersCancelLogsNoError(t *testing.T) {
	ctx, errorLines := canceledContext(t)
	backend := &fakeBackend{orderListErr: errors.Errorf("list open orders: %w", context.Canceled)}
	e := newExec(t, newStore(func() time.Time { return time.Unix(0, 0) }), backend, &fakeTxm{})
	e.syncOnce(ctx)
	if got := errorLines(); len(got) != 0 {
		t.Fatalf("canceled poll logged at Error: %v", got)
	}
}
