package rfq

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func TestHTTPMetrics_InstrumentRecordsRequest(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newRFQMetrics(reg, newStore(time.Now), "")
	if err != nil {
		t.Fatalf("newRFQMetrics: %v", err)
	}

	h := m.instrument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	labels := prometheus.Labels{"method": "POST", "route": "/quote", "status": "204"}
	got := metricstest.HistogramCount(t, m.duration.With(labels))
	if got != 1 {
		t.Fatalf("rfq_filler_http_request_duration_seconds_count{POST,/quote,204} = %d, want 1", got)
	}
	metricstest.RequireValue(t, m.requests.With(labels), 1)
}

func TestQuoteDecisionMetricsClassifyAuthenticatedRequestsOnce(t *testing.T) {
	tests := map[string]struct {
		configure func(*server)
		body      func() quoteRequest
		want      quoteDecisionOutcome
		wantCode  int
	}{
		"quoted": {
			want: quoteDecisionQuoted, wantCode: http.StatusOK,
		},
		"lane unavailable": {
			configure: func(s *server) { s.quotes.laneReady = func() bool { return false } },
			want:      quoteDecisionLaneUnavailable, wantCode: http.StatusNoContent,
		},
		"not quotable": {
			body: func() quoteRequest {
				body := validQuoteBody()
				body.TokenInChainID = 2
				return body
			},
			want: quoteDecisionNotQuotable, wantCode: http.StatusNoContent,
		},
		"below minimum": {
			configure: func(s *server) {
				s.quotes.minAmountsIn = map[common.Address]*big.Int{tIn: new(big.Int).Lsh(big.NewInt(1), 200)}
			},
			want: quoteDecisionBelowMinimum, wantCode: http.StatusNoContent,
		},
		"no candidates": {
			configure: func(s *server) {
				s.quotes.reader = &fakeQuoteCandidateReader{out: map[common.Address]*big.Int{}}
			},
			want: quoteDecisionNoCandidates, wantCode: http.StatusNoContent,
		},
		"strategy declined": {
			configure: func(s *server) {
				s.quotes.strategy = &inputRecordingStrategy{quoteOut: types.QuoteOutput{Decision: types.DecisionDecline}}
			},
			want: quoteDecisionStrategyDeclined, wantCode: http.StatusNoContent,
		},
		"error": {
			configure: func(s *server) { s.quotes.reader = failingQuoteReader{} },
			want:      quoteDecisionError, wantCode: http.StatusBadGateway,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			metrics, err := newRFQMetrics(reg, newStore(time.Now), "")
			if err != nil {
				t.Fatal(err)
			}
			metrics.now = func() time.Time { return time.Unix(123, 0) }
			srv := testServer()
			srv.metrics = metrics
			if test.configure != nil {
				test.configure(srv)
			}
			body := validQuoteBody()
			if test.body != nil {
				body = test.body()
			}

			rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body)
			if rr.Code != test.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, test.wantCode, rr.Body.String())
			}

			for _, outcome := range quoteDecisionOutcomes {
				count, timestamp := float64(0), float64(0)
				if outcome == test.want {
					count, timestamp = 1, 123
				}
				metricstest.RequireWorkflowEvent(t, reg, Name, "quote", string(outcome), count, timestamp)
			}
			if test.want == quoteDecisionQuoted {
				metricstest.RequireWorkflowAmount(
					t, reg, Name, "quote", tIn.Hex(), "input", 1_000_000_000_000_000_000,
				)
				metricstest.RequireWorkflowAmount(t, reg, Name, "quote", tOut.Hex(), "output", 1_000_000)
			}
		})
	}
}

func TestQuoteDecisionMetricsClassifyBadRequestSeparately(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, newStore(time.Now), "")
	if err != nil {
		t.Fatal(err)
	}
	metrics.now = func() time.Time { return time.Unix(123, 0) }
	srv := testServer()
	srv.metrics = metrics
	body := validQuoteBody()
	body.Type = "unsupported"

	output, err := srv.handleQuote(t.Context(), &quoteInput{Secret: testSecret, Body: body})
	if output != nil || err == nil {
		t.Fatalf("bad request result = (%v, %v), want nil/error", output, err)
	}
	metricstest.RequireWorkflowEvent(t, reg, Name, "quote", string(quoteDecisionBadRequest), 1, 123)
	metricstest.RequireWorkflowEvent(t, reg, Name, "quote", string(quoteDecisionError), 0, 0)
}

func TestQuoteDecisionMetricsIgnoreUnauthenticatedRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, newStore(time.Now), "")
	if err != nil {
		t.Fatal(err)
	}
	srv := testServer()
	srv.metrics = metrics

	rr := do(t, srv.handler(), http.MethodPost, "/quote", "wrong", validQuoteBody())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	for _, outcome := range quoteDecisionOutcomes {
		metricstest.RequireWorkflowEvent(t, reg, Name, "quote", string(outcome), 0, 0)
	}
}

func TestBoundedQuoteDecisionOutcomeFallsBackToError(t *testing.T) {
	if got := boundedQuoteDecisionOutcome(quoteDecisionOutcome("request-derived-value")); got != "error" {
		t.Fatalf("unknown outcome = %q, want error", got)
	}
}

type failingQuoteReader struct{}

func (failingQuoteReader) readQuoteCandidates(
	context.Context,
	[]solverInventory,
	common.Address,
	common.Address,
	*big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	return nil, errors.New("read failed")
}

func TestRouteLabel_BoundsCardinality(t *testing.T) {
	for path, want := range map[string]string{
		"/quote":        "/quote",
		"/health":       "/health",
		"/openapi.json": "/openapi.json",
		"/random/path":  "other",
		"/":             "other",
	} {
		if got := routeLabel(path); got != want {
			t.Fatalf("routeLabel(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestMethodLabel_BoundsCardinality(t *testing.T) {
	for method, want := range map[string]string{
		http.MethodGet:  http.MethodGet,
		http.MethodPost: http.MethodPost,
		http.MethodPut:  "other",
		"ARBITRARY":     "other",
	} {
		if got := methodLabel(method); got != want {
			t.Fatalf("methodLabel(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestRFQMetricsTrackUniqueWinsAndActiveOrders(t *testing.T) {
	now := time.Unix(1_000, 0)
	st := newStore(func() time.Time { return now })
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	metrics.now = func() time.Time { return now }
	exec := &executionService{
		executor:          common.HexToAddress("0x0000000000000000000000000000000000000010"),
		backend:           &fakeBackend{open: []backendOrder{{OrderID: "order-1"}}},
		store:             st,
		metrics:           metrics,
		orderPollObserver: metrics.orderPollObserver,
		log:               logr.Discard(),
		now:               func() time.Time { return now },
	}

	if err := exec.pollOpenOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := exec.pollOpenOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)

	metricstest.RequireWorkflowEvent(t, reg, Name, "order", "won", 1, 1_000)
	metricstest.RequireValue(t, metrics.activeOrders, 1)
	metricstest.RequireValue(t, metrics.oldestActive, 10)
	metricstest.RequireWorkflowEvent(t, reg, Name, "order_poll", "success", 2, 1_000)
	metricstest.RequireExternalOperationCount(t, reg, Name, orderPollOperation, "success", 2)
}

func TestRFQMetricsFailedPollKeepsLastSuccess(t *testing.T) {
	now := time.Unix(1_000, 0)
	st := newStore(func() time.Time { return now })
	reg := prometheus.NewRegistry()
	metrics, err := newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	exec := &executionService{
		executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		backend:  backend,
		store:    st,
		metrics:  metrics,
		log:      logr.Discard(),
		now:      func() time.Time { return now },
	}
	if err := exec.pollOpenOrders(t.Context()); err != nil {
		t.Fatalf("successful poll: %v", err)
	}

	now = now.Add(time.Minute)
	backend.orderListErr = errors.New("backend unavailable")
	if err := exec.pollOpenOrders(t.Context()); err == nil {
		t.Fatal("failed poll returned nil")
	}
	metricstest.RequireWorkflowEvent(t, reg, Name, "order_poll", "success", 1, 1_000)
}
