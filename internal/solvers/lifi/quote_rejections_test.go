package lifi

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/symbioticfi/vault-solver/internal/observability"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

type rejectionTestReader struct {
	fakeLifiReader

	snapshot quoteSnapshotSet
	err      error
}

func (r rejectionTestReader) quoteSnapshots(context.Context, []route, common.Address) (quoteSnapshotSet, error) {
	return r.snapshot, r.err
}

type rejectionTestStrategy struct {
	types.Strategy

	output types.QuoteOutput
	err    error
	after  func()
}

func (s rejectionTestStrategy) DecideQuotes(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	if s.after != nil {
		s.after()
	}
	return s.output, s.err
}

func TestStandingQuoteBuildRejectionPaths(t *testing.T) {
	for _, reason := range []string{"no_inventory", "strategy_declined", "pair_out_of_scope", "strategy_error", "chain_time_error", "snapshot_error", "gas_price_error", "lane_unavailable", "lane_changed", "success"} {
		t.Run(reason, func(t *testing.T) {
			r := testQuoteRoute()
			cfg := testLifiConfig()
			cfg.Gas = nil
			ready := true
			reader := rejectionTestReader{snapshot: quoteSnapshotSet{Direct: []liquidlane.Inventory{{Route: r}}}}
			strategy := rejectionTestStrategy{}
			s := &Solver{cfg: cfg,
				now:         func(context.Context) (time.Time, error) { return time.Unix(1_800_000_000, 0), nil },
				wallNow:     func() time.Time { return time.Unix(1_800_000_000, 0) },
				txLaneState: &testTransactionLaneState{ready: func() bool { return ready }},
			}
			want := reason
			events, write := standingObservationLogger(t)
			ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
			switch reason {
			case "no_inventory":
				reader.snapshot.Direct = nil
			case "pair_out_of_scope":
				cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, common.HexToAddress("0xffff"))
			case "strategy_error":
				strategy.err = errors.New("private arbitrary strategy failure")
			case "chain_time_error":
				s.now = func(context.Context) (time.Time, error) { return time.Time{}, errors.New("clock unavailable") }
			case "snapshot_error":
				reader.err = errors.New("RPC unavailable")
			case "gas_price_error":
				cfg.Gas = testLifiConfig().Gas
				s.maxFeePerGas = func(context.Context) (*big.Int, error) { return nil, errors.New("fee unavailable") }
			case "lane_unavailable":
				ready = false
			case "lane_changed":
				strategy.output.Quotes = []types.Quote{testStandingQuote(r, 100)}
				strategy.after = func() { ready = false }
				want = "lane_unavailable"
			case "success":
				q := testStandingQuote(r, 100)
				strategy.output.Quotes = []types.Quote{q}
				// Unchanged curves require no HTTP submission and are not rejections.
			}
			s.reader, s.strategy = reader, strategy
			state := newQuoteState(time.Second)
			if reason == "success" {
				state.active = indexQuotePairs(strategy.output.Quotes)
			}
			// Two physical routes for the same pair still produce one rejected pair evaluation.
			s.refreshQuotes(ctx, []route{r, r}, state)
			var rejected []map[string]any
			for _, event := range *events {
				if event["msg"] == "standing quote rejected" {
					rejected = append(rejected, event)
				}
			}
			wantCount := 0
			switch reason {
			case "strategy_declined", "strategy_error", "lane_changed":
				wantCount = 1
			}
			if len(rejected) != wantCount {
				t.Fatalf("rejected=%v wantCount=%d", rejected, wantCount)
			}
			if wantCount > 0 && rejected[0]["reason"] != want {
				t.Fatalf("reason=%v want=%s", rejected[0], want)
			}
		})
	}
}

func TestStandingQuoteSubmissionErrorsLogActualTrustedBatchPairs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	r := testQuoteRoute()
	events, write := standingObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
	s := &Solver{cfg: testLifiConfig(), orders: newOrderClient(server.URL, "test-key", time.Second, 1)}
	state := newQuoteState(time.Second)
	q := testStandingQuote(r, 100)
	if _, err := s.reconcileQuotes(ctx, state, []types.Quote{q, q}, []liquidlane.Inventory{{Route: r}}, time.Unix(1_800_000_000, 0)); err == nil {
		t.Fatal("expected publish failure")
	}
	if _, err := s.reconcileQuotes(ctx, state, nil, []liquidlane.Inventory{{Route: r}}, time.Unix(1_800_000_000, 0)); err == nil {
		t.Fatal("expected withdrawal failure")
	}
	counts := make(map[string]int)
	for _, event := range *events {
		if event["msg"] == "standing quote rejected" {
			counts[event["reason"].(string)]++
		}
		if event["msg"] == "standing quote snapshot" && event["snapshotState"] != "uncertain" {
			t.Fatalf("failure produced successful snapshot: %v", event)
		}
	}
	for _, reason := range []string{"publish_error", "withdrawal_error"} {
		if counts[reason] != 1 {
			t.Fatalf("%s=%d want=1", reason, counts[reason])
		}
	}
	if calls != 2 {
		t.Fatalf("changed submission behavior: calls=%d", calls)
	}
}

func TestStandingQuoteRejectionsExcludeArbitraryPairsWithoutMetrics(t *testing.T) {
	events, write := standingObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
	s := &Solver{cfg: testLifiConfig()}
	for i := int64(1); i <= 1000; i++ {
		s.observeStandingQuoteRejection(ctx, common.BigToAddress(big.NewInt(i)), common.BigToAddress(big.NewInt(i+1)), "arbitrary error", nil)
	}
	if len(*events) != 0 {
		t.Fatalf("arbitrary pairs leaked business detail: %v", *events)
	}
}
