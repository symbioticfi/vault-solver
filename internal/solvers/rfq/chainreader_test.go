package rfq

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestBindQuoteAdaptersCapacity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		secondVault common.Address
		wantTotal   int64
	}{
		{"independent", common.HexToAddress("0x22"), 200},
		{"shared", common.HexToAddress("0x11"), 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapterA, adapterB := common.HexToAddress("0xa1"), common.HexToAddress("0xb2")
			resolved := []solverInventory{
				testInventory(adapterA, tIn, tOut, big.NewInt(100), big.NewInt(1)),
				testInventory(adapterB, tIn, tOut, big.NewInt(100), big.NewInt(1)),
			}
			err := bindQuoteAdapters(1, resolved, resolvedQuoteAdapters([]recoveryVault{
				{Adapter: adapterA, Vault: common.HexToAddress("0x11"), TokenOut: tOut, TokenOutDecimals: 6},
				{Adapter: adapterB, Vault: tc.secondVault, TokenOut: tOut, TokenOutDecimals: 6},
			}))
			testcheck.NoError(t, err, "bindQuoteAdapters: %v")
			if shared := resolved[0].CapacityID == resolved[1].CapacityID; shared != (tc.wantTotal == 100) {
				t.Fatalf("capacity IDs = %q, %q", resolved[0].CapacityID, resolved[1].CapacityID)
			}
			total := new(big.Int)
			for _, item := range planning.AllocateInventoryCapacity(resolved, nil, 0) {
				total.Add(total, item.MaxAssets)
			}
			if total.Cmp(big.NewInt(tc.wantTotal)) != 0 {
				t.Fatalf("allocated capacity = %s, want %d", total, tc.wantTotal)
			}
		})
	}
}

func TestBindQuoteAdaptersFailsClosed(t *testing.T) {
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
			if err := bindQuoteAdapters(1, inventory, resolvedQuoteAdapters(resolved)); err == nil {
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
