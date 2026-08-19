package uniswapx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestQuoteHandlerRecordsDetailedDeclineOutcome(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	solver := newQuoteTestSolver(t, tokenIn, &quoteTestStrategy{})
	solver.log = logr.Discard()
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, solver)
	solver.metrics = metrics
	solver.metrics.now = func() time.Time { return time.Unix(123, 0) }
	body := new(bytes.Buffer)
	if err := json.NewEncoder(body).Encode(validQuoteRequest(tokenIn, tokenOut)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", body)
	response := httptest.NewRecorder()

	solver.quoteHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	metricstest.RequireWorkflowEvent(
		t, reg, Name, "quote", string(quoteOutcomeDeclinedStrategy), 1, 123,
	)
}

func TestQuoteMetricsRecordAmountsAndFreshness(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	solver := &Solver{}
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, solver)
	solver.metrics = metrics
	solver.metrics.now = func() time.Time { return time.Unix(123, 0) }
	solver.observeQuote(quoteOutcomeQuoted)
	solver.observeQuotedAmounts(quoteResponse{
		TokenIn: tokenIn.Hex(), AmountIn: "100", TokenOut: tokenOut.Hex(), AmountOut: "90",
	})

	metricstest.RequireWorkflowAmount(t, reg, Name, "quote", tokenIn.Hex(), "input", 100)
	metricstest.RequireWorkflowAmount(t, reg, Name, "quote", tokenOut.Hex(), "output", 90)
	metricstest.RequireWorkflowEvent(t, reg, Name, "quote", string(quoteOutcomeQuoted), 1, 123)
}

func TestQuoteDeclineMetricsUseBoundedOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		reason  quoteDeclineReason
		outcome quoteMetricOutcome
	}{
		{name: "blocked", reason: quoteDeclineBlocked, outcome: quoteOutcomeDeclinedBlocked},
		{name: "invalid request", reason: quoteDeclineInvalidRequest, outcome: quoteOutcomeDeclinedInvalidRequest},
		{name: "pair out of scope", reason: quoteDeclinePairOutOfScope, outcome: quoteOutcomeDeclinedPairOutOfScope},
		{name: "invalid amount", reason: quoteDeclineInvalidAmount, outcome: quoteOutcomeDeclinedInvalidAmount},
		{
			name: "quote state unavailable", reason: quoteDeclineQuoteStateUnavailable,
			outcome: quoteOutcomeDeclinedQuoteState,
		},
		{name: "strategy", reason: quoteDeclineStrategy, outcome: quoteOutcomeDeclinedStrategy},
		{name: "state changed", reason: quoteDeclineStateChanged, outcome: quoteOutcomeDeclinedStateChanged},
		{
			name: "unknown stays bounded", reason: quoteDeclineReason("request-controlled-text"),
			outcome: quoteOutcomeDeclinedUnknown,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := metricOutcomeForDecline(test.reason); got != test.outcome {
				t.Fatalf("metricOutcomeForDecline(%q) = %q, want %q", test.reason, got, test.outcome)
			}
		})
	}
}
