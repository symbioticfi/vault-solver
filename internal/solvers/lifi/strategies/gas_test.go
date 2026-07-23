package strategies

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

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
	prices := liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
		tokenOut: big.NewInt(nativeUnit),
	})
	leg := GasLeg{
		Route:     liquidlane.Route{Adapter: adapter, Vault: vault, TokenIn: tokenIn},
		AmountOut: big.NewInt(10),
	}

	direct, err := FillGasCost(big.NewInt(3), tokenOut, prices, snapshot, []GasLeg{leg})
	if err != nil {
		t.Fatalf("direct FillGasCost: %v", err)
	}
	wantDirect := new(big.Int).SetUint64(
		settlementGasUnits + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAllocate, true),
	)
	wantDirect.Mul(wantDirect, big.NewInt(3))
	if direct.Cmp(wantDirect) != 0 {
		t.Fatalf("direct gas cost = %s, want %s", direct, wantDirect)
	}

	leg.Private = true
	private, err := FillGasCost(big.NewInt(3), tokenOut, prices, snapshot, []GasLeg{leg})
	if err != nil {
		t.Fatalf("private FillGasCost: %v", err)
	}
	wantPrivate := new(big.Int).Add(
		wantDirect,
		new(big.Int).Mul(new(big.Int).SetUint64(privateRouteGasUnits), big.NewInt(3)),
	)
	if private.Cmp(wantPrivate) != 0 {
		t.Fatalf("private gas cost = %s, want %s", private, wantPrivate)
	}
}

func TestFillGasCostRejectsMissingTokenRate(t *testing.T) {
	tokenOut := common.HexToAddress("0x4444444444444444444444444444444444444444")
	_, err := FillGasCost(
		big.NewInt(1),
		tokenOut,
		nil,
		nil,
		[]GasLeg{{AmountOut: big.NewInt(1)}},
	)
	if err == nil {
		t.Fatal("expected missing token rate error")
	}
}
