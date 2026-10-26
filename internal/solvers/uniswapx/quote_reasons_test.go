package uniswapx

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
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func TestQuoteValidationReasonsAndPrecedence(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	tests := []struct {
		name   string
		change func(*quoteRequest)
		want   quoteDeclineReason
	}{
		{"request ID", func(r *quoteRequest) { r.RequestID = "" }, quoteDeclineMissingIDs},
		{"quote ID", func(r *quoteRequest) { r.QuoteID = "" }, quoteDeclineMissingIDs},
		{"type", func(r *quoteRequest) { r.Type = "OTHER" }, quoteDeclineUnsupportedType},
		{"missing type", func(r *quoteRequest) { r.Type = "" }, quoteDeclineUnsupportedType},
		{"outputs", func(r *quoteRequest) { r.NumOutputs = 0 }, quoteDeclineInvalidOutputs},
		{"negative outputs", func(r *quoteRequest) { r.NumOutputs = -1 }, quoteDeclineInvalidOutputs},
		{"v3", func(r *quoteRequest) { r.Protocol = "v3" }, quoteDeclineUnsupportedProtocol},
		{"missing protocol", func(r *quoteRequest) { r.Protocol = "" }, quoteDeclineUnsupportedProtocol},
		{"other protocol", func(r *quoteRequest) { r.Protocol = "v4" }, quoteDeclineUnsupportedProtocol},
		{"input chain", func(r *quoteRequest) { r.TokenInChainID = 42161 }, quoteDeclineUnsupportedChain},
		{"output chain", func(r *quoteRequest) { r.TokenOutChainID = 8453 }, quoteDeclineUnsupportedChain},
		{"missing chain", func(r *quoteRequest) { r.TokenInChainID = 0 }, quoteDeclineUnsupportedChain},
		{"swapper", func(r *quoteRequest) { r.Swapper = "bad" }, quoteDeclineInvalidAddress},
		{"input address", func(r *quoteRequest) { r.TokenIn = "" }, quoteDeclineInvalidAddress},
		{"output address", func(r *quoteRequest) { r.TokenOut = "0x1234" }, quoteDeclineInvalidAddress},
		{"identical pair", func(r *quoteRequest) { r.TokenOut = r.TokenIn }, quoteDeclinePairOutOfScope},
		{"native output", func(r *quoteRequest) { r.TokenOut = (common.Address{}).Hex() }, quoteDeclinePairOutOfScope},
		{"missing amount", func(r *quoteRequest) { r.Amount = "" }, quoteDeclineInvalidAmount},
		{"zero amount", func(r *quoteRequest) { r.Amount = "0" }, quoteDeclineInvalidAmount},
		{"negative amount", func(r *quoteRequest) { r.Amount = "-1" }, quoteDeclineInvalidAmount},
		{"malformed amount", func(r *quoteRequest) { r.Amount = "one" }, quoteDeclineInvalidAmount},
		{"IDs before type", func(r *quoteRequest) { r.QuoteID = ""; r.Type = "" }, quoteDeclineMissingIDs},
		{"type before outputs", func(r *quoteRequest) { r.Type = ""; r.NumOutputs = 0 }, quoteDeclineUnsupportedType},
		{"outputs before protocol", func(r *quoteRequest) { r.NumOutputs = 0; r.Protocol = "v3" }, quoteDeclineInvalidOutputs},
		{"protocol before chain", func(r *quoteRequest) { r.Protocol = "v3"; r.TokenInChainID = 8453 }, quoteDeclineUnsupportedProtocol},
		{"chain before address", func(r *quoteRequest) { r.TokenOutChainID = 8453; r.TokenOut = "bad" }, quoteDeclineUnsupportedChain},
		{"address before pair", func(r *quoteRequest) { r.Swapper = "bad"; r.TokenOut = r.TokenIn }, quoteDeclineInvalidAddress},
		{"pair before amount", func(r *quoteRequest) { r.TokenOut = r.TokenIn; r.Amount = "" }, quoteDeclinePairOutOfScope},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			strategy := &quoteTestStrategy{}
			s := newQuoteTestSolver(t, in, strategy)
			r := validQuoteRequest(in, out)
			tc.change(&r)
			response, err := s.quote(t.Context(), r)
			if err != nil || response.declineReason != tc.want || len(strategy.inputs) != 0 {
				t.Fatalf("response=%+v err=%v calls=%d; want %s", response, err, len(strategy.inputs), tc.want)
			}
			if quoteDeclineDecision(tc.want) == "bad_request" && metricOutcomeForDecline(tc.want) == quoteOutcomeDeclinedUnknown {
				t.Fatal("semantic failure has no metric mapping")
			}
		})
	}
	t.Run("blocked still first", func(t *testing.T) {
		s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
		s.warmupUntil.Store(time.Now().Add(time.Hour).Unix())
		response, err := s.quote(t.Context(), quoteRequest{})
		if err != nil || response.declineReason != quoteDeclineBlocked {
			t.Fatalf("response=%+v err=%v", response, err)
		}
	})
	t.Run("token policy", func(t *testing.T) {
		s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
		policy, err := tokenpolicy.New(tokenpolicy.Permissioned, []common.Address{out})
		if err != nil {
			t.Fatal(err)
		}
		s.cfg.TokenPolicy = policy
		response, err := s.quote(t.Context(), validQuoteRequest(in, out))
		if err != nil || response.declineReason != quoteDeclinePairOutOfScope {
			t.Fatalf("response=%+v err=%v", response, err)
		}
	})
}

func TestQuoteSupportedRequestsRemainAccepted(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, protocol := range []string{"v1", "v2"} {
		for _, mode := range []string{quoteTypeExactInput, quoteTypeExactOutput} {
			t.Run(protocol+mode, func(t *testing.T) {
				strategy := &quoteTestStrategy{quote: &types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(100)}}
				s := newQuoteTestSolver(t, in, strategy)
				// Chain applicability is equality with configured chain, not a new mainnet-only gate.
				s.chainID = 42161
				r := validQuoteRequest(in, out)
				r.Protocol, r.Type = protocol, mode
				r.Swapper = (common.Address{}).Hex() // Upstream intentionally anonymizes the swapper.
				r.TokenInChainID, r.TokenOutChainID = 42161, 42161
				response, err := s.quote(t.Context(), r)
				if err != nil || response.AmountOut != "100" || response.declineReason != "" || len(strategy.inputs) != 1 {
					t.Fatalf("response=%+v err=%v", response, err)
				}
			})
		}
	}
}

func TestQuoteLocalPlannerReportsRealDeclinesWithoutDebugLogging(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, name := range []string{types.DefaultName, types.SingleName} {
		for _, mode := range []string{quoteTypeExactInput, quoteTypeExactOutput} {
			for _, tc := range []struct {
				name     string
				capacity int64
				want     quoteDeclineReason
			}{
				{"no routes", 0, quoteDeclineNoMatchingRoutes},
				{"capacity", 50, quoteDeclineInsufficientCapacity},
			} {
				t.Run(name+mode+tc.name, func(t *testing.T) {
					local, err := defaultstrategy.New(defaultstrategy.Config{Name: name})
					if err != nil {
						t.Fatal(err)
					}
					s := newBlockingQuoteTestSolver(t, in, local)
					state := s.quoteState.Load()
					state.maxFeePerGas = new(big.Int)
					if tc.capacity > 0 {
						state.inventory = []liquidlane.Inventory{liquidlane.DirectInventory(liquidlane.Route{
							ID: "route", CapacityID: "budget", TokenIn: in, TokenOut: out,
						}, big.NewInt(tc.capacity), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))}
					}
					r := validQuoteRequest(in, out)
					r.Type = mode
					response, err := s.quote(t.Context(), r)
					if err != nil || response.declineReason != tc.want {
						t.Fatalf("response=%+v err=%v; want %s", response, err, tc.want)
					}
				})
			}
		}
	}
}

func TestStrategyDeclineCaptureIsBoundedAndOrderIndependent(t *testing.T) {
	for _, reasons := range [][]string{
		{"insufficient-capacity"}, {"insufficient-capacity", "insufficient-capacity"},
		{"insufficient-capacity", "gas-exceeds-output"}, {"gas-exceeds-output", "insufficient-capacity"},
		{"arbitrary request text"}, nil,
	} {
		d := &quoteStrategyDeclines{}
		trace := d.trace(nil)
		for _, reason := range reasons {
			trace.Decline("quote", reason)
		}
		want := quoteDeclineStrategy
		if len(reasons) > 0 && reasons[0] == "insufficient-capacity" && (len(reasons) == 1 || reasons[1] == reasons[0]) {
			want = quoteDeclineInsufficientCapacity
		}
		if got := d.reason(); got != want {
			t.Fatalf("reasons=%v got=%s want=%s", reasons, got, want)
		}
	}
	for _, reason := range []string{"no-matching-routes", "no-candidates", "amount-below-minimum", "no-allocation", "gas-exceeds-output", "minimum-input-cannot-cover-output"} {
		if got := strategyQuoteDeclineReason(reason); got == quoteDeclineStrategy || boundedQuoteRejectionReason(string(got)) == "unknown" {
			t.Fatalf("unmapped %q", reason)
		}
	}
}

func TestQuoteHTTPReasonsKeepExistingOutcomesAndLogs(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, tc := range []struct {
		name, raw, reason string
		status            int
		outcome           quoteMetricOutcome
	}{
		{"syntax", `{`, "invalid-json", 400, quoteOutcomeInvalid},
		{"wrong JSON type", `{"amount":100}`, "invalid-json", 400, quoteOutcomeInvalid},
		{"trailing", `{} {}`, "trailing-json", 400, quoteOutcomeInvalid},
		{"unknown field", `{"unexpected":true}`, "invalid-json", 400, quoteOutcomeInvalid},
		{"missing IDs", `{}`, "missing_ids", 400, quoteOutcomeInvalid},
		{"null", `null`, "missing_ids", 400, quoteOutcomeInvalid},
		{"bad breaker", `{"blockUntilTimestamp":-1}`, "invalid-breaker-notification", 400, quoteOutcomeInvalid},
		{"semantic protocol", "", "unsupported_protocol", 204, quoteOutcomeDeclinedInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newQuoteTestSolver(t, in, &quoteTestStrategy{})
			var logs []string
			log := funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{Verbosity: 1})
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
			s.metrics = metrics
			raw := tc.raw
			if raw == "" {
				r := validQuoteRequest(in, out)
				r.Protocol = "v3"
				b, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				raw = string(b)
			}
			r := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", strings.NewReader(raw))
			w := httptest.NewRecorder()
			s.quoteHandler(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "quote", string(tc.outcome), 1)
			if got := metricstest.FamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"reason": tc.reason}); got != 1 {
				t.Fatalf("reason counter=%f", got)
			}
			if !strings.Contains(strings.Join(logs, "\n"), `"reason":"`+tc.reason+`"`) {
				t.Fatalf("logs=%v", logs)
			}
			if tc.status == 204 && w.Body.Len() != 0 {
				t.Fatal("decline changed wire body")
			}
		})
	}
}

type errorQuoteStrategy struct{ quoteTestStrategy }

func (*errorQuoteStrategy) DecideQuote(context.Context, types.QuoteInput) (*types.Quote, error) {
	return nil, errors.New("request-specific backend error")
}

func TestQuoteStrategyErrorIsNotNilDecline(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	s := newBlockingQuoteTestSolver(t, in, &errorQuoteStrategy{})
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
	s.metrics = metrics
	body := new(bytes.Buffer)
	if err := json.NewEncoder(body).Encode(validQuoteRequest(in, out)); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.quoteHandler(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", body))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "quote", string(quoteOutcomeError), 1)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "quote", string(quoteOutcomeDeclinedStrategy), 0)
	if got := metricstest.FamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"reason": "strategy_error"}); got != 1 {
		t.Fatalf("counter=%f", got)
	}
}

func TestQuoteReasonMetricCannotExposeUnboundedLabels(t *testing.T) {
	s := &Solver{}
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
	s.metrics = metrics
	s.metrics.traffic.Observe(liquidlane.QuoteMetricRequest{}, false, "request-id-or-address")
	s.metrics.traffic.Observe(liquidlane.QuoteMetricRequest{}, false, "other arbitrary error")
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "solver_bot_quote_results_total" {
			continue
		}
		if len(family.GetMetric()) != 1 {
			t.Fatalf("unbounded series: %v", family)
		}
		m := family.GetMetric()[0]
		if len(m.GetLabel()) != 5 || m.GetCounter().GetValue() != 2 {
			t.Fatalf("metric=%v", m)
		}
	}
}

func TestQuotePlannerRejectionMetricsAndPairLogs(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, tc := range []struct {
		name      string
		minAmount string
		gas       bool
		want      quoteDeclineReason
	}{
		{"minimum", "200", false, quoteDeclineAmountBelowMinimum},
		{"gas floor", "", true, quoteDeclineGasExceedsOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, err := defaultstrategy.New(defaultstrategy.Config{MinAmount: tc.minAmount})
			if err != nil {
				t.Fatal(err)
			}
			s := newBlockingQuoteTestSolver(t, in, local)
			state := s.quoteState.Load()
			route := liquidlane.Route{ID: "route", CapacityID: "budget", TokenIn: in, TokenOut: out}
			state.inventory = []liquidlane.Inventory{liquidlane.DirectInventory(route, big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))}
			state.maxFeePerGas = new(big.Int)
			if tc.gas {
				state.maxFeePerGas.SetInt64(1)
				state.gasPrices = liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{out: new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)})
			}
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, s)
			s.metrics = metrics
			var logs []string
			log := funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{Verbosity: 1})
			request := validQuoteRequest(in, out)
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			s.quoteHandler(w, httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", bytes.NewReader(body)))
			if w.Code != http.StatusNoContent {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "quote", string(quoteOutcomeDeclinedStrategy), 1)
			if got := metricstest.FamilyValue(t, reg, "solver_bot_quote_results_total", map[string]string{"reason": string(tc.want)}); got != 1 {
				t.Fatalf("reason counter=%f logs=%v", got, logs)
			}
			var decline map[string]any
			for _, line := range logs {
				var fields map[string]any
				if err := json.Unmarshal([]byte(line), &fields); err != nil {
					t.Fatal(err)
				}
				if fields["msg"] == "quote declined" {
					decline = fields
				}
			}
			if decline["reason"] != string(tc.want) || decline["tokenIn"] != in.Hex() || decline["tokenOut"] != out.Hex() || decline["requestId"] != request.RequestID || decline["quoteId"] != request.QuoteID || decline["protocol"] != request.Protocol {
				t.Fatalf("decline=%v logs=%v", decline, logs)
			}
		})
	}
}
