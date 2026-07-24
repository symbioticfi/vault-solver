package rfq

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

func TestApplyResolvedQuoteAdaptersPreservesIndependentCapacity(t *testing.T) {
	adapterA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	adapterB := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	vaultA := common.HexToAddress("0x0000000000000000000000000000000000000011")
	vaultB := common.HexToAddress("0x0000000000000000000000000000000000000022")
	inventory := []solverInventory{
		testInventory(adapterA, tIn, tOut, big.NewInt(100), big.NewInt(1)),
		testInventory(adapterB, tIn, tOut, big.NewInt(100), big.NewInt(1)),
	}

	resolved, err := applyResolvedQuoteAdapters(1, inventory, resolvedQuoteAdapters([]recoveryVault{
		{Adapter: adapterA, Vault: vaultA, TokenOut: tOut, TokenOutDecimals: 6},
		{Adapter: adapterB, Vault: vaultB, TokenOut: tOut, TokenOutDecimals: 6},
	}))
	if err != nil {
		t.Fatalf("applyResolvedQuoteAdapters: %v", err)
	}
	if resolved[0].CapacityID == resolved[1].CapacityID {
		t.Fatalf("independent vaults share capacity ID %q", resolved[0].CapacityID)
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(resolved, nil, 0)
	total := new(big.Int)
	for _, item := range allocated {
		total.Add(total, item.MaxAssets)
	}
	if total.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("allocated capacity = %s, want 200", total)
	}
}

func TestApplyResolvedQuoteAdaptersSharesVaultCapacity(t *testing.T) {
	adapterA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	adapterB := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	vault := common.HexToAddress("0x0000000000000000000000000000000000000011")
	inventory := []solverInventory{
		testInventory(adapterA, tIn, tOut, big.NewInt(100), big.NewInt(1)),
		testInventory(adapterB, tIn, tOut, big.NewInt(100), big.NewInt(1)),
	}

	resolved, err := applyResolvedQuoteAdapters(1, inventory, resolvedQuoteAdapters([]recoveryVault{
		{Adapter: adapterA, Vault: vault, TokenOut: tOut, TokenOutDecimals: 6},
		{Adapter: adapterB, Vault: vault, TokenOut: tOut, TokenOutDecimals: 6},
	}))
	if err != nil {
		t.Fatalf("applyResolvedQuoteAdapters: %v", err)
	}
	if resolved[0].CapacityID != resolved[1].CapacityID {
		t.Fatalf("shared vault capacity IDs = %q, %q", resolved[0].CapacityID, resolved[1].CapacityID)
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(resolved, nil, 0)
	total := new(big.Int)
	for _, item := range allocated {
		total.Add(total, item.MaxAssets)
	}
	if total.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("allocated capacity = %s, want shared limit 100", total)
	}
}

func TestApplyResolvedQuoteAdaptersFailsClosed(t *testing.T) {
	adapter := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	otherAsset := common.HexToAddress("0x0000000000000000000000000000000000000099")
	vault := common.HexToAddress("0x0000000000000000000000000000000000000011")
	inventory := []solverInventory{
		testInventory(adapter, tIn, tOut, big.NewInt(100), big.NewInt(1)),
	}

	for name, resolved := range map[string][]liquidlane.Adapter{
		"missing adapter": nil,
		"asset mismatch": {{
			Adapter:  adapter,
			Vault:    vault,
			TokenOut: otherAsset,
		}},
		"decimals mismatch": {{
			Adapter: adapter, Vault: vault, TokenOut: tOut, TokenOutDecimals: 18,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := applyResolvedQuoteAdapters(1, inventory, resolvedQuoteAdapters(resolved)); err == nil {
				t.Fatal("expected unresolved quote adapter error")
			}
		})
	}
}

func TestUnresolvedQuoteAdaptersSkipsStartupMetadata(t *testing.T) {
	known := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	unknown := common.HexToAddress("0x00000000000000000000000000000000000000b2")
	inventory := []solverInventory{
		testInventory(known, tIn, tOut, big.NewInt(100), big.NewInt(1)),
		testInventory(unknown, tIn, tOut, big.NewInt(100), big.NewInt(1)),
		testInventory(unknown, tIn, tOut, big.NewInt(100), big.NewInt(1)),
	}
	resolved := map[common.Address]recoveryVault{
		known: {
			Adapter:  known,
			Vault:    common.HexToAddress("0x0000000000000000000000000000000000000011"),
			TokenOut: tOut,
		},
	}

	got := unresolvedQuoteAdapters(inventory, resolved)
	if len(got) != 1 || got[0] != unknown {
		t.Fatalf("unresolved adapters = %v, want only %s", got, unknown.Hex())
	}
}
