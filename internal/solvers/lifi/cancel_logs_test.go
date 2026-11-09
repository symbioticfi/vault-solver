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
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

// assertCancelLogged fails unless msg was logged at V(1) and nothing was logged at Error
// (funcr omits "level" for Error lines).
func assertCancelLogged(t *testing.T, events []map[string]any, msg string) {
	t.Helper()
	found := false
	for _, event := range events {
		if _, ok := event["level"]; !ok {
			t.Fatalf("canceled context logged at Error: %v", event)
		}
		found = found || event["msg"] == msg
	}
	if !found {
		t.Fatalf("missing V(1) %q in %v", msg, events)
	}
}

func TestQuoteRefreshCancelLogsNoError(t *testing.T) {
	canceled := func(ctx context.Context) error { return errors.Errorf("rpc: %w", ctx.Err()) }
	for _, tc := range []struct{ stage, msg string }{
		{"latest block", "quote refresh: read latest block"},
		{"chain time", "quote refresh: read latest block time"},
		{"snapshot", "quote refresh: read routes"},
		{"gas", "quote refresh: read max fee per gas"},
		{"discounts", "private discounts: list for quote"},
		{"strategy", "quote refresh: strategy"},
		{"submit", "quote refresh: submit quotes"},
	} {
		t.Run(tc.stage, func(t *testing.T) {
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
			switch tc.stage {
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
			if tc.stage == "latest block" {
				var lastBlock uint64
				s.shouldRefreshQuotes(ctx, state, &lastBlock)
			} else {
				s.refreshQuotes(ctx, []route{r}, state)
			}
			assertCancelLogged(t, *events, tc.msg)
		})
	}
}

func TestFillDiscountCancelLogsNoError(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	inventory := testDirectDiscountInventory()
	quote := liquidlane.FillQuote{
		Inventory: inventory, AmountIn: big.NewInt(1_000),
		GrossAmountOut: big.NewInt(1_000), MaxAmountOut: big.NewInt(900), MinDiscount: big.NewInt(100_000),
	}
	listed := &discounts.List{Discounts: []discounts.ListItem{testDiscountListItem(inventory, 1_000, now.Add(time.Minute))}}
	for _, tc := range []struct {
		name      string
		ctx       func(context.Context) (context.Context, context.CancelFunc)
		resolve   bool
		wantError bool
		msg       string
	}{
		{name: "list canceled", ctx: context.WithCancel, msg: "private discounts: list for fill"},
		{name: "resolve canceled", ctx: context.WithCancel, resolve: true, msg: "private discounts: resolve"},
		{name: "list deadline", ctx: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithDeadline(ctx, time.Unix(0, 0))
		}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, write := standingObservationLogger(t)
			base := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
			ctx, cancel := tc.ctx(base)
			cancel()
			fake := &fakeDiscountClient{listErr: errors.Errorf("list: %w", ctx.Err())}
			if tc.resolve {
				fake = &fakeDiscountClient{listed: listed, resolveErr: errors.Errorf("resolve: %w", ctx.Err())}
			}
			s := &Solver{cfg: testLifiConfig(), discounts: fake}
			s.fillDiscountQuotes(ctx, []liquidlane.FillQuote{quote}, now)
			if tc.wantError {
				if len(errorLines(*events)) == 0 {
					t.Fatalf("deadline not logged at Error: %v", *events)
				}
				return
			}
			if tc.resolve && fake.resolveCalls == 0 {
				t.Fatal("resolve was not reached")
			}
			assertCancelLogged(t, *events, tc.msg)
		})
	}
}

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
