package uniswapx

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
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
	s.quoteState.Store(&quoteState{expiresAt: time.Now().Add(time.Minute), inventory: []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: newIn, TokenOut: out}}}})
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

func TestDynamicPairRefusalWithoutInventory(t *testing.T) {
	now := time.Now()
	route := testDiscountRoute()
	provider := &fakeDiscountProvider{list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
		testDiscountOffer(route, now.Add(time.Hour), "100"),
	}}}
	reader := &quoteModeReader{now: now, resolved: []liquidlane.Route{route}}
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := quoteModeSolver(reader, provider)
	s.chainID, s.strategy = 1, strategy
	s.cfg.QuoteServer.HTTPTimeout = time.Second
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
	s.metrics = metrics
	refresh := func() {
		t.Helper()
		if err := s.refreshQuoteState(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	if len(s.metricRoutes) != 0 || len(s.quoteState.Load().inventory) != 0 {
		t.Fatal("test requires dynamic discovery with no fillable inventory")
	}
	if !s.metricPairSupported(route.TokenIn, route.TokenOut) {
		t.Fatal("resolved advertised direction lost when inventory is empty")
	}
	var decline string
	log := funcr.NewJSON(func(line string) { decline = line }, funcr.Options{Verbosity: 1})
	send := func(request quoteRequest) {
		t.Helper()
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.quoteHandler(w, httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(string(body))))
		if w.Code != http.StatusNoContent {
			t.Fatalf("HTTP=%d; empty inventory must not return a quote", w.Code)
		}
	}
	// The existing authority property test covers1000arbitraryrequests; retain each dynamic boundary.
	for _, pair := range [][2]common.Address{{route.TokenIn, common.HexToAddress("0x99")}, {common.HexToAddress("0x99"), route.TokenOut}, {route.TokenOut, route.TokenIn}} {
		send(validQuoteRequest(pair[0], pair[1]))
	}
	wrongChain := validQuoteRequest(route.TokenIn, route.TokenOut)
	wrongChain.TokenOutChainID = 10
	send(wrongChain)
	requirePairSeries(t, reg, 0)
	if len(reader.adapters) != 1 {
		t.Fatal("unsupported HTTP requests triggered adapter discovery")
	}
	send(validQuoteRequest(route.TokenIn, route.TokenOut))
	requirePairSeries(t, reg, 1)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"event": "quote", "result": "rejected", "reason": "no_matching_routes", "chain_id": "1", "token_in": strings.ToLower(route.TokenIn.Hex()), "token_out": strings.ToLower(route.TokenOut.Hex())}, 1)
	for _, field := range []string{`"msg":"quote declined"`, `"pairInScope":true`, `"pairScopeSource":"verified_quote_routes"`, `"tokenIn":"` + route.TokenIn.Hex() + `"`, `"tokenOut":"` + route.TokenOut.Hex() + `"`} {
		if !strings.Contains(decline, field) {
			t.Fatalf("missing %s in %s", field, decline)
		}
	}

	// Replacing the advertised catalog withdraws proof, without resetting historical counters.
	provider.list = &liquiddiscounts.List{}
	refresh()
	if s.metricPairSupported(route.TokenIn, route.TokenOut) {
		t.Fatal("removed advertised route retained support proof")
	}
	send(validQuoteRequest(route.TokenIn, route.TokenOut))
	requirePairSeries(t, reg, 1)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"reason": "no_matching_routes"}, 1)
	// Existing discovery can admit a new trusted token without optional metadata or old proof.
	next := route
	next.TokenIn = common.HexToAddress("0x9876")
	next.ID = liquidlane.NewRouteID(s.chainID, next.Adapter, next.TokenIn, next.TokenOut)
	provider.list.Discounts = []liquiddiscounts.ListItem{testDiscountOffer(next, now.Add(time.Hour), "100")}
	reader.resolved = []liquidlane.Route{next}
	refresh()
	send(validQuoteRequest(next.TokenIn, next.TokenOut))
	requirePairSeries(t, reg, 2)
	if s.metricPairSupported(route.TokenIn, route.TokenOut) || s.tokenMetadata != nil {
		t.Fatal("route replacement or metadata-independent support regressed")
	}
}

func TestDynamicPairProofExpiresWithState(t *testing.T) {
	route := testDiscountRoute()
	for _, mode := range []string{"live", "expired state", "invalidated", "policy denied", "expired offer", "unresolved adapter", "backend failure"} {
		t.Run(mode, func(t *testing.T) {
			reader := &quoteModeReader{now: time.Now(), resolved: []liquidlane.Route{route}}
			provider := &fakeDiscountProvider{list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
				testDiscountOffer(route, reader.now.Add(time.Hour), "100"),
			}}}
			s := quoteModeSolver(reader, provider)
			s.chainID = 1
			if err := s.refreshQuoteState(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "expired state":
				s.quoteState.Load().expiresAt = time.Now().Add(-time.Second)
			case "invalidated":
				s.quoteEpoch.Add(1)
			case "policy denied":
				policy, err := tokenpolicy.New(tokenpolicy.Permissioned, []common.Address{route.TokenOut})
				if err != nil {
					t.Fatal(err)
				}
				s.cfg.TokenPolicy = policy
			case "expired offer":
				provider.list.Discounts = []liquiddiscounts.ListItem{
					testDiscountOffer(route, reader.now.Add(-time.Second), "100"),
				}
			case "unresolved adapter":
				reader.resolved = nil
			case "backend failure":
				provider.listErr = errors.New("backend unavailable")
			}
			if mode == "expired offer" || mode == "unresolved adapter" || mode == "backend failure" {
				if err := s.refreshQuoteState(t.Context(), nil); err != nil {
					t.Fatal(err)
				}
			}
			if got := s.metricPairSupported(route.TokenIn, route.TokenOut); got != (mode == "live") {
				t.Fatalf("pair proof=%v mode=%s", got, mode)
			}
		})
	}
}
