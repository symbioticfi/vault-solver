package webhookstrategy

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func TestWebhookStrategyDelegatesOneQuoteAndCurrentFill(t *testing.T) {
	tokenIn := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	adapter := common.HexToAddress("0x4444444444444444444444444444444444444444")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter,
		TokenIn: tokenIn, TokenOut: tokenOut, TokenInDecimals: 6, TokenOutDecimals: 6,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case decideQuoteRoute:
			_ = json.NewEncoder(w).Encode(types.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)})
		case decideFillRoute:
			_ = json.NewEncoder(w).Encode(types.FillPlan{Routes: []types.FillRoute{{
				RouteID: route.ID, AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(100),
				MinAmountOut: big.NewInt(90), ReservedAmountOut: big.NewInt(100),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	strategy := newWebhookTestStrategy(t, server.URL)

	quote, err := strategy.DecideQuote(t.Context(), types.QuoteInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100),
	})
	if err != nil || quote == nil || quote.AmountOut.String() != "90" {
		t.Fatalf("quote = %+v, err %v", quote, err)
	}
	inventory := liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(100)}
	plan, err := strategy.DecideFill(t.Context(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(90),
		Quotes: []liquidlane.FillQuote{{Inventory: inventory, AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100)}},
	})
	if err != nil || plan == nil || len(plan.Routes) != 1 || plan.Routes[0].RouteID != route.ID {
		t.Fatalf("plan = %+v, err %v", plan, err)
	}
}

func newWebhookTestStrategy(t *testing.T, url string) *Strategy {
	t.Helper()
	client, err := webhook.NewClient(webhook.Config{URL: url, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return New(client)
}
