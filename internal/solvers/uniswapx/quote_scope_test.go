package uniswapx

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	sdktracetest "go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func TestQuoteScopeRequiresExactInventoryDirection(t *testing.T) {
	a, b := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	x, y := common.HexToAddress("0x3"), common.HexToAddress("0x4")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if request.Method == "eth_chainId" {
			response["result"] = "0x1"
		} else {
			// Failed optional metadata keeps trusted membership without invented decimals/symbols.
			response["error"] = map[string]any{"code": -32000, "message": "optional metadata unavailable"}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	client, err := chain.Dial(t.Context(), []string{server.URL}, "", "0x0000000000000000000000000000000000000011", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	metadata := chain.NewDecimals(client)
	if err := metadata.WarmMetadata(t.Context(), []common.Address{a, b}); err == nil {
		t.Fatal("expected optional metadata failure")
	}
	s := newQuoteTestSolver(t, a, &quoteTestStrategy{})
	s.tokenMetadata = metadata
	s.quoteState.Load().inventory = []liquidlane.Inventory{
		{Route: liquidlane.Route{TokenIn: a, TokenOut: x}},
		{Route: liquidlane.Route{TokenIn: b, TokenOut: y}},
	}
	for _, pair := range []struct {
		in, out common.Address
		want    bool
	}{{a, x, true}, {b, y, true}, {a, y, false}, {b, x, false}, {x, a, false}} {
		if got := s.quotePairInScope(validQuoteRequest(pair.in, pair.out)); got != pair.want {
			t.Fatalf("scope %s -> %s=%v want=%v", pair.in, pair.out, got, pair.want)
		}
	}
}

func TestQuoteReturnedEventRequiresSuccessfulWrite(t *testing.T) {
	for _, mode := range []string{"success", "failure", "short"} {
		t.Run(mode, func(t *testing.T) {
			var returned []map[string]any
			log := funcr.NewJSON(func(line string) {
				var event map[string]any
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Error(err)
				}
				if event["msg"] == "quote returned" {
					returned = append(returned, event)
				}
			}, funcr.Options{Verbosity: 1})
			a, b := common.HexToAddress("0x1"), common.HexToAddress("0x2")
			s := newQuoteTestSolver(t, a, &quoteTestStrategy{quote: &types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}})
			s.quoteState.Load().inventory = []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: a, TokenOut: b}}}
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
			s.metrics = metrics
			body, err := json.Marshal(validQuoteRequest(a, b))
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(string(body)))
			var writer http.ResponseWriter = httptest.NewRecorder()
			switch mode {
			case "failure":
				writer = errorQuoteWriter{httptest.NewRecorder()}
			case "short":
				writer = shortQuoteWriter{httptest.NewRecorder()}
			}
			s.quoteHandler(writer, r)
			if mode != "success" && len(returned) != 0 {
				t.Fatalf("false returned event: %v", returned)
			}
			if mode == "success" && len(returned) != 1 {
				t.Fatalf("post-write event=%v", returned)
			}
			reason, result := "none", "returned"
			if mode != "success" {
				reason, result = "response_write_error", "rejected"
			} else if written, ok := returned[0]["responseWritten"].(bool); !ok || !written {
				t.Fatalf("missing write fact: %v", returned[0])
			}
			metricstest.RequireFamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"result": result, "reason": reason}, 1)
			assertQuoteTrafficTotals(t, reg, 1)
		})
	}
}

type shortQuoteWriter struct{ *httptest.ResponseRecorder }

func (w shortQuoteWriter) Write(body []byte) (int, error) {
	return w.ResponseRecorder.Write(body[:len(body)-1])
}

func TestQuoteScopeDoesNotDependOnDisplayMetadata(t *testing.T) {
	in, out := common.HexToAddress("0x101"), common.HexToAddress("0x202")
	for _, tc := range []struct {
		name   string
		change func(*Solver, *quoteRequest)
		want   bool
	}{
		{name: "new trusted route with no metadata", want: true},
		{name: "same addresses another chain", change: func(_ *Solver, r *quoteRequest) { r.TokenOutChainID = 8453 }},
		{name: "reverse direction", change: func(_ *Solver, r *quoteRequest) { r.TokenIn, r.TokenOut = r.TokenOut, r.TokenIn }},
		{name: "same token", change: func(_ *Solver, r *quoteRequest) { r.TokenOut = r.TokenIn }},
		{name: "absent route", change: func(s *Solver, _ *quoteRequest) { s.quoteState.Load().inventory = nil }},
		{name: "denied input policy", change: func(s *Solver, _ *quoteRequest) {
			policy, err := tokenpolicy.New(tokenpolicy.Permissioned, []common.Address{out})
			if err != nil {
				t.Fatal(err)
			}
			s.cfg.TokenPolicy = policy
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
			s.quoteState.Load().inventory = []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: in, TokenOut: out}}}
			r := validQuoteRequest(in, out)
			if tc.change != nil {
				tc.change(s, &r)
			}
			if got := s.quotePairInScope(r); got != tc.want {
				t.Fatalf("scope=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestUnsupportedQuoteLogsOmitUntrustedRequestDetails(t *testing.T) {
	in, out := common.HexToAddress("0x101"), common.HexToAddress("0x202")
	var events []map[string]any
	log := funcr.NewJSON(func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		events = append(events, event)
	}, funcr.Options{Verbosity: 1})
	s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
	r := validQuoteRequest(in, out)
	r.Protocol = "unsupported-protocol-canary"
	r.RequestID, r.QuoteID = "request-canary", "quote-canary"
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.quoteHandler(w, httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(string(body))))
	if w.Code != http.StatusNoContent {
		t.Fatalf("HTTP=%d", w.Code)
	}
	if len(events) != 1 || events[0]["reason"] != string(quoteDeclineUnsupportedProtocol) {
		t.Fatalf("events=%v", events)
	}
	if known, ok := events[0]["pairInScope"].(bool); !ok || known {
		t.Fatalf("unsupported pair scope: %v", events)
	}
	for _, key := range []string{"tokenIn", "tokenOut", "requestId", "quoteId", "amount", "protocol"} {
		if _, present := events[0][key]; present {
			t.Fatalf("untrusted detail %s: %v", key, events)
		}
	}
}

func TestAwardPairIsSelfContainedAndEmittedOnce(t *testing.T) {
	in, out := common.HexToAddress("0x101"), common.HexToAddress("0x202")
	s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
	s.quoteState.Load().inventory = []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: in, TokenOut: out}}}
	s.exclusiveUntil = make(map[common.Hash]trackedExclusive)
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
	s.metrics = metrics
	var events []map[string]any
	log := funcr.NewJSON(func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		if event["msg"] == "exclusive quote awarded" {
			events = append(events, event)
		}
	}, funcr.Options{})
	ctx := observability.WithLogger(t.Context(), log)
	order := &resolvedOrder{Hash: common.HexToHash("0xabc"), QuoteID: "no-http-history", Source: orderSourceExclusiveV2, TokenIn: in, TokenOut: out, ExclusiveUntil: uint64(time.Now().Add(time.Minute).Unix())}
	for range 2 {
		if s.trackExclusive(order, time.Now()) {
			s.observeAward(ctx, order)
		}
	}
	if len(events) != 1 || events[0]["pairScopeSource"] != "verified_inventory" || events[0]["tokenIn"] != in.Hex() || events[0]["tokenOut"] != out.Hex() || events[0]["orderHash"] != order.Hash.Hex() {
		t.Fatalf("events=%v", events)
	}
	if known, ok := events[0]["pairInScope"].(bool); !ok || !known {
		t.Fatalf("verified pair scope: %v", events)
	}
	order.TokenIn, order.TokenOut = out, in
	s.observeAward(ctx, order)
	if len(events) != 1 {
		t.Fatalf("unsupported reverse award: %v", events)
	}
	requirePairSeries(t, reg, 1)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"event": "assignment", "result": "observed"}, 1)
}

// The HTTP scope check and decision may see different refresh snapshots. Only the decision's
// proof may attribute a positive response; this test preserves the existing custom strategy result.
func TestQuoteReturnedScopeUsesDecisionSnapshot(t *testing.T) {
	tracetest.Install(t)
	in, out := common.HexToAddress("0x101"), common.HexToAddress("0x202")
	s := newQuoteTestSolver(t, in, &quoteTestStrategy{quote: &types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}})
	s.quoteState.Load().inventory = []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: in, TokenOut: out}}}
	provider, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	if !ok {
		t.Fatal("expected test trace provider")
	}
	processor := &quoteScopeSwap{SpanProcessor: sdktracetest.NewSpanRecorder(), swap: func() {
		next := *s.quoteState.Load()
		next.inventory = nil
		s.quoteState.Store(&next)
	}}
	provider.RegisterSpanProcessor(processor)
	t.Cleanup(func() { provider.UnregisterSpanProcessor(processor) })
	var returned bool
	log := funcr.NewJSON(func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		returned = returned || event["msg"] == "quote returned"
	}, funcr.Options{Verbosity: 1})
	body, err := json.Marshal(validQuoteRequest(in, out))
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.quoteHandler(w, httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(string(body))))
	if w.Code != http.StatusOK || returned {
		t.Fatalf("HTTP=%d supported returned event=%v", w.Code, returned)
	}
}

type quoteScopeSwap struct {
	sdktrace.SpanProcessor

	swap func()
}

func (p *quoteScopeSwap) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	if span.Name() == "uniswapx.quote" {
		p.swap()
	}
	p.SpanProcessor.OnStart(ctx, span)
}

func TestUnsupportedPairStrategyTraceOmitsRequestDetails(t *testing.T) {
	in, out := common.HexToAddress("0x101"), common.HexToAddress("0x202")
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := newBlockingQuoteTestSolver(t, in, strategy)
	s.quoteState.Load().inventory = []liquidlane.Inventory{liquidlane.DirectInventory(
		liquidlane.Route{ID: "other-direction", TokenIn: in, TokenOut: common.HexToAddress("0x303")},
		big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))}
	var logs []string
	log := funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{Verbosity: 1})
	r := validQuoteRequest(in, out)
	r.RequestID, r.QuoteID = "unsupported-request-canary", "unsupported-quote-canary"
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.quoteHandler(w, httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(string(body))))
	if w.Code != http.StatusNoContent {
		t.Fatalf("HTTP=%d", w.Code)
	}
	if len(logs) != 1 {
		t.Fatalf("unexpected detail events: %v", logs)
	}
	for _, raw := range logs {
		for _, canary := range []string{r.RequestID, r.QuoteID, in.Hex(), out.Hex()} {
			if strings.Contains(raw, canary) {
				t.Fatalf("unsupported request detail: %s", raw)
			}
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatal(err)
		}
		if event["reason"] != string(quoteDeclineNoMatchingRoutes) {
			t.Fatalf("decline reason lost: %v", event)
		}
	}
}

func TestQuoteStrategyErrorDetailRequiresSupportedPair(t *testing.T) {
	in, out := common.HexToAddress("0x101"), common.HexToAddress("0x202")
	for _, scoped := range []bool{false, true} {
		s := newBlockingQuoteTestSolver(t, in, &errorQuoteStrategy{})
		if scoped {
			s.quoteState.Load().inventory = []liquidlane.Inventory{{Route: liquidlane.Route{TokenIn: in, TokenOut: out}}}
		}
		var logs []string
		log := funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{Verbosity: 1})
		body, err := json.Marshal(validQuoteRequest(in, out))
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.quoteHandler(w, httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(string(body))))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("HTTP=%d", w.Code)
		}
		if strings.Contains(strings.Join(logs, "\n"), "request-specific backend error") != scoped {
			t.Fatalf("scoped=%v logs=%v", scoped, logs)
		}
	}
}
