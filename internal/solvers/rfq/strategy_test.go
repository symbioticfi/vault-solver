package rfq

import (
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/webhook"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func baseQuoteInput(t *testing.T) types.QuoteInput {
	t.Helper()
	return types.QuoteInput{
		RequestID: "r", QuoteID: "q", ChainID: 1,
		Executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
		TokenIn:  tIn, TokenOut: tOut, AmountIn: mustBig(t, "1000000000000000000"),
		Candidates: []types.QuoteCandidate{{
			ID: "c0", Adapter: vlt, Asset: tOut, AssetDecimals: 6,
			MaxAssets: mustBig(t, "10000000"),
			MaxRate:   mustBig(t, "1000000000000000000"),
		}},
		Now: time.Unix(0, 0),
	}
}

func TestDefaultStrategyDecideQuote(t *testing.T) {
	pricing := &fakeStrategyPricing{
		decimals: 18,
		out:      map[common.Address]*big.Int{tOut: mustBig(t, "1000000")},
	}
	out, err := defaultstrategy.New(pricing).DecideQuote(t.Context(), baseQuoteInput(t))
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if out.Decision != types.DecisionQuote || out.QuotedAmountOut.String() != "1000000" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(out.Legs) != 1 || out.Legs[0].CandidateID != "c0" {
		t.Fatalf("legs = %+v, want candidate c0", out.Legs)
	}
	if len(pricing.queries) != 1 || len(pricing.queries[0]) != 1 || pricing.queries[0][0].Adapter != vlt {
		t.Fatalf("pricing queries = %+v, want one batched query for %s", pricing.queries, vlt.Hex())
	}
}

func TestNewStrategyUsesRegistry(t *testing.T) {
	got, err := newStrategy(StrategyConfig{Name: "default"}, nil, logr.Discard())
	if err != nil {
		t.Fatalf("newStrategy default: %v", err)
	}
	if got == nil {
		t.Fatal("newStrategy default returned nil")
	}
	names := strategies.Registered()
	if len(names) < 2 || names[0] != "default" || names[1] != "webhook" {
		t.Fatalf("registered strategies = %v, want default and webhook", names)
	}
}

func TestDefaultStrategyBuildFillPlanUsesQuoteCache(t *testing.T) {
	strategy := defaultstrategy.New(&fakeStrategyPricing{
		decimals: 18,
		out:      map[common.Address]*big.Int{tOut: mustBig(t, "1000000")},
	})
	input := baseQuoteInput(t)
	if _, err := strategy.DecideQuote(t.Context(), input); err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	plan, err := strategy.BuildFillPlan(t.Context(), types.FillInput{
		RequestID: input.RequestID,
		QuoteID:   input.QuoteID,
		ChainID:   input.ChainID,
		Executor:  input.Executor,
		TokenIn:   input.TokenIn,
		TokenOut:  input.TokenOut,
		AmountIn:  input.AmountIn,
		Now:       input.Now,
	})
	if err != nil {
		t.Fatalf("BuildFillPlan: %v", err)
	}
	if plan == nil || len(plan.Legs) != 1 || plan.Legs[0].Adapter != vlt {
		t.Fatalf("cached fill plan = %+v, want vlt leg", plan)
	}
}

func TestWebhookStrategyDecodesLowerCamelResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if !strings.Contains(string(body), `"amountIn":"1000000000000000000"`) ||
			strings.Contains(string(body), `"AmountIn"`) {
			t.Fatalf("request body does not use decimal-string lower-camel JSON: %s", string(body))
		}
		_, _ = w.Write([]byte(`{
			"decision": "quote",
			"quotedAmountOut": "1000000",
			"legs": [{"candidateId": "c0", "amountIn": "1000000000000000000", "amountOut": "1000000"}]
		}`))
	}))
	defer srv.Close()
	client, err := webhook.NewClient(webhook.Config{URL: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	out, err := webhookstrategy.New(client).DecideQuote(t.Context(), baseQuoteInput(t))
	if err != nil {
		t.Fatalf("DecideQuote: %v", err)
	}
	if out.Decision != types.DecisionQuote || out.QuotedAmountOut.String() != "1000000" ||
		len(out.Legs) != 1 || out.Legs[0].CandidateID != "c0" {
		t.Fatalf("unexpected webhook output: %+v", out)
	}
}

func TestQuoteRejectsWebhookMultiLegPlanForPermissionedScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if !strings.Contains(string(body), `"requireSingleRoute":true`) {
			t.Fatalf("webhook request missing single-route constraint: %s", body)
		}
		_, _ = w.Write([]byte(`{
			"decision": "quote",
			"quotedAmountOut": "1000000",
			"legs": [
				{"candidateId": "candidate-0", "amountIn": "500000000000000000", "amountOut": "500000"},
				{"candidateId": "candidate-1", "amountIn": "500000000000000000", "amountOut": "500000"}
			]
		}`))
	}))
	defer srv.Close()
	client, err := webhook.NewClient(webhook.Config{URL: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	quoteServer := testServer()
	quoteServer.quotes.tokensToQuote = tokensToQuotePermissioned
	quoteServer.quotes.permissionedTokens = map[common.Address]bool{tIn: true}
	quoteServer.quotes.strategy = webhookstrategy.New(client)
	request := validQuoteBody()
	request.Adapters = append(request.Adapters, quoteAdapter{
		Adapter: "0x0000000000000000000000000000000000000004", Asset: tOut.Hex(), AssetDecimals: 6,
		MaxAssets: "10000000", MaxRate: "1000000000000000000",
	})

	response, err := quoteServer.quotes.quote(t.Context(), &request)
	if err == nil || !strings.Contains(err.Error(), "single-route input requires exactly one leg") {
		t.Fatalf("quote error = %v, want single-route rejection", err)
	}
	if response != nil {
		t.Fatalf("quote response = %+v, want nil", response)
	}
}
