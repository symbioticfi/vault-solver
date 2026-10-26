package uniswapx

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

func TestQuoteTrafficHTTPFinalOutcome(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, tc := range []struct {
		name                              string
		change                            func(*quoteRequest)
		raw                               string
		status                            int
		result, reason, inLabel, outLabel string
		quote                             *types.Quote
	}{
		{name: "returned", status: 200, result: "returned", reason: "none", quote: &types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}},
		{name: "nil strategy", status: 204, result: "rejected", reason: "strategy-declined"},
		{name: "unsupported protocol", change: func(r *quoteRequest) { r.Protocol = "v3" }, status: 204, result: "rejected", reason: "unsupported_protocol"},
		{name: "unsupported chain", change: func(r *quoteRequest) { r.TokenInChainID = 8453 }, status: 204, result: "rejected", reason: "unsupported_chain", inLabel: "other"},
		{name: "missing quote ID", change: func(r *quoteRequest) { r.QuoteID = "" }, status: 204, result: "rejected", reason: "missing_ids"},
		{name: "missing request ID", change: func(r *quoteRequest) { r.RequestID = "" }, status: 400, result: "rejected", reason: "missing_ids"},
		{name: "malformed address", change: func(r *quoteRequest) { r.TokenOut = "garbage" }, status: 204, result: "rejected", reason: "invalid_address", outLabel: "unknown"},
		{name: "unregistered address", change: func(r *quoteRequest) { r.TokenOut = common.HexToAddress("0x03").Hex() }, status: 204, result: "rejected", reason: "strategy-declined", outLabel: "other"},
		{name: "bad JSON", raw: `{"tokenIn":`, status: 400, result: "rejected", reason: "invalid-json", inLabel: "unknown", outLabel: "unknown"},
		{name: "unknown field even with pair", raw: `{"tokenIn":"` + in.Hex() + `","tokenOut":"` + out.Hex() + `","extra":true}`, status: 400, result: "rejected", reason: "invalid-json", inLabel: "unknown", outLabel: "unknown"},
		{name: "invalid amount", change: func(r *quoteRequest) { r.Amount = "bad" }, status: 204, result: "rejected", reason: "invalid-amount"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newQuoteTestSolver(t, in, &quoteTestStrategy{quote: tc.quote})
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
			s.metrics = metrics
			metrics.traffic.Initialize(1, []liquidlane.Route{{TokenIn: in, TokenOut: out, TokenInDecimals: 6, TokenOutDecimals: 6}}, nil)
			r := validQuoteRequest(in, out)
			if tc.change != nil {
				tc.change(&r)
			}
			body := tc.raw
			if body == "" {
				data, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				body = string(data)
			}
			w := httptest.NewRecorder()
			s.quoteHandler(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", strings.NewReader(body)))
			if w.Code != tc.status {
				t.Fatalf("HTTP=%d want=%d", w.Code, tc.status)
			}
			inLabel, outLabel := strings.ToLower(in.Hex()), strings.ToLower(out.Hex())
			if tc.inLabel != "" {
				inLabel = tc.inLabel
			}
			if tc.outLabel != "" {
				outLabel = tc.outLabel
			}
			metricstest.RequireFamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"token_in": inLabel, "token_out": outLabel, "result": tc.result, "reason": tc.reason}, 1)
			assertQuoteTrafficTotals(t, reg, 1)
		})
	}
}

func TestQuoteTrafficExcludesBreakersAndCountsErrorPaths(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, tc := range []struct {
		name, reason        string
		strategy            types.Strategy
		body                io.ReadCloser
		writeError, breaker bool
	}{
		{name: "breaker", breaker: true},
		{name: "strategy error", reason: "strategy_error", strategy: &errorQuoteStrategy{}},
		{name: "panic", reason: "internal_error", strategy: panicQuoteStrategy{}},
		{name: "write error", reason: "response_write_error", writeError: true},
		{name: "read error", reason: "read-body", body: io.NopCloser(errorQuoteReader{})},
		{name: "oversize", reason: "body-too-large", body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxQuoteRequestBytes+1)))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strategy := tc.strategy
			if strategy == nil {
				strategy = &quoteTestStrategy{quote: &types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
			}
			s := newBlockingQuoteTestSolver(t, in, strategy)
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
			s.metrics = metrics
			data, err := json.Marshal(validQuoteRequest(in, out))
			if err != nil {
				t.Fatal(err)
			}
			if tc.breaker {
				data = []byte(`{"blockUntilTimestamp":0}`)
			}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", bytes.NewReader(data))
			if tc.body != nil {
				r.Body = tc.body
			}
			w := httptest.NewRecorder()
			var writer http.ResponseWriter = w
			if tc.writeError {
				writer = errorQuoteWriter{w}
			}
			recoverQuoteServer(http.HandlerFunc(s.quoteHandler)).ServeHTTP(writer, r)
			if tc.breaker {
				assertQuoteTrafficTotals(t, reg, 0)
				return
			}
			assertQuoteTrafficTotals(t, reg, 1)
			families, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range families {
				if f.GetName() != "solver_bot_quote_results_total" {
					continue
				}
				for _, m := range f.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == "reason" && label.GetValue() != tc.reason {
							t.Fatalf("reason=%s want=%s", label.GetValue(), tc.reason)
						}
					}
				}
			}
			if tc.writeError {
				metricstest.RequireWorkflowEventCount(t, reg, Name, "quote", string(quoteOutcomeQuoted), 1)
			}
		})
	}
}

type errorQuoteReader struct{}

func (errorQuoteReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type errorQuoteWriter struct{ *httptest.ResponseRecorder }

func (errorQuoteWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func assertQuoteTrafficTotals(t *testing.T, reg prometheus.Gatherer, results float64) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	totals := map[string]float64{}
	for _, f := range families {
		if f.GetName() == "solver_bot_quote_results_total" {
			for _, m := range f.GetMetric() {
				totals[f.GetName()] += m.GetCounter().GetValue()
			}
		}
	}
	if totals["solver_bot_quote_results_total"] != results {
		t.Fatalf("totals=%v want results=%f", totals, results)
	}
}
