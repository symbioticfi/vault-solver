package planning

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

func testGasEnvelope() GasEnvelope {
	return GasEnvelope{SettlementUnits: 250_000, PrivateRouteUnits: 75_000}
}

func TestFillGasCostIncludesSettlementRouteAndPrivateOverhead(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	vault := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenIn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	tokenOut := common.HexToAddress("0x4444444444444444444444444444444444444444")
	snapshot := &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vault: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(100)},
		},
	}
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{tokenOut: big.NewInt(nativeUnit)})
	leg := GasLeg{
		Route: liquidlane.Route{Adapter: adapter, Vault: vault, TokenIn: tokenIn}, AmountOut: big.NewInt(10),
	}

	direct, err := FillGasCost(big.NewInt(3), tokenOut, prices, snapshot, testGasEnvelope(), []GasLeg{leg})
	if err != nil {
		t.Fatalf("direct FillGasCost: %v", err)
	}
	wantDirect := new(big.Int).SetUint64(
		testGasEnvelope().SettlementUnits + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAllocate, true),
	)
	wantDirect.Mul(wantDirect, big.NewInt(3))
	if direct.Cmp(wantDirect) != 0 {
		t.Fatalf("direct gas cost = %s, want %s", direct, wantDirect)
	}

	leg.Private = true
	private, err := FillGasCost(big.NewInt(3), tokenOut, prices, snapshot, testGasEnvelope(), []GasLeg{leg})
	if err != nil {
		t.Fatalf("private FillGasCost: %v", err)
	}
	wantPrivate := new(big.Int).Add(
		wantDirect,
		new(big.Int).Mul(new(big.Int).SetUint64(testGasEnvelope().PrivateRouteUnits), big.NewInt(3)),
	)
	if private.Cmp(wantPrivate) != 0 {
		t.Fatalf("private gas cost = %s, want %s", private, wantPrivate)
	}
}

func TestGasPricingAppliesInventoryReserveBeforeRoutePrediction(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	vault := common.HexToAddress("0x1414141414141414141414141414141414141414")
	tokenIn := common.HexToAddress("0x1212121212121212121212121212121212121212")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	snapshot := &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vault: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(200)},
		},
	}
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{tokenOut: big.NewInt(nativeUnit)})
	pricing, err := NewGasPricing(big.NewInt(1), tokenOut, prices, snapshot, 1_000, testGasEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	cost := pricing.Cost([]GasLeg{{
		Route: liquidlane.Route{Adapter: adapter, Vault: vault, TokenIn: tokenIn}, AmountOut: big.NewInt(95),
	}})
	want := new(big.Int).SetUint64(
		testGasEnvelope().SettlementUnits + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteDeallocate, true),
	)
	if cost.Cmp(want) != 0 {
		t.Fatalf("reserved route cost = %s, want %s", cost, want)
	}
}

func TestGasPricingUsesExecutorDirectThenPrivateOrder(t *testing.T) {
	tokenIn := common.HexToAddress("0x00000000000000000000000000000000000000ca")
	tokenOut := common.HexToAddress("0x00000000000000000000000000000000000000cb")
	directAdapter := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	privateAdapter := common.HexToAddress("0x00000000000000000000000000000000000000a2")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000f1")
	snapshot := &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			directAdapter:  {Vault: vault, Acquire: map[common.Address]*big.Int{}},
			privateAdapter: {Vault: vault, Acquire: map[common.Address]*big.Int{}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vault: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(150)},
		},
	}
	legs := []GasLeg{
		{
			Route:     liquidlane.Route{Adapter: privateAdapter, Vault: vault, TokenIn: tokenIn},
			AmountOut: big.NewInt(50), Private: true,
		},
		{
			Route:     liquidlane.Route{Adapter: directAdapter, Vault: vault, TokenIn: tokenIn},
			AmountOut: big.NewInt(120),
		},
	}
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{tokenOut: big.NewInt(nativeUnit)})

	pricing, err := NewGasPricing(big.NewInt(1), tokenOut, prices, snapshot, 0, GasEnvelope{})
	if err != nil {
		t.Fatalf("NewGasPricing: %v", err)
	}
	cost := pricing.Cost(legs)
	want := new(big.Int).SetUint64(
		liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteDeallocate, true) +
			liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, true),
	)
	if cost.Cmp(want) != 0 {
		t.Fatalf("cost = %s, want direct-then-private cost %s", cost, want)
	}
}

func TestGasPricingMaxCostBoundsEveryRouteAsFirstUnknown(t *testing.T) {
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{tokenOut: big.NewInt(nativeUnit)})
	pricing, err := NewGasPricing(big.NewInt(2), tokenOut, prices, nil, 0, testGasEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	cost := pricing.MaxCost(3, 2)
	units := testGasEnvelope().SettlementUnits +
		3*liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, true) +
		2*testGasEnvelope().PrivateRouteUnits
	want := new(big.Int).Mul(new(big.Int).SetUint64(units), big.NewInt(2))
	if cost.Cmp(want) != 0 {
		t.Fatalf("max gas cost = %s, want %s", cost, want)
	}
}

func TestFillGasCostRejectsMissingTokenRate(t *testing.T) {
	tokenOut := common.HexToAddress("0x4444444444444444444444444444444444444444")
	_, err := FillGasCost(
		big.NewInt(1), tokenOut, nil, nil, testGasEnvelope(), []GasLeg{{AmountOut: big.NewInt(1)}},
	)
	if err == nil {
		t.Fatal("expected missing token rate error")
	}
}
