package rfq

import "github.com/ethereum/go-ethereum/common"

// adapterWhitelist is the set of adapters this filler will quote and fill through, built from the
// configured `vaults` list's LiquidLane adapters (the config analogue of the TS filler's deployment
// manifest). nil means filtering is disabled: every adapter the backend advertises is accepted. A
// non-nil empty set (whitelist enabled, no vaults configured) accepts no adapters at all — fail closed.
type adapterWhitelist map[common.Address]bool

// buildAdapterWhitelist derives the whitelist from the configured vaults, or nil when disabled.
func buildAdapterWhitelist(enabled bool, vaults []recoveryVault) adapterWhitelist {
	if !enabled {
		return nil
	}
	wl := make(adapterWhitelist, len(vaults))
	for _, v := range vaults {
		wl[v.Adapter] = true
	}
	return wl
}

// allows reports whether the adapter may be quoted and filled through.
func (w adapterWhitelist) allows(adapter common.Address) bool {
	return w == nil || w[adapter]
}

// filter returns the inventories whose adapter is whitelisted.
func (w adapterWhitelist) filter(inv []solverInventory) []solverInventory {
	if w == nil {
		return inv
	}
	out := make([]solverInventory, 0, len(inv))
	for _, v := range inv {
		if w[v.Adapter] {
			out = append(out, v)
		}
	}
	return out
}
