package lifi

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

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
			reg := prometheus.NewRegistry()
			metrics, err := newLIFIMetrics(reg, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			r := testQuoteRoute()
			metrics.quotes.routes = []route{r}
			cfg := testLifiConfig()
			cfg.Gas = nil
			ready := true
			reader := rejectionTestReader{snapshot: quoteSnapshotSet{Direct: []liquidlane.Inventory{{Route: r}}}}
			strategy := rejectionTestStrategy{}
			s := &Solver{cfg: cfg, metrics: metrics,
				now:         func(context.Context) (time.Time, error) { return time.Unix(1_800_000_000, 0), nil },
				wallNow:     func() time.Time { return time.Unix(1_800_000_000, 0) },
				txLaneState: &testTransactionLaneState{ready: func() bool { return ready }},
			}
			want := reason
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
			s.refreshQuotes(t.Context(), []route{r, r}, state)
			families, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, family := range families {
				if family.GetName() != "solver_bot_quote_standing_rejections_total" {
					continue
				}
				for _, sample := range family.GetMetric() {
					count++
					if sample.GetCounter().GetValue() != 1 {
						t.Fatal("pair double counted")
					}
					for _, label := range sample.GetLabel() {
						if label.GetName() == "reason" && label.GetValue() != want {
							t.Fatalf("reason=%s want=%s", label.GetValue(), want)
						}
					}
				}
			}
			if (reason == "success" && count != 0) || (reason != "success" && count != 1) {
				t.Fatalf("series=%d", count)
			}
		})
	}
}

func TestStandingQuoteSubmissionErrorsCountActualBatchPairs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	metrics, err := newLIFIMetrics(prometheus.NewRegistry(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	r := testQuoteRoute()
	metrics.quotes.routes = []route{r}
	s := &Solver{metrics: metrics, orders: newOrderClient(server.URL, "test-key", time.Second, 1)}
	state := newQuoteState(time.Second)
	q := testStandingQuote(r, 100)
	if _, err := s.reconcileQuotes(t.Context(), state, []types.Quote{q, q}, time.Unix(1_800_000_000, 0)); err == nil {
		t.Fatal("expected publish failure")
	}
	if _, err := s.reconcileQuotes(t.Context(), state, nil, time.Unix(1_800_000_000, 0)); err == nil {
		t.Fatal("expected withdrawal failure")
	}
	for _, reason := range []string{"publish_error", "withdrawal_error"} {
		if got := testutil.ToFloat64(metrics.quotes.rejections.WithLabelValues(strings.ToLower(r.TokenIn.Hex()), strings.ToLower(r.TokenOut.Hex()), reason)); got != 1 {
			t.Fatalf("%s=%v want=1", reason, got)
		}
	}
	if calls != 2 {
		t.Fatalf("changed submission behavior: calls=%d", calls)
	}
}

func TestStandingQuoteRejectionsBoundUntrustedLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	metrics.quotes.routes = []route{testQuoteRoute()}
	s := &Solver{metrics: metrics}
	for i := int64(1); i <= 1000; i++ {
		s.observeStandingQuoteRejection(t.Context(), common.BigToAddress(big.NewInt(i)), common.BigToAddress(big.NewInt(i+1)), "arbitrary error")
	}
	if got := testutil.CollectAndCount(metrics.quotes.rejections); got != 1 {
		t.Fatalf("unbounded series=%d", got)
	}
	if got := testutil.ToFloat64(metrics.quotes.rejections.WithLabelValues("other", "other", "unknown")); got != 1000 {
		t.Fatalf("foreign count=%v", got)
	}
}
