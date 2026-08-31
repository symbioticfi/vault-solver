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

func TestValidateFillRoutesCanonicalTable(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1000000000000000000000000000000000000001")
	tokenOut := common.HexToAddress("0x2000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x3000000000000000000000000000000000000003")
	routeA := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x4000000000000000000000000000000000000004"),
		vault,
		tokenIn,
		tokenOut,
		6,
		6,
	)
	routeB := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x5000000000000000000000000000000000000005"),
		vault,
		tokenIn,
		tokenOut,
		6,
		6,
	)
	discountID := common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	directA := liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{Route: routeA, MaxAssets: big.NewInt(30)},
		AmountIn:  big.NewInt(10), MaxAmountOut: big.NewInt(20),
	}
	privateA := directA
	privateA.DiscountID = new(discountID)
	directB := liquidlane.FillQuote{
		Inventory: liquidlane.Inventory{Route: routeB, MaxAssets: big.NewInt(30)},
		AmountIn:  big.NewInt(10), MaxAmountOut: big.NewInt(20),
	}
	fill := func(route liquidlane.Route, amountIn, expected, minimum, reserved int64) FillRoute {
		return FillRoute{
			RouteID: route.ID, CapacityID: "forged-capacity", Adapter: common.HexToAddress("0xdead"),
			CandidateID: "forged-candidate", AmountIn: big.NewInt(amountIn),
			ExpectedAmountOut: big.NewInt(expected), MinAmountOut: big.NewInt(minimum),
			ReservedAmountOut: big.NewInt(reserved),
		}
	}

	tests := []struct {
		name            string
		input           FillValidation
		routes          []FillRoute
		wantErr         bool
		wantDiscount    *common.Hash
		wantReservation int64
	}{
		{
			name: "canonical direct route",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{directA}},
			routes: []FillRoute{fill(routeA, 10, 20, 18, 20)}, wantReservation: 20,
		},
		{
			name: "canonical signed discount route",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{privateA}},
			routes: func() []FillRoute {
				route := fill(routeA, 10, 20, 18, 20)
				route.DiscountID = new(discountID)
				return []FillRoute{route}
			}(),
			wantDiscount: new(discountID), wantReservation: 20,
		},
		{
			name: "forged output capacity id canonicalizes",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{directA}},
			routes: []FillRoute{fill(routeA, 10, 20, 18, 20)}, wantReservation: 20,
		},
		{
			name: "unknown candidate rejects",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{directA}},
			routes: func() []FillRoute {
				route := fill(routeA, 10, 20, 18, 20)
				route.RouteID = "unknown"
				return []FillRoute{route}
			}(),
			wantErr: true,
		},
		{
			name: "direct and private alternatives repeat physical route",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3,
				Quotes: []liquidlane.FillQuote{directA, privateA}},
			routes: func() []FillRoute {
				direct := fill(routeA, 5, 10, 9, 10)
				private := fill(routeA, 5, 10, 9, 10)
				private.DiscountID = new(discountID)
				return []FillRoute{direct, private}
			}(),
			wantErr: true,
		},
		{
			name: "two routes exactly fill shared vault capacity",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(20),
				RequiredAmountOut: big.NewInt(30), MaxRoutes: 3,
				Quotes: []liquidlane.FillQuote{directA, directB}},
			routes:          []FillRoute{fill(routeA, 10, 15, 15, 15), fill(routeB, 10, 15, 15, 15)},
			wantReservation: 30,
		},
		{
			name: "pending canonical shared vault capacity exceeds cap by one",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(20),
				RequiredAmountOut: big.NewInt(30), MaxRoutes: 3,
				Quotes: []liquidlane.FillQuote{directA, directB},
				Reservations: liquidlane.CapacityReservations{
					liquidlane.RouteCapacityID(routeA): big.NewInt(1),
					"forged-capacity":                  big.NewInt(1_000),
				}},
			routes:  []FillRoute{fill(routeA, 10, 15, 15, 15), fill(routeB, 10, 15, 15, 15)},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceInput := cloneFillValidationFixture(test.input)
			sourceRoutes := cloneFillRouteFixtures(test.routes)
			callInput := cloneFillValidationFixture(sourceInput)
			callRoutes := cloneFillRouteFixtures(sourceRoutes)
			normalized, err := ValidateFillRoutes(callInput, callRoutes)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateFillRoutes() error = %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			capacityID := liquidlane.RouteCapacityID(routeA)
			for index, route := range normalized {
				candidate := test.input.Quotes[index]
				if route.RouteID != candidate.ID || route.CapacityID != capacityID ||
					route.Adapter != candidate.Adapter ||
					route.CandidateID != liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID) {
					t.Fatalf("canonical route %d = %+v", index, route)
				}
			}
			if test.wantDiscount != nil &&
				(normalized[0].DiscountID == nil || *normalized[0].DiscountID != *test.wantDiscount) {
				t.Fatalf("canonical discount = %v, want %s", normalized[0].DiscountID, test.wantDiscount.Hex())
			}
			reservations, ok := FillRouteReservations(normalized)
			if !ok || reservations[capacityID].Cmp(big.NewInt(test.wantReservation)) != 0 {
				t.Fatalf("canonical reservations = %v, want %d", reservations, test.wantReservation)
			}

			normalized[0].AmountIn.SetInt64(999)
			normalized[0].ReservedAmountOut.SetInt64(999)
			if normalized[0].DiscountID != nil {
				*normalized[0].DiscountID = common.HexToHash("0x1")
			}
			reservations[capacityID].SetInt64(999)
			if sourceRoutes[0].AmountIn.Cmp(test.routes[0].AmountIn) != 0 ||
				callRoutes[0].AmountIn.Cmp(test.routes[0].AmountIn) != 0 ||
				sourceRoutes[0].ReservedAmountOut.Cmp(test.routes[0].ReservedAmountOut) != 0 ||
				callRoutes[0].ReservedAmountOut.Cmp(test.routes[0].ReservedAmountOut) != 0 ||
				callInput.Quotes[0].AmountIn.Cmp(test.input.Quotes[0].AmountIn) != 0 ||
				callInput.Quotes[0].MaxAssets.Cmp(test.input.Quotes[0].MaxAssets) != 0 {
				t.Fatal("canonical output or reservation aliases immutable validation input")
			}
			if sourceRoutes[0].CapacityID != "forged-capacity" || sourceRoutes[0].CandidateID != "forged-candidate" {
				t.Fatalf("source route metadata mutated: %+v", sourceRoutes[0])
			}
		})
	}
}

func TestValidateFillRoutesGasFloorUsesImmutableSnapshot(t *testing.T) {
	t.Parallel()
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	adapter := common.HexToAddress("0x3333333333333333333333333333333333333333")
	vault := common.HexToAddress("0x4444444444444444444444444444444444444444")
	route := liquidlane.NewRoute(1, adapter, vault, tokenIn, tokenOut, 18, 18)
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
	}
	if _, err := ValidateFillRoutes(cloneFillValidationFixture(input), cloneFillRouteFixtures([]FillRoute{fill})); err != nil {
		t.Fatalf("gas-disabled validation: %v", err)
	}
	input.MaxFeePerGas = big.NewInt(1)
	input.GasPrices = liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
		tokenOut: big.NewInt(1_000_000_000_000_000_000),
	})
	input.GasSnapshot = &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vault: {FreeAssets: big.NewInt(1_000_000), Withdrawable: big.NewInt(1_000_000)},
		},
	}
	input.GasEnvelope = testGasEnvelope()
	if _, err := ValidateFillRoutes(input, cloneFillRouteFixtures([]FillRoute{fill})); err == nil {
		t.Fatal("gas-priced validation accepted output below required plus gas")
	}
	if fill.MinAmountOut.Cmp(big.NewInt(900_000)) != 0 || quote.MaxAssets.Cmp(big.NewInt(1_000_000)) != 0 ||
		input.GasSnapshot.Vaults[vault].FreeAssets.Cmp(big.NewInt(1_000_000)) != 0 ||
		input.GasSnapshot.Vaults[vault].Withdrawable.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatal("gas validation mutated immutable route, quote, or gas snapshot")
	}
}

func cloneFillValidationFixture(input FillValidation) FillValidation {
	input.AmountIn = liquidlane.CloneBig(input.AmountIn)
	input.RequiredAmountOut = liquidlane.CloneBig(input.RequiredAmountOut)
	input.MaxFeePerGas = liquidlane.CloneBig(input.MaxFeePerGas)
	input.Quotes = append([]liquidlane.FillQuote(nil), input.Quotes...)
	for index := range input.Quotes {
		input.Quotes[index].MaxAssets = liquidlane.CloneBig(input.Quotes[index].MaxAssets)
		input.Quotes[index].AmountIn = liquidlane.CloneBig(input.Quotes[index].AmountIn)
		input.Quotes[index].GrossAmountOut = liquidlane.CloneBig(input.Quotes[index].GrossAmountOut)
		input.Quotes[index].MaxAmountOut = liquidlane.CloneBig(input.Quotes[index].MaxAmountOut)
		input.Quotes[index].MinDiscount = liquidlane.CloneBig(input.Quotes[index].MinDiscount)
		input.Quotes[index].DiscountID = liquidlane.CloneHash(input.Quotes[index].DiscountID)
	}
	reservations := make(liquidlane.CapacityReservations, len(input.Reservations))
	reservations.AddAll(input.Reservations)
	input.Reservations = reservations
	return input
}

func cloneFillRouteFixtures(routes []FillRoute) []FillRoute {
	out := make([]FillRoute, len(routes))
	for index, route := range routes {
		out[index] = cloneFillRoute(route)
	}
	return out
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
	routes := []FillRoute{
		{CapacityID: "shared", ReservedAmountOut: big.NewInt(4)},
		{CapacityID: "shared", ReservedAmountOut: big.NewInt(6)},
	}
	reservations, ok := FillRouteReservations(routes)
	if !ok || reservations["shared"].Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("reservations = %#v, ok = %t", reservations, ok)
	}
	reservations["shared"].SetInt64(77)
	if routes[0].ReservedAmountOut.Cmp(big.NewInt(4)) != 0 || routes[1].ReservedAmountOut.Cmp(big.NewInt(6)) != 0 {
		t.Fatalf("aggregate aliases route input: %+v", routes)
	}
	routes[0].ReservedAmountOut.SetInt64(99)
	if reservations["shared"].Cmp(big.NewInt(77)) != 0 {
		t.Fatalf("reservation retained caller amount: %s", reservations["shared"])
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
