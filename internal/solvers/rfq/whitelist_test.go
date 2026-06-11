package rfq

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestBuildAdapterWhitelist(t *testing.T) {
	listed := common.HexToAddress("0x0000000000000000000000000000000000000042")
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	vaults := []recoveryVault{{Adapter: listed}}

	cases := map[string]struct {
		enabled bool
		vaults  []recoveryVault
		adapter common.Address
		want    bool
	}{
		"disabled allows anything":        {enabled: false, vaults: vaults, adapter: rogue, want: true},
		"disabled with no vaults allows":  {enabled: false, vaults: nil, adapter: rogue, want: true},
		"enabled allows listed adapter":   {enabled: true, vaults: vaults, adapter: listed, want: true},
		"enabled denies unlisted adapter": {enabled: true, vaults: vaults, adapter: rogue, want: false},
		"enabled with no vaults denies":   {enabled: true, vaults: nil, adapter: listed, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wl := buildAdapterWhitelist(tc.enabled, tc.vaults)
			if got := wl.allows(tc.adapter); got != tc.want {
				t.Fatalf("allows(%s) = %v, want %v", tc.adapter.Hex(), got, tc.want)
			}
		})
	}
}

func TestAdapterWhitelist_Filter(t *testing.T) {
	listed := common.HexToAddress("0x0000000000000000000000000000000000000042")
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	inv := []solverInventory{
		{Adapter: listed, Asset: tOut, MaxAssets: big.NewInt(1), MaxRate: big.NewInt(1)},
		{Adapter: rogue, Asset: tOut, MaxAssets: big.NewInt(1), MaxRate: big.NewInt(1)},
	}

	wl := buildAdapterWhitelist(true, []recoveryVault{{Adapter: listed}})
	got := wl.filter(inv)
	if len(got) != 1 || got[0].Adapter != listed {
		t.Fatalf("filter kept %d entries, want only the listed adapter", len(got))
	}

	if all := adapterWhitelist(nil).filter(inv); len(all) != len(inv) {
		t.Fatalf("nil whitelist filtered to %d entries, want all %d", len(all), len(inv))
	}

	if none := buildAdapterWhitelist(true, nil).filter(inv); len(none) != 0 {
		t.Fatalf("empty whitelist kept %d entries, want 0 (fail closed)", len(none))
	}
}
