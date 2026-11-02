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

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

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
	s := newBlockingQuoteTestSolver(t, route.TokenIn, strategy)
	s.cfg.SolverMode = solverModeInternal
	s.cfg.Discounts = &DiscountConfig{HTTPTimeout: time.Second}
	s.cfg.QuoteServer.QuoteTTL = time.Minute
	s.reader, s.discounts = reader, provider
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
	var decline map[string]any
	log := funcr.NewJSON(func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		if event["msg"] == "quote declined" {
			decline = event
		}
	}, funcr.Options{Verbosity: 1})
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
	for i := range 100 {
		unknown := common.BigToAddress(big.NewInt(int64(i + 100)))
		send(validQuoteRequest(route.TokenIn, unknown))
		send(validQuoteRequest(unknown, route.TokenOut))
	}
	send(validQuoteRequest(route.TokenOut, route.TokenIn))
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
	known, ok := decline["pairInScope"].(bool)
	if !ok || !known || decline["pairScopeSource"] != "verified_quote_routes" || decline["tokenIn"] != route.TokenIn.Hex() || decline["tokenOut"] != route.TokenOut.Hex() {
		t.Fatalf("missing truthful structural pair proof: %v", decline)
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
