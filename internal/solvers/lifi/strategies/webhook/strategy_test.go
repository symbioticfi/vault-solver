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
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
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
	if plan == nil || len(plan.Routes) != 1 || plan.Routes[0].Adapter != adapter ||
		plan.Routes[0].CapacityID != route.CapacityID {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestWebhookStrategyRejectsSharedCapacityOverspend(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.FillPlan{Routes: []types.FillRoute{
			{RouteID: "route-1", AmountIn: big.NewInt(50), ExpectedAmountOut: big.NewInt(50),
				MinAmountOut: big.NewInt(45), ReservedAmountOut: big.NewInt(60)},
			{RouteID: "route-2", AmountIn: big.NewInt(50), ExpectedAmountOut: big.NewInt(50),
				MinAmountOut: big.NewInt(45), ReservedAmountOut: big.NewInt(60)},
		}})
	}))
	defer server.Close()
	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	quotes := make([]liquidlane.FillQuote, 0, 2)
	for _, routeID := range []liquidlane.RouteID{"route-1", "route-2"} {
		quotes = append(quotes, liquidlane.FillQuote{
			Inventory: liquidlane.Inventory{Route: liquidlane.Route{
				ID: routeID, CapacityID: "shared", TokenIn: tokenIn, TokenOut: tokenOut,
			}, MaxAssets: big.NewInt(100)},
			AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		})
	}
	_, err = New(client).DecideFill(t.Context(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(90), Quotes: quotes,
	})
	if err == nil {
		t.Fatal("expected shared capacity error")
	}
}

func TestWebhookStrategyRejectsPendingCapacityOverspend(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.FillPlan{Routes: []types.FillRoute{{
			RouteID: "route-1", AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(70),
			MinAmountOut: big.NewInt(70), ReservedAmountOut: big.NewInt(70),
		}}})
	}))
	defer server.Close()
	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = New(client).DecideFill(t.Context(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(70),
		Reservations: map[liquidlane.CapacityID]*big.Int{"shared": big.NewInt(40)},
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{Route: liquidlane.Route{
				ID: "route-1", CapacityID: "shared", TokenIn: tokenIn, TokenOut: tokenOut,
			}, MaxAssets: big.NewInt(100)},
			AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	})
	if err == nil {
		t.Fatal("expected pending reservation capacity error")
	}
}

func TestWebhookStrategyRejectsUnderReservedFill(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.FillPlan{Routes: []types.FillRoute{{
			RouteID: "route-1", AmountIn: big.NewInt(100), ExpectedAmountOut: big.NewInt(100),
			MinAmountOut: big.NewInt(90), ReservedAmountOut: big.NewInt(99),
		}}})
	}))
	defer server.Close()
	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = New(client).DecideFill(t.Context(), types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), OutputAmount: big.NewInt(90),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{Route: liquidlane.Route{
				ID: "route-1", CapacityID: "capacity-1", TokenIn: tokenIn, TokenOut: tokenOut,
			}, MaxAssets: big.NewInt(100)},
			AmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(100),
		}},
	})
	if err == nil {
		t.Fatal("expected under-reservation error")
	}
}

func TestWebhookStrategyRejectsFillThatDoesNotCoverGas(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	vault := common.HexToAddress("0x4444444444444444444444444444444444444444")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter, Vault: vault,
		TokenIn: tokenIn, TokenOut: tokenOut,
	}
	plan := &types.FillPlan{Routes: []types.FillRoute{{
		RouteID: "route-1", AmountIn: big.NewInt(1_000_000), ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut: big.NewInt(900_000), ReservedAmountOut: big.NewInt(1_000_000),
	}}}
	input := types.FillInput{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000_000), OutputAmount: big.NewInt(500_000),
		Quotes: []liquidlane.FillQuote{{
			Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(1_000_000)},
			AmountIn:  big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
		}},
		MaxFeePerGas: big.NewInt(1),
		GasPrices: liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
			tokenOut: big.NewInt(1_000_000_000_000_000_000),
		}),
		GasSnapshot: &liquidlanegas.Snapshot{
			Adapters: map[common.Address]*liquidlanegas.AdapterState{
				adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{}},
			},
			Vaults: map[common.Address]*liquidlanegas.VaultState{
				vault: {FreeAssets: big.NewInt(1_000_000), Withdrawable: big.NewInt(1_000_000)},
			},
		},
	}

	if err := validateFill(input, plan); err == nil {
		t.Fatal("expected gas-negative fill to be rejected")
	}
}

func TestWebhookStrategyRejectsUnknownFillCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.FillPlan{Routes: []types.FillRoute{{
			RouteID: "unknown", AmountIn: big.NewInt(1), ExpectedAmountOut: big.NewInt(1),
			MinAmountOut: big.NewInt(1), ReservedAmountOut: big.NewInt(1),
		}}})
	}))
	defer server.Close()
	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = New(client).DecideFill(t.Context(), types.FillInput{AmountIn: big.NewInt(1)})
	if err == nil {
		t.Fatal("expected unknown candidate error")
	}
}
