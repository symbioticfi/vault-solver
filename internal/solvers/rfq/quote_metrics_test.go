package rfq

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestRFQPairMetricsCountFinalHTTPResults(t *testing.T) {
	for _, tc := range []struct {
		name           string
		change         func(*quoteRequest)
		secret, raw    string
		status         int
		result, reason string
		schema         bool
	}{
		{name: "returned", status: 200, result: "returned", reason: "none"},
		{name: "foreign chain", change: func(q *quoteRequest) { q.TokenInChainID = 2 }, status: 204, result: "rejected", reason: "unsupported_chain"},
		{name: "no inventory", change: func(q *quoteRequest) { q.Adapters = nil }, status: 204, result: "rejected", reason: "no_candidates"},
		{name: "unauthorized", secret: "wrong", status: 403, result: "rejected", reason: "unauthorized"},
		{name: "unsupported protocol", change: func(q *quoteRequest) { q.Protocol = "v3" }, status: 422, result: "rejected", reason: "unsupported_protocol", schema: true},
		{name: "unsupported type precedence", change: func(q *quoteRequest) { q.Type = "EXACT_OUTPUT"; q.Protocol = "v3" }, status: 422, result: "rejected", reason: "unsupported_type", schema: true},
		{name: "missing ID", change: func(q *quoteRequest) { q.QuoteID = "" }, status: 422, result: "rejected", reason: "missing_ids", schema: true},
		{name: "invalid ID", change: func(q *quoteRequest) { q.QuoteID = "bad" }, status: 422, result: "rejected", reason: "invalid_ids", schema: true},
		{name: "malformed address", change: func(q *quoteRequest) { q.TokenIn = "bad" }, status: 422, result: "rejected", reason: "invalid_address", schema: true},
		{name: "malformed amount", change: func(q *quoteRequest) { q.Amount = "bad" }, status: 422, result: "rejected", reason: "invalid_amount", schema: true},
		{name: "raw JSON", raw: "{", status: 400, result: "rejected", reason: "invalid_json", schema: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m, err := newRFQMetrics(reg, newStore(time.Now), "")
			if err != nil {
				t.Fatal(err)
			}
			m.traffic.Initialize(1, []liquidlane.Route{{TokenIn: tIn, TokenOut: tOut, TokenInDecimals: 18, TokenOutDecimals: 6}}, nil)
			srv := testServer()
			srv.metrics = m
			body := validQuoteBody()
			if tc.change != nil {
				tc.change(&body)
			}
			secret := testSecret
			if tc.secret != "" {
				secret = tc.secret
			}
			var response *httptest.ResponseRecorder
			if tc.raw != "" {
				r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", strings.NewReader(tc.raw))
				response = httptest.NewRecorder()
				srv.handler().ServeHTTP(response, r)
			} else {
				response = do(t, srv.handler(), http.MethodPost, "/quote", secret, body)
			}
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body)
			}
			in, out := lowerAddr(tIn), lowerAddr(tOut)
			if tc.schema {
				in, out = "unknown", "unknown"
			}
			if tc.name == "foreign chain" {
				in = "other"
			}
			metricstest.RequireFamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"solver": Name, "token_in": in, "token_out": out, "result": tc.result, "reason": tc.reason}, 1)
			families, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			var total float64
			for _, f := range families {
				if f.GetName() == "solver_bot_quote_results_total" {
					for _, sample := range f.GetMetric() {
						total += sample.GetCounter().GetValue()
					}
				}
			}
			if total != 1 {
				t.Fatalf("double-counted result: %f", total)
			}
		})
	}
}

func TestRFQPairMetricsExcludeNonQuoteAndInternalCalls(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newRFQMetrics(reg, newStore(time.Now), "")
	if err != nil {
		t.Fatal(err)
	}
	srv := testServer()
	srv.metrics = m
	do(t, srv.handler(), http.MethodGet, "/health", "", nil)
	q := validQuoteBody()
	if _, err := srv.quotes.quote(t.Context(), &q); err != nil {
		t.Fatal(err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "solver_bot_quote_results_total" {
			t.Fatal("non-transport activity counted as HTTP quotes")
		}
	}
}

type failingQuoteWriter struct{ *httptest.ResponseRecorder }

func (failingQuoteWriter) Write([]byte) (int, error) { return 0, errors.New("response write failed") }

func TestRFQPairMetricsObserveTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name, reason                            string
		panicStrategy, backendError, writeError bool
		media                                   string
	}{
		{name: "panic", reason: "internal_error", panicStrategy: true},
		{name: "dependency", reason: "error", backendError: true},
		{name: "write", reason: "response_write_error", writeError: true},
		{name: "media", reason: "unsupported_media_type", media: "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m, err := newRFQMetrics(reg, newStore(time.Now), "")
			if err != nil {
				t.Fatal(err)
			}
			srv := testServer()
			srv.metrics = m
			if tc.panicStrategy {
				srv.quotes.strategy = panicStrategy{}
			}
			if tc.backendError {
				srv.quotes.reader = failingQuoteReader{}
			}
			body, err := json.Marshal(validQuoteBody())
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", bytes.NewReader(body))
			request.Header.Set(sharedSecretHeader, testSecret)
			if tc.media != "" {
				request.Header.Set("Content-Type", tc.media)
			}
			writer := httptest.NewRecorder()
			var response http.ResponseWriter = writer
			if tc.writeError {
				response = failingQuoteWriter{writer}
			}
			srv.handler().ServeHTTP(response, request)
			metricstest.RequireFamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"result": "rejected", "reason": tc.reason}, 1)
			if tc.panicStrategy && writer.Code != 500 {
				t.Fatalf("panic status=%d", writer.Code)
			}
			if tc.backendError && writer.Code != 502 {
				t.Fatalf("dependency status=%d", writer.Code)
			}
		})
	}
}
