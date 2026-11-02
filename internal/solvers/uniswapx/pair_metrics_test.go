package uniswapx

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func TestSupportedPairHTTPMetrics(t *testing.T) {
	in, out := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	for _, tc := range []struct {
		name                string
		change              func(*quoteRequest)
		writeFailure, short bool
		result, reason      string
	}{
		{name: "returned", result: "returned", reason: "none"},
		{name: "unsupported protocol on known pair", change: func(r *quoteRequest) { r.Protocol = "v3" }, result: "rejected", reason: "unsupported_protocol"},
		{name: "invalid amount", change: func(r *quoteRequest) { r.Amount = "bad" }, result: "rejected", reason: "invalid_amount"},
		{name: "missing ID", change: func(r *quoteRequest) { r.RequestID = "" }, result: "rejected", reason: "missing_ids"},
		{name: "failed write", writeFailure: true, result: "rejected", reason: "response_write_error"},
		{name: "short write", short: true, result: "rejected", reason: "response_write_error"},
		{name: "wrong chain", change: func(r *quoteRequest) { r.TokenInChainID = 10 }},
		{name: "reverse", change: func(r *quoteRequest) { r.TokenIn, r.TokenOut = r.TokenOut, r.TokenIn }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newQuoteTestSolver(t, in, &quoteTestStrategy{quote: &types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}})
			s.quoteState.Load().inventory = []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: in, TokenOut: out}}}
			s.metricRoutes = []liquidlane.Route{{TokenIn: in, TokenOut: out}}
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
			s.metrics = metrics
			request := validQuoteRequest(in, out)
			if tc.change != nil {
				tc.change(&request)
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var writer http.ResponseWriter = httptest.NewRecorder()
			if tc.writeFailure {
				writer = errorQuoteWriter{httptest.NewRecorder()}
			}
			if tc.short {
				writer = shortQuoteWriter{httptest.NewRecorder()}
			}
			s.quoteHandler(writer, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", strings.NewReader(string(body))))
			if tc.result == "" {
				requirePairSeries(t, reg, 0)
				return
			}
			requirePairSeries(t, reg, 1)
			metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"event": "quote", "result": tc.result, "reason": tc.reason, "chain_id": "1", "token_in": strings.ToLower(in.Hex()), "token_out": strings.ToLower(out.Hex())}, 1)
		})
	}
}

func TestSupportedPairMetricAuthorityNoArbitraryGrowth(t *testing.T) {
	in, out := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
	s.quoteState.Store(&quoteState{}) // structural support survives zero liquidity; metadata is nil
	s.metricRoutes = []liquidlane.Route{{TokenIn: in, TokenOut: out}}
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
	s.metrics = metrics
	send := func(request quoteRequest) {
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		s.quoteHandler(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", strings.NewReader(string(body))))
	}
	for i := range 500 {
		other := common.BigToAddress(big.NewInt(int64(i + 100)))
		send(validQuoteRequest(other, out))
		send(validQuoteRequest(in, other))
	}
	send(validQuoteRequest(out, in))
	requirePairSeries(t, reg, 0)
	send(validQuoteRequest(in, out))
	requirePairSeries(t, reg, 1)
	policy, err := tokenpolicy.New(tokenpolicy.Permissioned, []common.Address{out})
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.TokenPolicy = policy
	if s.metricPairSupported(in, out) {
		t.Fatal("denied policy retained pair eligibility")
	}
	send(validQuoteRequest(in, out))
	requirePairSeries(t, reg, 1)
	// A newly trusted exact inventory route extends eligibility, without metadata or a parallel registry.
	newIn := common.HexToAddress("0x3")
	policy, err = tokenpolicy.New(tokenpolicy.Permissioned, []common.Address{newIn})
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.TokenPolicy = policy
	s.quoteState.Store(&quoteState{inventory: []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: newIn, TokenOut: out}}}})
	if !s.metricPairSupported(newIn, out) || s.metricPairSupported(out, newIn) {
		t.Fatal("new trusted route direction incorrect")
	}
	send(validQuoteRequest(newIn, out))
	requirePairSeries(t, reg, 2)
}

func requirePairSeries(t *testing.T, reg prometheus.Gatherer, want int) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, family := range families {
		if family.GetName() == "solver_bot_pair_events_total" {
			got = len(family.GetMetric())
		}
	}
	if got != want {
		t.Fatalf("pair series=%d want=%d", got, want)
	}
}
