package defaultstrategy

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

func TestGasPricingUsesSharedRoutePrediction(t *testing.T) {
	adapter := common.HexToAddress("0x1111111111111111111111111111111111111111")
	vault := common.HexToAddress("0x1313131313131313131313131313131313131313")
	tokenIn := common.HexToAddress("0x1212121212121212121212121212121212121212")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	pricing := testGasPricing(t, tokenOut, big.NewInt(3), &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			adapter: {Vault: vault, Acquire: map[common.Address]*big.Int{}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vault: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(100)},
		},
	}, 0)
	leg := gasLeg{
		route: liquidlane.Route{Adapter: adapter, Vault: vault, TokenIn: tokenIn}, amountOut: big.NewInt(10),
	}
	direct := pricing.cost([]gasLeg{leg})
	wantUnits := uint64(250_000) + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAllocate, true)
	wantDirect := new(big.Int).Mul(new(big.Int).SetUint64(wantUnits), big.NewInt(3))
	if direct.Cmp(wantDirect) != 0 {
		t.Fatalf("direct cost = %s, want %s", direct, wantDirect)
	}
	leg.private = true
	private := pricing.cost([]gasLeg{leg})
	wantPrivate := new(big.Int).Add(wantDirect,
		new(big.Int).Mul(new(big.Int).SetUint64(75_000), big.NewInt(3)))
	if private.Cmp(wantPrivate) != 0 {
		t.Fatalf("private cost = %s, want %s", private, wantPrivate)
	}
}

func TestGasPricingChargesSettlementOnceForMultiAdapterPlan(t *testing.T) {
	adapterA := common.HexToAddress("0x1111111111111111111111111111111111111111")
	adapterB := common.HexToAddress("0x2222222222222222222222222222222222222222")
	vaultA := common.HexToAddress("0x1414141414141414141414141414141414141414")
	vaultB := common.HexToAddress("0x1515151515151515151515151515151515151515")
	tokenIn := common.HexToAddress("0x1212121212121212121212121212121212121212")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	snapshot := &liquidlanegas.Snapshot{
		Adapters: map[common.Address]*liquidlanegas.AdapterState{
			adapterA: {Vault: vaultA, Acquire: map[common.Address]*big.Int{}},
			adapterB: {Vault: vaultB, Acquire: map[common.Address]*big.Int{}},
		},
		Vaults: map[common.Address]*liquidlanegas.VaultState{
			vaultA: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(100)},
			vaultB: {FreeAssets: big.NewInt(100), Withdrawable: big.NewInt(100)},
		},
	}
	pricing := testGasPricing(t, tokenOut, big.NewInt(3), snapshot, 0)
	cost := pricing.cost([]gasLeg{
		{route: liquidlane.Route{Adapter: adapterA, Vault: vaultA, TokenIn: tokenIn}, amountOut: big.NewInt(10)},
		{route: liquidlane.Route{Adapter: adapterB, Vault: vaultB, TokenIn: tokenIn}, amountOut: big.NewInt(10)},
	})
	wantUnits := uint64(250_000) +
		liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAllocate, true) +
		liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAllocate, true)
	want := new(big.Int).Mul(new(big.Int).SetUint64(wantUnits), big.NewInt(3))
	if cost.Cmp(want) != 0 {
		t.Fatalf("multi-adapter cost = %s, want %s", cost, want)
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
	pricing := testGasPricing(t, tokenOut, big.NewInt(1), snapshot, 1_000)
	cost := pricing.cost([]gasLeg{{
		route: liquidlane.Route{Adapter: adapter, Vault: vault, TokenIn: tokenIn}, amountOut: big.NewInt(95),
	}})
	want := new(big.Int).SetUint64(250_000 + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteDeallocate, true))
	if cost.Cmp(want) != 0 {
		t.Fatalf("reserved route cost = %s, want %s", cost, want)
	}
}

func TestGasPricingRejectsMissingOracleRate(t *testing.T) {
	_, err := newGasPricing(big.NewInt(1), common.HexToAddress("0x2222222222222222222222222222222222222222"), nil, nil, 0)
	if err == nil {
		t.Fatal("expected missing token rate error")
	}
}

func testGasPricing(
	t *testing.T,
	tokenOut common.Address,
	fee *big.Int,
	snapshot *liquidlanegas.Snapshot,
	reserveBps int,
) gasPricing {
	t.Helper()
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
		tokenOut: big.NewInt(1_000_000_000_000_000_000),
	})
	pricing, err := newGasPricing(fee, tokenOut, prices, snapshot, reserveBps)
	if err != nil {
		t.Fatalf("newGasPricing: %v", err)
	}
	return pricing
}
