package lifi

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func TestWebhookStrategyDelegatesQuotesAndFill(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	solver := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenIn := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	adapter := common.HexToAddress("0x4444444444444444444444444444444444444444")
	laneRoute := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter,
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 6, TokenOutDecimals: 6,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case decideQuotesRoute:
			_ = json.NewEncoder(w).Encode(QuoteOutput{Quotes: []Quote{{
				FromAsset: tokenIn, ToAsset: tokenOut, FromDecimals: 6, ToDecimals: 6,
				Ranges: []QuoteRange{{MinAmount: big.NewInt(1), MaxAmount: big.NewInt(100), Quote: "1"}},
				Expiry: now.Add(30 * time.Second).Unix(),
			}}})
		case decideFillRoute:
			_ = json.NewEncoder(w).Encode(liquidlane.Plan{Routes: []liquidlane.PlanLeg{{
				RouteID: "route-1", AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(100),
				MinAmountOut: big.NewInt(90), ReservedAmountOut: big.NewInt(100),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	strategy := &webhookPlanner{client: client}
	inventory := liquidlane.Inventory{Route: laneRoute, MaxAssets: big.NewInt(100), MaxRate: big.NewInt(1)}
	quotes, err := strategy.DecideQuotes(t.Context(), QuoteInput{
		Solver: solver, Inventory: []liquidlane.Inventory{inventory}, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(quotes.Quotes) != 1 || quotes.Quotes[0].ExclusiveFor != solver {
		t.Fatalf("quotes = %+v", quotes.Quotes)
	}
	decision, err := strategy.DecideFill(t.Context(), FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(90),
		Quotes: []liquidlane.FillQuote{{
			Inventory: inventory, AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if decision.Plan == nil || len(decision.Plan.Routes) != 1 || decision.Plan.Routes[0].RouteID != laneRoute.ID {
		t.Fatalf("plan = %+v", decision.Plan)
	}
}

func TestWebhookStrategyClassifiesFillHTTPFailures(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		body          string
		wantPermanent bool
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, body: "invalid fill input", wantPermanent: true},
		{name: "unprocessable entity", statusCode: http.StatusUnprocessableEntity, body: "unsupported order", wantPermanent: true},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, body: "bad credentials"},
		{name: "forbidden", statusCode: http.StatusForbidden, body: "forbidden"},
		{name: "not found", statusCode: http.StatusNotFound, body: "route unavailable"},
		{name: "request timeout", statusCode: http.StatusRequestTimeout, body: "timeout"},
		{name: "too many requests", statusCode: http.StatusTooManyRequests, body: "retry later"},
		{name: "server error", statusCode: http.StatusInternalServerError, body: "boom"},
		{name: "decode error", statusCode: http.StatusOK, body: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			_, err = (&webhookPlanner{client: client}).DecideFill(t.Context(), FillInput{})
			if err == nil {
				t.Fatal("DecideFill error = nil, want webhook failure")
			}
			if got := IsPermanentFillDecisionError(err); got != tt.wantPermanent {
				t.Fatalf("permanent = %v, want %v (error: %v)", got, tt.wantPermanent, err)
			}
		})
	}
}

func TestWebhookStrategyKeepsFillTransportFailureTransient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	server.Close()

	_, err = (&webhookPlanner{client: client}).DecideFill(t.Context(), FillInput{})
	if err == nil {
		t.Fatal("DecideFill error = nil, want transport failure")
	}
	if IsPermanentFillDecisionError(err) {
		t.Fatalf("transport failure was marked permanent: %v", err)
	}
}
