package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func pairLabels(event, result, reason string) map[string]string {
	return map[string]string{"solver": Name, "chain_id": "1", "token_in": strings.ToLower(tIn.Hex()), "token_out": strings.ToLower(tOut.Hex()), "event": event, "result": result, "reason": reason}
}

func requirePairSeries(t *testing.T, reg *prometheus.Registry, want int) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range families {
		if f.GetName() == "solver_bot_pair_events_total" {
			n = len(f.GetMetric())
		}
	}
	if n != want {
		t.Fatalf("pair series=%d want=%d", n, want)
	}
}

func TestPairQuoteFinalWriteAndConfiguredDirection(t *testing.T) {
	for _, mode := range []string{"success", "failure", "short", "panic", "zero liquidity", "reverse", "wrong chain", "policy", "new trusted direction"} {
		t.Run(mode, func(t *testing.T) {
			srv := testServer()
			reg := prometheus.NewRegistry()
			var err error
			srv.metrics, err = newRFQMetrics(reg, newStore(time.Now), "")
			if err != nil {
				t.Fatal(err)
			}
			route := liquidlane.NewRoute(1, vlt, common.Address{}, tIn, tOut, 18, 6)
			routes := []liquidlane.Route{route}
			srv.quotes.pairRoutes.Store(&routes)
			body := validQuoteBody()
			writerMode := "success"
			reason := "none"
			result := "returned"
			want := 1
			switch mode {
			case "failure", "short":
				writerMode = mode
				reason = "response_write_error"
				result = "rejected"
			case "panic":
				writerMode = mode
				reason = "internal_error"
				result = "rejected"
			case "zero liquidity":
				body.Adapters = nil
				reason = "no_candidates"
				result = "rejected"
			case "reverse":
				body.TokenIn, body.TokenOut = body.TokenOut, body.TokenIn
				body.Adapters = nil
				want = 0
			case "wrong chain":
				body.TokenInChainID = 2
				want = 0
			case "policy":
				srv.quotes.tokenPolicy = testPermissionedPolicy(t, common.HexToAddress("0x999"))
				want = 0
			case "new trusted direction":
				srv.quotes.pairRoutes.Store(nil) // current candidate proof is sufficient, metadata is nil
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", bytes.NewReader(raw))
			req.Header.Set(sharedSecretHeader, testSecret)
			srv.handler().ServeHTTP(returnedEventWriter{ResponseRecorder: httptest.NewRecorder(), mode: writerMode}, req)
			requirePairSeries(t, reg, want)
			if want > 0 {
				metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairLabels("quote", result, reason), 1)
			}
		})
	}
}

func TestUnsupportedRequestsDoNotCreatePairSeries(t *testing.T) {
	srv := testServer()
	reg := prometheus.NewRegistry()
	var err error
	srv.metrics, err = newRFQMetrics(reg, newStore(time.Now), "")
	if err != nil {
		t.Fatal(err)
	}
	routes := []liquidlane.Route{liquidlane.NewRoute(1, vlt, common.Address{}, tIn, tOut, 18, 6)}
	srv.quotes.pairRoutes.Store(&routes)
	reader := srv.quotes.reader.(*fakeQuoteCandidateReader)
	for i := range 100 {
		body := validQuoteBody()
		body.TokenIn = common.BigToAddress(big.NewInt(int64(1000 + i))).Hex()
		body.Adapters = nil
		do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body)
	}
	requirePairSeries(t, reg, 0)
	if len(reader.queries) != 0 {
		t.Fatalf("unsupported requests read candidates: %d", len(reader.queries))
	}
}

func TestPairAssignmentDedupAndOwnedReceipt(t *testing.T) {
	st, backend := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, backend, txm)
	reg := prometheus.NewRegistry()
	var err error
	e.metrics, err = newRFQMetrics(reg, st, "")
	if err != nil {
		t.Fatal(err)
	}
	syncCycle(t.Context(), e)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairLabels("assignment", "observed", "none"), 1)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairLabels("fill", "mined_success", "none"), 1)
	req := strategyRequest{TokenIn: tIn, TokenOut: tOut, Amount: big.NewInt(1)}
	candidates := []liquidlane.QuoteCandidate{{Route: liquidlane.NewRoute(1, vlt, common.Address{}, tIn, tOut, 18, 6)}}
	if !e.observeAwardPair(t.Context(), "o1", req, big.NewInt(1), candidates) {
		t.Fatal("repeat lost current route proof")
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairLabels("assignment", "observed", "none"), 1)
	requirePairSeries(t, reg, 2)
}

type pairRouteReader struct {
	*fakeRecoveryReader

	started chan struct{}
	release chan struct{}
	stopped chan struct{}
	routes  []liquidlane.Route
	err     error
}

func (r *pairRouteReader) resolvePairRoutes(ctx context.Context, _ []common.Address) ([]liquidlane.Route, error) {
	close(r.started)
	defer close(r.stopped)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return r.routes, r.err
	}
}

func TestPairRouteWarmupIsOptionalAsyncAndCancellable(t *testing.T) {
	for _, mode := range []string{"success", "failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			srv := testServer()
			reg := prometheus.NewRegistry()
			var err error
			srv.metrics, err = newRFQMetrics(reg, newStore(time.Now), "")
			if err != nil {
				t.Fatal(err)
			}
			r := &pairRouteReader{fakeRecoveryReader: &fakeRecoveryReader{}, started: make(chan struct{}), release: make(chan struct{}), stopped: make(chan struct{}), routes: []liquidlane.Route{liquidlane.NewRoute(1, vlt, common.Address{}, tIn, tOut, 18, 6)}}
			if mode == "failure" {
				r.err = errors.New("offline")
			}
			s := &Solver{cfg: &Config{Adapters: []recoveryVault{{Adapter: vlt}}}, server: srv, exec: &executionService{reader: r}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s.warmPairRoutes(ctx)
			select {
			case <-r.started:
			case <-time.After(time.Second):
				t.Fatal("background route read did not start")
			}
			// Native HTTP remains usable while optional observation RPC is blocked; current candidate proof works.
			if got := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody()).Code; got != http.StatusOK {
				t.Fatalf("HTTP while warming=%d", got)
			}
			if mode == "cancel" {
				cancel()
			} else {
				close(r.release)
			}
			select {
			case <-r.stopped:
			case <-time.After(time.Second):
				t.Fatal("background route read did not stop")
			}
			deadline := time.Now().Add(time.Second)
			for mode == "success" && srv.quotes.pairRoutes.Load() == nil && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if got := srv.quotes.configuredPairInScope(ptrQuote(validQuoteBody())); got != (mode == "success") {
				t.Fatalf("configured direction=%v mode=%s", got, mode)
			}
			if mode == "success" {
				r.routes[0].TokenIn = common.Address{}
				if !srv.quotes.configuredPairInScope(ptrQuote(validQuoteBody())) {
					t.Fatal("published route slice aliases resolver storage")
				}
			}
		})
	}
}
func ptrQuote(q quoteRequest) *quoteRequest { return &q }

// Strategy panics happen before a quoteDecision returns, unlike a response-writer panic.
// Only an already trusted configured direction may survive that boundary as pair evidence.
func TestKnownPairStrategyPanicMetrics(t *testing.T) {
	for _, mode := range []string{"configured", "unknown input", "no configured proof", "foreign route chain"} {
		t.Run(mode, func(t *testing.T) {
			srv := testServer()
			reg := prometheus.NewRegistry()
			var err error
			srv.metrics, err = newRFQMetrics(reg, newStore(time.Now), "")
			if err != nil {
				t.Fatal(err)
			}
			srv.quotes.strategy = panicStrategy{}
			body := validQuoteBody()
			routes := []liquidlane.Route{liquidlane.NewRoute(1, vlt, common.Address{}, tIn, tOut, 18, 6)}
			want := 1
			switch mode {
			case "unknown input":
				body.TokenIn = common.HexToAddress("0xbeef").Hex()
				want = 0
			case "no configured proof":
				routes = nil
				want = 0
			case "foreign route chain":
				routes[0].ID = liquidlane.NewRouteID(10, vlt, tIn, tOut)
				want = 0
			}
			srv.quotes.pairRoutes.Store(&routes)
			response := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d want500", response.Code)
			}
			metricstest.RequireFamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"solver": Name, "result": "rejected", "reason": "internal_error"}, 1)
			requirePairSeries(t, reg, want)
			if want == 1 {
				metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairLabels("quote", "rejected", "internal_error"), 1)
			}
		})
	}
}
