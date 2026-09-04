package planning

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

type canonicalFillTestCase struct {
	name            string
	input           FillValidation
	routes          []liquidlane.PlanLeg
	wantErr         bool
	wantDiscount    *common.Hash
	wantReservation int64
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
	fill := func(route liquidlane.Route, amountIn, expected, minimum, reserved int64) liquidlane.PlanLeg {
		return liquidlane.PlanLeg{
			RouteID: route.ID, CapacityID: "forged-capacity", Adapter: common.HexToAddress("0xdead"),
			CandidateID: "forged-candidate", AmountIn: big.NewInt(amountIn),
			ExpectedAmountOut: big.NewInt(expected), MinAmountOut: big.NewInt(minimum),
			ReservedAmountOut: big.NewInt(reserved),
		}
	}

	tests := []canonicalFillTestCase{
		{
			name: "canonical direct route",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{directA}},
			routes: []liquidlane.PlanLeg{fill(routeA, 10, 20, 18, 20)}, wantReservation: 20,
		},
		{
			name: "canonical signed discount route",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{privateA}},
			routes: func() []liquidlane.PlanLeg {
				route := fill(routeA, 10, 20, 18, 20)
				route.DiscountID = new(discountID)
				return []liquidlane.PlanLeg{route}
			}(),
			wantDiscount: new(discountID), wantReservation: 20,
		},
		{
			name: "unknown candidate rejects",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{directA}},
			routes: func() []liquidlane.PlanLeg {
				route := fill(routeA, 10, 20, 18, 20)
				route.RouteID = "unknown"
				return []liquidlane.PlanLeg{route}
			}(),
			wantErr: true,
		},
		{
			name: "direct and private alternatives repeat physical route",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(10),
				RequiredAmountOut: big.NewInt(18), MaxRoutes: 3,
				Quotes: []liquidlane.FillQuote{directA, privateA}},
			routes: func() []liquidlane.PlanLeg {
				direct := fill(routeA, 5, 10, 9, 10)
				private := fill(routeA, 5, 10, 9, 10)
				private.DiscountID = new(discountID)
				return []liquidlane.PlanLeg{direct, private}
			}(),
			wantErr: true,
		},
		{
			name: "two routes exactly fill shared vault capacity",
			input: FillValidation{TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(20),
				RequiredAmountOut: big.NewInt(30), MaxRoutes: 3,
				Quotes: []liquidlane.FillQuote{directA, directB}},
			routes:          []liquidlane.PlanLeg{fill(routeA, 10, 15, 15, 15), fill(routeB, 10, 15, 15, 15)},
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
			routes:  []liquidlane.PlanLeg{fill(routeA, 10, 15, 15, 15), fill(routeB, 10, 15, 15, 15)},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCanonicalFillTestCase(t, test, routeA)
		})
	}
}

type canonicalFillOracleFixture struct {
	sourceInput        FillValidation
	sourceInputBefore  FillValidation
	sourceRoutes       []liquidlane.PlanLeg
	sourceRoutesBefore []liquidlane.PlanLeg
	sourceQuoteOracle  []fillQuoteOracle
	callInput          FillValidation
	callInputBefore    FillValidation
	callRoutes         []liquidlane.PlanLeg
	callRoutesBefore   []liquidlane.PlanLeg
	callQuoteOracle    []fillQuoteOracle
}

func runCanonicalFillTestCase(t *testing.T, test canonicalFillTestCase, routeA liquidlane.Route) {
	t.Helper()
	fixture := newCanonicalFillOracleFixture(t, test)
	normalized, err := ValidateFillRoutes(fixture.callInput, fixture.callRoutes)
	if (err != nil) != test.wantErr {
		t.Fatalf("ValidateFillRoutes() error = %v, wantErr %t", err, test.wantErr)
	}
	if test.wantErr {
		return
	}
	capacityID := liquidlane.RouteCapacityID(routeA)
	canonicalReservations := assertCanonicalFillOutput(t, test, fixture, normalized, capacityID)

	mutateCanonicalFillOutput(normalized, canonicalReservations)
	assertCanonicalFillInputsImmutable(t, fixture)
}

func newCanonicalFillOracleFixture(
	t *testing.T,
	test canonicalFillTestCase,
) canonicalFillOracleFixture {
	t.Helper()
	sourceInput := cloneFillValidationFixture(test.input)
	reservations := make(liquidlane.CapacityReservations)
	reservations.AddAll(sourceInput.Reservations)
	reservations.Add("oracle-ledger", big.NewInt(7))
	sourceInput.Reservations = reservations
	sourceRoutes := cloneFillRouteFixtures(test.routes)
	callInput := cloneFillValidationFixture(sourceInput)
	callRoutes := cloneFillRouteFixtures(sourceRoutes)
	return canonicalFillOracleFixture{
		sourceInput: sourceInput, sourceInputBefore: cloneFillValidationFixture(sourceInput),
		sourceRoutes: sourceRoutes, sourceRoutesBefore: cloneFillRouteFixtures(sourceRoutes),
		sourceQuoteOracle: immutableFillQuoteOracle(sourceInput.Quotes),
		callInput:         callInput, callInputBefore: cloneFillValidationFixture(callInput),
		callRoutes: callRoutes, callRoutesBefore: cloneFillRouteFixtures(callRoutes),
		callQuoteOracle: immutableFillQuoteOracle(callInput.Quotes),
	}
}

func assertCanonicalFillOutput(
	t *testing.T,
	test canonicalFillTestCase,
	fixture canonicalFillOracleFixture,
	normalized []liquidlane.PlanLeg,
	capacityID liquidlane.CapacityID,
) liquidlane.CapacityReservations {
	t.Helper()
	reservations := make(liquidlane.CapacityReservations)
	for index, route := range normalized {
		candidate := fixture.callInputBefore.Quotes[index]
		untrusted := fixture.callRoutesBefore[index]
		if route.RouteID != candidate.ID || route.CapacityID != capacityID ||
			route.Adapter != candidate.Adapter ||
			route.CandidateID != liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID) ||
			route.AmountIn.Cmp(untrusted.AmountIn) != 0 ||
			route.ExpectedAmountOut.Cmp(untrusted.ExpectedAmountOut) != 0 ||
			route.MinAmountOut.Cmp(untrusted.MinAmountOut) != 0 ||
			route.ReservedAmountOut.Cmp(untrusted.ReservedAmountOut) != 0 ||
			!reflect.DeepEqual(route.DiscountID, candidate.DiscountID) {
			t.Fatalf("canonical route %d = %+v", index, route)
		}
		reservations.Add(route.CapacityID, route.ReservedAmountOut)
	}
	if test.wantDiscount != nil &&
		(normalized[0].DiscountID == nil || *normalized[0].DiscountID != *test.wantDiscount) {
		t.Fatalf("canonical discount = %v, want %s", normalized[0].DiscountID, test.wantDiscount.Hex())
	}
	if got := reservations[capacityID]; got == nil || got.Cmp(big.NewInt(test.wantReservation)) != 0 {
		t.Fatalf("canonical reservations = %v, want %d", reservations, test.wantReservation)
	}
	return reservations
}

func mutateCanonicalFillOutput(normalized []liquidlane.PlanLeg, reservations liquidlane.CapacityReservations) {
	for index := range normalized {
		normalized[index].AmountIn.SetInt64(901 + int64(index))
		normalized[index].ExpectedAmountOut.SetInt64(911 + int64(index))
		normalized[index].MinAmountOut.SetInt64(921 + int64(index))
		normalized[index].ReservedAmountOut.SetInt64(931 + int64(index))
		if normalized[index].DiscountID != nil {
			*normalized[index].DiscountID = common.HexToHash("0x1")
		}
	}
	for _, amount := range reservations {
		amount.SetInt64(999)
	}
}

func assertCanonicalFillInputsImmutable(t *testing.T, fixture canonicalFillOracleFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.sourceRoutes, fixture.sourceRoutesBefore) {
		t.Fatalf("source routes mutated through canonical output: got %+v want %+v",
			fixture.sourceRoutes, fixture.sourceRoutesBefore)
	}
	if !reflect.DeepEqual(fixture.callRoutes, fixture.callRoutesBefore) {
		t.Fatalf("call routes mutated through canonical output: got %+v want %+v",
			fixture.callRoutes, fixture.callRoutesBefore)
	}
	if !reflect.DeepEqual(fixture.sourceInput, fixture.sourceInputBefore) ||
		!reflect.DeepEqual(immutableFillQuoteOracle(fixture.sourceInput.Quotes), fixture.sourceQuoteOracle) {
		t.Fatalf("source candidate quotes mutated: got %+v want %+v",
			fixture.sourceInput.Quotes, fixture.sourceInputBefore.Quotes)
	}
	if !reflect.DeepEqual(fixture.callInput, fixture.callInputBefore) ||
		!reflect.DeepEqual(immutableFillQuoteOracle(fixture.callInput.Quotes), fixture.callQuoteOracle) {
		t.Fatalf("call candidate quotes mutated: got %+v want %+v",
			fixture.callInput.Quotes, fixture.callInputBefore.Quotes)
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
	fill := liquidlane.PlanLeg{
		RouteID: route.ID, AmountIn: big.NewInt(1_000_000), ExpectedAmountOut: big.NewInt(1_000_000),
		MinAmountOut: big.NewInt(900_000), ReservedAmountOut: big.NewInt(1_000_000),
	}
	input := FillValidation{
		TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(1_000_000),
		RequiredAmountOut: big.NewInt(500_000), MaxRoutes: 3, Quotes: []liquidlane.FillQuote{quote},
	}
	if _, err := ValidateFillRoutes(cloneFillValidationFixture(input), cloneFillRouteFixtures([]liquidlane.PlanLeg{fill})); err != nil {
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
	if _, err := ValidateFillRoutes(input, cloneFillRouteFixtures([]liquidlane.PlanLeg{fill})); err == nil {
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
		input.Quotes[index].MaxRate = liquidlane.CloneBig(input.Quotes[index].MaxRate)
		input.Quotes[index].AdapterMinDiscount = liquidlane.CloneBig(input.Quotes[index].AdapterMinDiscount)
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

func cloneFillRouteFixtures(routes []liquidlane.PlanLeg) []liquidlane.PlanLeg {
	out := make([]liquidlane.PlanLeg, len(routes))
	for index, route := range routes {
		out[index] = cloneFillRoute(route)
	}
	return out
}

type fillQuoteOracle struct {
	CandidateID     liquidlane.CandidateID
	RouteID         liquidlane.RouteID
	CapacityID      liquidlane.CapacityID
	MaxAssets       string
	MaxRate         string
	AdapterMinimum  string
	AmountIn        string
	GrossAmountOut  string
	MaxAmountOut    string
	MinimumDiscount string
	DiscountID      string
}

func immutableFillQuoteOracle(quotes []liquidlane.FillQuote) []fillQuoteOracle {
	oracle := make([]fillQuoteOracle, len(quotes))
	for index, quote := range quotes {
		oracle[index] = fillQuoteOracle{
			CandidateID:     liquidlane.NewCandidateID(quote.Route, quote.DiscountID),
			RouteID:         quote.ID,
			CapacityID:      liquidlane.RouteCapacityID(quote.Route),
			MaxAssets:       immutableBigInt(quote.MaxAssets),
			MaxRate:         immutableBigInt(quote.MaxRate),
			AdapterMinimum:  immutableBigInt(quote.AdapterMinDiscount),
			AmountIn:        immutableBigInt(quote.AmountIn),
			GrossAmountOut:  immutableBigInt(quote.GrossAmountOut),
			MaxAmountOut:    immutableBigInt(quote.MaxAmountOut),
			MinimumDiscount: immutableBigInt(quote.MinDiscount),
			DiscountID:      immutableHash(quote.DiscountID),
		}
	}
	return oracle
}

func immutableBigInt(value *big.Int) string {
	if value == nil {
		return "<nil>"
	}
	return value.String()
}

func immutableHash(value *common.Hash) string {
	if value == nil {
		return ""
	}
	return value.Hex()
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
	valid := liquidlane.PlanLeg{
		RouteID: route.ID, AmountIn: big.NewInt(10), ExpectedAmountOut: big.NewInt(20),
		MinAmountOut: big.NewInt(18), ReservedAmountOut: big.NewInt(20),
	}
	tests := map[string]func(*FillValidation, *liquidlane.PlanLeg){
		"unknown candidate": func(_ *FillValidation, route *liquidlane.PlanLeg) { route.RouteID = "missing" },
		"wrong pair":        func(input *FillValidation, _ *liquidlane.PlanLeg) { input.TokenOut = common.HexToAddress("0xbeef") },
		"input sum":         func(_ *FillValidation, route *liquidlane.PlanLeg) { route.AmountIn = big.NewInt(9) },
		"output":            func(_ *FillValidation, route *liquidlane.PlanLeg) { route.ExpectedAmountOut = big.NewInt(21) },
		"minimum":           func(_ *FillValidation, route *liquidlane.PlanLeg) { route.MinAmountOut = big.NewInt(17) },
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
			if _, err := ValidateFillRoutes(input, []liquidlane.PlanLeg{candidate}); err == nil {
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
	fill := liquidlane.PlanLeg{
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

	if _, err := ValidateFillRoutes(input, []liquidlane.PlanLeg{fill}); err == nil {
		t.Fatal("expected gas-negative fill to be rejected")
	}
}

func TestFillRouteReservationsAggregatesAndClonesInput(t *testing.T) {
	t.Parallel()
	routes := []liquidlane.PlanLeg{
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
	raw, err := json.Marshal(liquidlane.PlanLeg{CandidateID: "internal", RouteID: "route"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "internal") || !strings.Contains(string(raw), `"routeId":"route"`) {
		t.Fatalf("wire route = %s", raw)
	}
}
