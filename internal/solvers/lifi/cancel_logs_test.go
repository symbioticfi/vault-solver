package lifi

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

// errorLines returns the logged events that were written at Error (funcr omits "level" for them).
func errorLines(events []map[string]any) []map[string]any {
	var out []map[string]any
	for _, event := range events {
		if _, ok := event["level"]; !ok {
			out = append(out, event)
		}
	}
	return out
}

func TestQuoteRefreshCancelLogsNoError(t *testing.T) {
	canceled := func(ctx context.Context) error { return errors.Errorf("rpc: %w", ctx.Err()) }
	for _, stage := range []string{"latest block", "chain time", "snapshot", "gas", "discounts", "strategy", "submit"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			defer server.Close()
			r := testQuoteRoute()
			cfg := testLifiConfig()
			cfg.Gas = nil
			cfg.QuoteTTL = time.Minute
			reader := rejectionTestReader{snapshot: quoteSnapshotSet{Direct: []liquidlane.Inventory{{Route: r}}}}
			strategy := rejectionTestStrategy{}
			s := &Solver{cfg: cfg,
				now:         func(context.Context) (time.Time, error) { return time.Unix(1_800_000_000, 0), nil },
				wallNow:     func() time.Time { return time.Unix(1_800_000_000, 0) },
				txLaneState: &testTransactionLaneState{ready: func() bool { return true }},
				orders:      newOrderClient(server.URL, "test-key", time.Second, 1),
			}
			events, write := standingObservationLogger(t)
			base := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
			ctx, cancel := context.WithCancel(base)
			cancel()
			switch stage {
			case "latest block":
				s.cfg.QuoteRefreshMode = quoteRefreshModeBlock
				reader.latestBlockErr = canceled(ctx)
			case "chain time":
				s.now = func(ctx context.Context) (time.Time, error) { return time.Time{}, canceled(ctx) }
			case "snapshot":
				reader.err = canceled(ctx)
			case "gas":
				s.cfg.Gas = testLifiConfig().Gas
				s.maxFeePerGas = func(ctx context.Context) (*big.Int, error) { return nil, canceled(ctx) }
			case "discounts":
				reader.snapshot.Physical = []liquidlane.Inventory{{Route: r}}
				s.discounts = &fakeDiscountClient{listErr: canceled(ctx)}
			case "strategy":
				strategy.err = canceled(ctx)
			case "submit":
				strategy.output.Quotes = []types.Quote{testStandingQuote(r, 100)}
			}
			s.reader, s.strategy = reader, strategy
			s.setPairRoutes([]route{r})
			state := newQuoteState(time.Second)
			if stage == "latest block" {
				var lastBlock uint64
				s.shouldRefreshQuotes(ctx, state, &lastBlock)
			} else {
				s.refreshQuotes(ctx, []route{r}, state)
			}
			if got := errorLines(*events); len(got) != 0 {
				t.Fatalf("canceled context logged at Error: %v", got)
			}
		})
	}
}

func TestFillDiscountListCancelLogsNoError(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ctx       func(context.Context) (context.Context, context.CancelFunc)
		wantError bool
	}{
		{name: "canceled", ctx: context.WithCancel},
		{name: "deadline", ctx: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithDeadline(ctx, time.Unix(0, 0))
		}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, write := standingObservationLogger(t)
			base := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
			ctx, cancel := tc.ctx(base)
			cancel()
			s := &Solver{cfg: testLifiConfig(), discounts: &fakeDiscountClient{listErr: errors.Errorf("list: %w", ctx.Err())}}
			s.fillDiscountQuotes(ctx, []liquidlane.FillQuote{{}}, time.Unix(1_800_000_000, 0))
			if got := len(errorLines(*events)) != 0; got != tc.wantError {
				t.Fatalf("logged at Error=%v want %v: %v", got, tc.wantError, *events)
			}
		})
	}
}
