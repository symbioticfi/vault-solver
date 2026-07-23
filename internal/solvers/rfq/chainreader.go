package rfq

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// reader is the RFQ adapter over the shared LiquidLane read surface.
type reader struct {
	chain *chain.Client
	ll    *liquidlane.Reader
}

func newReader(c *chain.Client, log logr.Logger) *reader {
	return &reader{chain: c, ll: liquidlane.NewReader(c, log)}
}

// recoveryVault is one configured LiquidLane adapter plus the Vault and Asset derived from it. Config
// carries only Adapter; Vault (adapter.vault()) and Asset (vault.asset()) are resolved on-chain at
// startup (see resolveVaults) and are fixed for the adapter's lifetime. The entries double as the
// adapter whitelist source (see buildAdapterWhitelist) and the fill-plan recovery candidate universe.
type recoveryVault = liquidlane.Adapter

// readVaultInventories reads each adapter's fill-time views (paused, getMaxAssets(tokenIn),
// getMaxRate(tokenIn)) in one multicall, using the startup-resolved Vault/Asset (decimals cached). Used
// to rebuild a fill plan when the quote-time one isn't cached (e.g. after a restart). Paused / failing /
// zero-liquidity adapters are dropped; direct legs only. Mirrors readAdapterInventories in inventories.ts.
func (r *reader) readVaultInventories(
	ctx context.Context, tokenIn common.Address, vaults []recoveryVault,
) ([]solverInventory, error) {
	if len(vaults) == 0 {
		return nil, nil
	}
	return r.ll.ReadInventory(ctx, r.ll.RoutesForToken(ctx, vaults, tokenIn))
}

// resolveVaults returns a copy of the configured entries with each Vault (adapter.vault()) and Asset
// (vault.asset()) resolved from chain at startup — config carries only adapter addresses, both fixed
// for the adapter's lifetime. Returning a fresh slice (rather than mutating the input) keeps the
// resolved fill-plan recovery universe independent of the config slice. Two batched multicalls (adapters'
// vault(), then those vaults' asset()); an entry whose reads revert is left zero and skipped by
// fill-time reads (readVaultInventories needs a non-zero Asset). Errors only on a multicall transport failure.
func (r *reader) resolveVaults(ctx context.Context, vaults []recoveryVault) ([]recoveryVault, error) {
	adapters := make([]common.Address, len(vaults))
	for i := range vaults {
		adapters[i] = vaults[i].Adapter
	}
	return r.ll.ResolveAdapters(ctx, adapters)
}

// readPermissionedVaultInventories returns the subset of readVaultInventories the executor is
// authorized to fill through: adapter.marketMaker() == executor, adapter.owner() == executor, or the
// marketMaker has delegated via adapter.isFiller(marketMaker, executor). Used at fill time so we never
// build inputs for an unauthorized adapter. Mirrors readPermissionedAdapterInventories in
// inventories.ts (marketMaker / owner / isFiller).
func (r *reader) readPermissionedVaultInventories(
	ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
) ([]solverInventory, error) {
	base, err := r.readVaultInventories(ctx, tokenIn, vaults)
	if err != nil || len(base) == 0 {
		return base, err
	}
	return r.ll.FilterAuthorized(ctx, base, executor)
}
