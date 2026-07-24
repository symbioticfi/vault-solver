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
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func TestWebhookStrategyDelegatesQuotesAndFill(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	solver := common.HexToAddress("0x1111111111111111111111111111111111111111")
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
		case decideQuotesRoute:
			_ = json.NewEncoder(w).Encode(types.QuoteOutput{Quotes: []types.Quote{{
				FromAsset: tokenIn, ToAsset: tokenOut, FromDecimals: 6, ToDecimals: 6,
				Ranges: []types.QuoteRange{{MinAmount: big.NewInt(1), MaxAmount: big.NewInt(100), Quote: "1"}},
				Expiry: now.Add(30 * time.Second).Unix(),
			}}})
		case decideFillRoute:
			_ = json.NewEncoder(w).Encode(types.FillPlan{Routes: []types.FillRoute{{
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
	strategy := New(client)
	inventory := liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(100), MaxRate: big.NewInt(1)}
	quotes, err := strategy.DecideQuotes(t.Context(), types.QuoteInput{
		Solver: solver, Inventory: []liquidlane.Inventory{inventory}, ServerTime: now, QuoteExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("DecideQuotes: %v", err)
	}
	if len(quotes.Quotes) != 1 || quotes.Quotes[0].ExclusiveFor != solver {
		t.Fatalf("quotes = %+v", quotes.Quotes)
	}
	plan, err := strategy.DecideFill(t.Context(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(90),
		Quotes: []liquidlane.FillQuote{{
			Inventory: inventory, AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	})
	if err != nil {
		t.Fatalf("DecideFill: %v", err)
	}
	if plan == nil || len(plan.Routes) != 1 || plan.Routes[0].RouteID != route.ID {
		t.Fatalf("plan = %+v", plan)
	}
}
