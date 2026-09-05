package strategies

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

func TestValidateFillRoutesCanonicalizesAndChecksCapacity(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1000000000000000000000000000000000000001")
	tokenOut := common.HexToAddress("0x2000000000000000000000000000000000000002")
	adapter := common.HexToAddress("0x3000000000000000000000000000000000000003")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter,
		TokenIn: tokenIn, TokenOut: tokenOut,
	}
	quote := liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(100)},
		AmountIn:  big.NewInt(10), MaxAmountOut: big.NewInt(20),
	}
	untrusted := []FillRoute{{
		RouteID: route.ID, Adapter: common.HexToAddress("0xdead"), AmountIn: big.NewInt(10),
		ExpectedAmountOut: big.NewInt(20), MinAmountOut: big.NewInt(18), ReservedAmountOut: big.NewInt(20),
	}}

	normalized, err := ValidateFillRoutes(FillValidation{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10), RequiredAmountOut: big.NewInt(18),
		MaxRoutes: 3, Quotes: []liquidlane.FillQuote{quote},
	}, untrusted)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(normalized) != 1 || normalized[0].Adapter != adapter || normalized[0].CapacityID != route.CapacityID {
		t.Fatalf("normalized route = %#v", normalized)
	}
	if untrusted[0].Adapter == adapter || untrusted[0].CapacityID != "" {
		t.Fatalf("input route was mutated: %#v", untrusted[0])
	}

	_, err = ValidateFillRoutes(FillValidation{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10), RequiredAmountOut: big.NewInt(18),
		MaxRoutes: 3, Quotes: []liquidlane.FillQuote{quote},
		Reservations: liquidlane.CapacityReservations{route.CapacityID: big.NewInt(90)},
	}, untrusted)
	if err == nil {
		t.Fatal("expected shared capacity error")
	}
}

func TestValidateFillRoutesRejectsUntrustedOutput(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1000000000000000000000000000000000000001")
	tokenOut := common.HexToAddress("0x2000000000000000000000000000000000000002")
	route := liquidlane.Route{ID: "route-1", CapacityID: "capacity-1", TokenIn: tokenIn, TokenOut: tokenOut}
	quote := liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(100)},
		AmountIn:  big.NewInt(10), MaxAmountOut: big.NewInt(20),
	}
	valid := FillRoute{
		RouteID: route.ID, AmountIn: big.NewInt(10), ExpectedAmountOut: big.NewInt(20),
		MinAmountOut: big.NewInt(18), ReservedAmountOut: big.NewInt(20),
	}
	tests := map[string]func(*FillValidation, *FillRoute){
		"unknown candidate": func(_ *FillValidation, route *FillRoute) { route.RouteID = "missing" },
		"wrong pair":        func(input *FillValidation, _ *FillRoute) { input.TokenOut = common.HexToAddress("0xbeef") },
		"input sum":         func(_ *FillValidation, route *FillRoute) { route.AmountIn = big.NewInt(9) },
		"output":            func(_ *FillValidation, route *FillRoute) { route.ExpectedAmountOut = big.NewInt(21) },
		"minimum":           func(_ *FillValidation, route *FillRoute) { route.MinAmountOut = big.NewInt(17) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := FillValidation{
				TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{quote},
			}
			candidate := cloneFillRoute(valid)
			mutate(&input, &candidate)
			if _, err := ValidateFillRoutes(input, []FillRoute{candidate}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateFillRoutesIncludesSettlementGas(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	vault := common.HexToAddress("0x4444444444444444444444444444444444444444")
	route := liquidlane.Route{
		ID: "route-1", CapacityID: "capacity-1", Adapter: adapter, Vault: vault,
		TokenIn: tokenIn, TokenOut: tokenOut,
	}
	quote := liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{Route: route, MaxAssets: big.NewInt(1_000_000)},
		AmountIn:  big.NewInt(1_000_000), MaxAmountOut: big.NewInt(1_000_000),
	}
	fill := FillRoute{
		RouteID: route.ID, AmountIn: big.NewInt(1_000_000), ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut: big.NewInt(900_000), ReservedAmountOut: big.NewInt(1_000_000),
	}
	input := FillValidation{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000_000),
		RequiredAmountOut: big.NewInt(500_000), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{quote},
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
		GasEnvelope: testGasEnvelope(),
	}

	if _, err := ValidateFillRoutes(input, []FillRoute{fill}); err == nil {
		t.Fatal("expected gas-negative fill to be rejected")
	}
}

func TestFillRouteReservationsAggregatesAndClonesInput(t *testing.T) {
	t.Parallel()
	amount := big.NewInt(4)
	reservations, ok := FillRouteReservations([]FillRoute{
		{CapacityID: "shared", ReservedAmountOut: amount},
		{CapacityID: "shared", ReservedAmountOut: big.NewInt(6)},
	})
	if !ok || reservations["shared"].Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("reservations = %#v, ok = %t", reservations, ok)
	}
	amount.SetInt64(99)
	if reservations["shared"].Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("reservation retained caller amount: %s", reservations["shared"])
	}
}

func TestPlannedSurplusUsesExpectedOutput(t *testing.T) {
	t.Parallel()
	routes := []FillRoute{{
		ExpectedAmountOut: big.NewInt(110),
		MinAmountOut:      big.NewInt(100),
	}}
	if got := PlannedSurplus(routes, big.NewInt(100)); got.Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("planned surplus = %s, want 10", got)
	}
}

func TestFillRouteWireOmitsInternalCandidateID(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(FillRoute{CandidateID: "internal", RouteID: "route"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "internal") || !strings.Contains(string(raw), `"routeId":"route"`) {
		t.Fatalf("wire route = %s", raw)
	}
}
