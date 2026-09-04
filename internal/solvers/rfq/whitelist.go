package rfq

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// adapterWhitelist is the set of adapters this filler will quote and fill through, built from the
// configured `adapters` list (the config analogue of the TS filler's deployment manifest). nil means
// filtering is disabled: every adapter the backend advertises is accepted. A non-nil empty set accepts
// no adapters at all — fail closed as defense in depth; parseConfig rejects the enabled-but-empty
// configuration outright, so it is unreachable from config.
type adapterWhitelist map[common.Address]struct{}

// buildAdapterWhitelist derives the whitelist from the configured adapters, or nil when disabled.
func buildAdapterWhitelist(enabled bool, adapters []recoveryVault) adapterWhitelist {
	if !enabled {
		return nil
	}
	wl := make(adapterWhitelist, len(adapters))
	for _, a := range adapters {
		wl[a.Adapter] = struct{}{}
	}
	return wl
}

// allows reports whether the adapter may be quoted and filled through.
func (w adapterWhitelist) allows(adapter common.Address) bool {
	if w == nil {
		return true
	}
	_, allowed := w[adapter]
	return allowed
}

// filter returns the inventories whose adapter is whitelisted.
func (w adapterWhitelist) filter(inv []liquidlane.Inventory) []liquidlane.Inventory {
	if w == nil {
		return inv
	}
	out := make([]liquidlane.Inventory, 0, len(inv))
	for _, v := range inv {
		if w.allows(v.Adapter) {
			out = append(out, v)
		}
	}
	return out
}
