package lifi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func TestStandingQuoteGlobalFailureExcludesForbiddenPolicyPair(t *testing.T) {
	allowed := testQuoteRoute()
	forbidden := allowed
	forbidden.TokenIn = common.HexToAddress("0xffff")
	cfg := testLifiConfig()
	cfg.Gas = nil
	cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, allowed.TokenIn)
	reg := prometheus.NewRegistry()
	m, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var rejected []map[string]any
	log := funcr.NewJSON(func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		if event["msg"] == "standing quote rejected" {
			rejected = append(rejected, event)
		}
	}, funcr.Options{Verbosity: 1})
	s := &Solver{cfg: cfg, metrics: m, reader: rejectionTestReader{err: errors.New("snapshot unavailable")},
		now: func(context.Context) (time.Time, error) { return time.Unix(1_800_000_000, 0), nil }, txLaneState: alwaysReadyTransactionLane()}
	s.refreshQuotes(observability.WithLogger(t.Context(), log), []route{allowed, forbidden}, newQuoteState(time.Second))
	if len(rejected) != 0 {
		t.Fatalf("supported rejections=%v", rejected)
	}
	requireNoPairMetricDimensions(t, reg)
}

func TestStandingQuoteMetricsHaveNoPairDimensions(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	r := testQuoteRoute()
	known := testStandingQuote(r, 100)
	newPair := testStandingQuote(r, 200)
	newPair.FromAsset = common.HexToAddress("0xffff")
	newDecimals := testStandingQuote(r, 300)
	newDecimals.FromDecimals++
	state := newQuoteState(time.Second)
	state.active = indexQuotePairs([]types.Quote{known, newPair, newDecimals})
	m.quotes.observe(state)
	requireNoPairMetricDimensions(t, reg)
	if got := testutil.ToFloat64(m.quotes.activeQuotes); got != 3 {
		t.Fatalf("active quotes=%v", got)
	}
}

func requireNoPairMetricDimensions(t *testing.T, reg *prometheus.Registry) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		switch family.GetName() {
		case "lifi_active_quote_max_input_atomic_units", "solver_bot_quote_published", "lifi_quote_pair_state_omitted_total", "solver_bot_quote_standing_rejections_total":
			t.Fatalf("removed pair collector remains: %s", family.GetName())
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				switch label.GetName() {
				case "token_in", "token_out", "token_in_decimals", "token_out_decimals", "asset", "pair", "quote_id":
					t.Fatalf("dynamic label %s remains in %s", label.GetName(), family.GetName())
				}
			}
		}
	}
}
