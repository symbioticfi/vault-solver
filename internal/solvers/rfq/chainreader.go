package rfq

import (
	"context"
	"maps"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

// reader is the RFQ adapter over the shared LiquidLane read surface.
type reader struct {
	ll            *liquidlane.Reader
	chainID       int64
	quoteAdapters map[common.Address]recoveryVault // assigned once before the quote server starts
}

func newReader(c *chain.Client, log logr.Logger, liquidityLens common.Address) *reader {
	return &reader{ll: liquidlane.NewReader(c, log, liquidityLens), chainID: c.ChainID().Int64()}
}

// recoveryVault is one configured LiquidLane adapter plus the Vault and Asset derived from it. Config
// carries only Adapter; Vault (adapter.vault()) and Asset (vault.asset()) are resolved on-chain at
// startup (see resolveVaults) and are fixed for the adapter's lifetime. The entries double as the
// adapter whitelist source (see buildAdapterWhitelist) and the fill-plan recovery candidate universe.
type recoveryVault = liquidlane.Adapter

// readVaultInventories reads each adapter's fill-time views (paused, getMaxAssets(tokenIn),
// getMaxRate(tokenIn)) in one multicall, using the startup-resolved Vault/Asset (decimals cached). Used
// to build each fill plan from current state. Paused / failing / zero-liquidity adapters are dropped;
// direct legs only. Mirrors readAdapterInventories in inventories.ts.
func (r *reader) readVaultInventories(
	ctx context.Context, tokenIn common.Address, vaults []recoveryVault,
) ([]solverInventory, error) {
	if len(vaults) == 0 {
		return nil, nil
	}
	return r.ll.ReadInventory(ctx, r.ll.RoutesForToken(ctx, vaults, tokenIn))
}

// readQuoteCandidates turns amount-independent inventory into current,
// amount-normalized LiquidLane candidates. This protocol/on-chain adaptation
// belongs to the solver; strategies receive only the completed decision input.
func (r *reader) readQuoteCandidates(
	ctx context.Context,
	inventory []solverInventory,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	matching := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if item.TokenIn == tokenIn && item.TokenOut == tokenOut {
			matching = append(matching, item)
		}
	}
	if len(matching) == 0 {
		return nil, nil
	}
	metadata := r.quoteAdapters
	unknown := unresolvedQuoteAdapters(matching, metadata)
	if len(unknown) > 0 {
		resolved, err := r.ll.ResolveAdapters(ctx, unknown)
		if err != nil {
			return nil, errors.Errorf("resolve quote adapters: %w", err)
		}
		metadata = make(map[common.Address]recoveryVault, len(r.quoteAdapters)+len(resolved))
		maps.Copy(metadata, r.quoteAdapters)
		for _, adapter := range resolved {
			metadata[adapter.Adapter] = adapter
		}
	}
	matching, err := applyResolvedQuoteAdapters(r.chainID, matching, metadata)
	if err != nil {
		return nil, err
	}
	inputDecimals, err := r.ll.TokenDecimals(ctx, tokenIn)
	if err != nil {
		return nil, errors.Errorf("tokenIn decimals: %w", err)
	}
	for index := range matching {
		matching[index].TokenInDecimals = inputDecimals
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(matching, nil, 0)
	if len(allocated) == 0 {
		return nil, nil
	}
	routes := make([]liquidlane.Route, 0, len(allocated))
	seen := make(map[liquidlane.RouteID]bool, len(allocated))
	for _, item := range allocated {
		if !seen[item.ID] {
			routes = append(routes, item.Route)
			seen[item.ID] = true
		}
	}
	quotes, err := r.ll.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
	if err != nil {
		return nil, err
	}
	return liquidgreedy.NormalizeOracleInventory(amountIn, allocated, quotes), nil
}

func (r *reader) setQuoteAdapters(resolved []recoveryVault) {
	r.quoteAdapters = resolvedQuoteAdapters(resolved)
}

func unresolvedQuoteAdapters(
	inventory []solverInventory,
	resolved map[common.Address]recoveryVault,
) []common.Address {
	seen := make(map[common.Address]bool, len(inventory))
	out := make([]common.Address, 0, len(inventory))
	for _, item := range inventory {
		if _, ok := resolved[item.Adapter]; ok || seen[item.Adapter] {
			continue
		}
		seen[item.Adapter] = true
		out = append(out, item.Adapter)
	}
	return out
}

func resolvedQuoteAdapters(resolved []recoveryVault) map[common.Address]recoveryVault {
	out := make(map[common.Address]recoveryVault, len(resolved))
	for _, adapter := range resolved {
		if adapter.Adapter != (common.Address{}) && adapter.Vault != (common.Address{}) &&
			adapter.TokenOut != (common.Address{}) {
			out[adapter.Adapter] = adapter
		}
	}
	return out
}

func applyResolvedQuoteAdapters(
	chainID int64,
	inventory []solverInventory,
	byAdapter map[common.Address]recoveryVault,
) ([]solverInventory, error) {
	out := make([]solverInventory, len(inventory))
	for index, item := range inventory {
		adapter, ok := byAdapter[item.Adapter]
		if !ok {
			return nil, errors.Errorf("resolve quote adapter %s: metadata unavailable", item.Adapter.Hex())
		}
		if adapter.TokenOut != item.TokenOut {
			return nil, errors.Errorf(
				"resolve quote adapter %s: backend asset %s does not match on-chain asset %s",
				item.Adapter.Hex(), item.TokenOut.Hex(), adapter.TokenOut.Hex(),
			)
		}
		if adapter.TokenOutDecimals != item.TokenOutDecimals {
			return nil, errors.Errorf(
				"resolve quote adapter %s: backend asset decimals %d do not match on-chain decimals %d",
				item.Adapter.Hex(), item.TokenOutDecimals, adapter.TokenOutDecimals,
			)
		}
		item.Vault = adapter.Vault
		item.CapacityID = liquidlane.NewCapacityID(chainID, adapter.Vault, item.TokenOut)
		out[index] = item
	}
	return out, nil
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

func (r *reader) validateDirectAuthorization(
	ctx context.Context,
	executor common.Address,
	vaults []recoveryVault,
) error {
	routes := make([]liquidlane.Route, len(vaults))
	for i := range vaults {
		routes[i].Adapter = vaults[i].Adapter
	}
	authorized, err := r.ll.FilterAuthorizedRoutes(ctx, routes, executor)
	if err != nil {
		return err
	}
	if missing := liquidlane.UnauthorizedAdapters(routes, authorized); len(missing) > 0 {
		return errors.Errorf(
			"executor %s is not authorized as direct filler for configured adapters: %v",
			executor.Hex(), missing,
		)
	}
	return nil
}

// readPermissionedVaultInventories returns the subset of readVaultInventories the executor is
// authorized to fill through: adapter.marketMaker() == executor, adapter.owner() == executor, or the
// current marketMaker value (including zero) has delegated via adapter.isFiller(marketMaker, executor).
// Used at fill time so we never build inputs for an unauthorized adapter. Mirrors
// readPermissionedAdapterInventories in inventories.ts (marketMaker / owner / isFiller).
func (r *reader) readPermissionedVaultInventories(
	ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
) ([]solverInventory, error) {
	base, err := r.readVaultInventories(ctx, tokenIn, vaults)
	if err != nil || len(base) == 0 {
		return base, err
	}
	return r.ll.FilterAuthorized(ctx, base, executor)
}
