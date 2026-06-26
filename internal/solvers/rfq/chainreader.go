package rfq

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/adapter"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Contract bindings (abigen --v2): typed Pack/Unpack helpers for the Multicall3 sub-calls below, so an
// ABI change fails at compile time (see CLAUDE.md "Code generation").
var (
	llAdapter = adapter.NewLiquidLaneAdapter()
	// erc4626b serves both the vault's asset() and the asset token's decimals(): IERC4626 is an
	// ERC-20, and a method's selector/return shape is fixed by the ABI regardless of target.
	erc4626b = erc4626.NewIERC4626()
)

// readsPerAdapter is the number of Multicall3 sub-calls readVaultInventories issues per adapter
// (paused, getMaxAssets, getMaxRate). vault() and the vault's asset() are resolved once at startup
// (see resolveVaults), not re-read here.
const readsPerAdapter = 3

// reader performs the on-chain reads, batching via Multicall3. Token decimals are cached; the HTTP
// server serves quotes concurrently, so the cache is mutex-guarded.
type reader struct {
	chain *chain.Client
	log   logr.Logger

	mu       sync.Mutex
	decimals map[common.Address]int
}

func newReader(c *chain.Client, log logr.Logger) *reader {
	return &reader{chain: c, log: log, decimals: make(map[common.Address]int)}
}

// recoveryVault is one configured LiquidLane adapter plus the Vault and Asset derived from it. Config
// carries only Adapter; Vault (adapter.vault()) and Asset (vault.asset()) are resolved on-chain at
// startup (see resolveVaults) and are fixed for the adapter's lifetime. The entries double as the
// adapter whitelist source (see buildAdapterWhitelist) and the recovery candidate universe.
type recoveryVault struct {
	Adapter common.Address
	Vault   common.Address
	Asset   common.Address
}

// tokenDecimals returns the ERC-20 decimals for token, caching the result.
func (r *reader) tokenDecimals(ctx context.Context, token common.Address) (int, error) {
	r.mu.Lock()
	if d, ok := r.decimals[token]; ok {
		r.mu.Unlock()
		return d, nil
	}
	r.mu.Unlock()

	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: token, Data: erc4626b.PackDecimals()}})
	if err != nil {
		return 0, err
	}
	if len(res) != 1 || !res[0].Success {
		return 0, errors.Errorf("erc20.decimals() reverted for %s", token)
	}
	d, err := erc4626b.UnpackDecimals(res[0].ReturnData)
	if err != nil {
		return 0, errors.Errorf("unpack decimals: %w", err)
	}
	r.mu.Lock()
	r.decimals[token] = int(d)
	r.mu.Unlock()
	return int(d), nil
}

// amountsOut prices each distinct asset by calling its representative adapter's getAmountOut(tokenIn,
// amount). The quote oracle is per asset-group: the representative is the first inventory entry seen
// for that asset, matching evaluateInventoryGroup in strategy.ts (inventories[0].adapter). Targets are
// heterogeneous (each call hits that asset's adapter). A reverting sub-call leaves the asset unpriced
// (the selector then skips it), so the map only holds successfully-priced assets.
func (r *reader) amountsOut(
	ctx context.Context, tokenIn common.Address, inventories []solverInventory, amount *big.Int,
) (map[common.Address]*big.Int, error) {
	// Pick the representative adapter per distinct asset (first seen), preserving deterministic order.
	type group struct {
		asset   common.Address
		adapter common.Address
	}
	var groups []group
	seen := make(map[common.Address]bool, len(inventories))
	for _, inv := range inventories {
		if seen[inv.Asset] {
			continue
		}
		seen[inv.Asset] = true
		groups = append(groups, group{asset: inv.Asset, adapter: inv.Adapter})
	}
	if len(groups) == 0 {
		return map[common.Address]*big.Int{}, nil
	}

	calls := make([]chain.Call, len(groups))
	for i, g := range groups {
		calls[i] = chain.Call{
			Target:       g.adapter,
			AllowFailure: true,
			Data:         llAdapter.PackGetAmountOut(tokenIn, amount),
		}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("amountsOut: got %d results for %d calls", len(res), len(calls))
	}
	out := make(map[common.Address]*big.Int, len(groups))
	for i, rr := range res {
		if !rr.Success {
			r.log.V(1).Info("getAmountOut reverted; asset left unpriced", "asset", groups[i].asset.Hex())
			continue
		}
		amt, derr := llAdapter.UnpackGetAmountOut(rr.ReturnData)
		if derr != nil {
			r.log.V(1).Error(derr, "getAmountOut decode failed; asset left unpriced", "asset", groups[i].asset.Hex())
			continue
		}
		out[groups[i].asset] = amt
	}
	return out, nil
}

// readVaultInventories reads each adapter's recovery views (paused, getMaxAssets(tokenIn),
// getMaxRate(tokenIn)) in one multicall, using the startup-resolved Vault/Asset (decimals cached). Used
// to rebuild a strategy when the quote-time one isn't cached (e.g. after a restart). Paused / failing /
// zero-liquidity adapters are dropped; direct legs only. Mirrors readAdapterInventories in inventories.ts.
func (r *reader) readVaultInventories(
	ctx context.Context, tokenIn common.Address, vaults []recoveryVault,
) ([]solverInventory, error) {
	vaults = dedupeVaultsByAdapter(vaults)
	if len(vaults) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, 0, len(vaults)*readsPerAdapter)
	for _, v := range vaults {
		calls = append(calls,
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: llAdapter.PackGetMaxAssets(tokenIn)},
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: llAdapter.PackGetMaxRate(tokenIn)},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("inventory multicall: got %d results, want %d", len(res), len(calls))
	}

	out := make([]solverInventory, 0, len(vaults))
	for i, v := range vaults {
		base := i * readsPerAdapter
		paused, maxA, mr := res[base], res[base+1], res[base+2]
		if !maxA.Success || !mr.Success {
			continue
		}
		if p, perr := llAdapter.UnpackPaused(paused.ReturnData); paused.Success && perr == nil && p {
			continue
		}
		maxAssets, e1 := llAdapter.UnpackGetMaxAssets(maxA.ReturnData)
		maxRate, e2 := llAdapter.UnpackGetMaxRate(mr.ReturnData)
		if e1 != nil || e2 != nil {
			continue
		}
		if maxAssets.Sign() <= 0 || maxRate.Sign() <= 0 {
			continue
		}
		decimals, derr := r.tokenDecimals(ctx, v.Asset)
		if derr != nil {
			continue
		}
		out = append(out, solverInventory{
			Adapter: v.Adapter, Asset: v.Asset, AssetDecimals: decimals,
			MaxAssets: maxAssets, MaxRate: maxRate, DiscountID: nil,
		})
	}
	return out, nil
}

// resolveVaults returns a copy of the configured entries with each Vault (adapter.vault()) and Asset
// (vault.asset()) resolved from chain at startup — config carries only adapter addresses, both fixed
// for the adapter's lifetime. Returning a fresh slice (rather than mutating the input) keeps the
// resolved recovery universe independent of the config slice. Two batched multicalls (adapters'
// vault(), then those vaults' asset()); an entry whose reads revert is left zero and skipped by
// recovery (readVaultInventories needs a non-zero Asset). Errors only on a multicall transport failure.
func (r *reader) resolveVaults(ctx context.Context, vaults []recoveryVault) ([]recoveryVault, error) {
	out := make([]recoveryVault, len(vaults))
	for i := range vaults {
		out[i].Adapter = vaults[i].Adapter
	}
	if len(out) == 0 {
		return out, nil
	}
	vcalls := make([]chain.Call, len(out))
	for i := range out {
		vcalls[i] = chain.Call{Target: out[i].Adapter, AllowFailure: true, Data: llAdapter.PackVault()}
	}
	vres, err := r.chain.Multicall(ctx, vcalls)
	if err != nil {
		return nil, err
	}
	acalls := make([]chain.Call, len(out))
	for i := range out {
		if i < len(vres) && vres[i].Success {
			if vault, verr := llAdapter.UnpackVault(vres[i].ReturnData); verr == nil {
				out[i].Vault = vault
			}
		}
		// asset() reads the resolved vault; a zero target reverts (AllowFailure) and is skipped.
		acalls[i] = chain.Call{Target: out[i].Vault, AllowFailure: true, Data: erc4626b.PackAsset()}
	}
	ares, err := r.chain.Multicall(ctx, acalls)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if i < len(ares) && ares[i].Success {
			if asset, aerr := erc4626b.UnpackAsset(ares[i].ReturnData); aerr == nil {
				out[i].Asset = asset
			}
		}
		if out[i].Vault == (common.Address{}) || out[i].Asset == (common.Address{}) {
			r.log.Error(errors.New("adapter vault/asset unresolved"), "recovery entry skipped until restart",
				"adapter", out[i].Adapter.Hex())
		}
	}
	return out, nil
}

// readPermissionedVaultInventories returns the subset of readVaultInventories the executor is
// authorized to fill through: adapter.marketMaker() == executor, adapter.owner() == executor, or the
// marketMaker has delegated via adapter.isFiller(marketMaker, executor). Used in recovery so we never
// build a fill against an unauthorized adapter. Mirrors readPermissionedAdapterInventories in
// inventories.ts (marketMaker / owner / isFiller).
func (r *reader) readPermissionedVaultInventories(
	ctx context.Context, executor, tokenIn common.Address, vaults []recoveryVault,
) ([]solverInventory, error) {
	base, err := r.readVaultInventories(ctx, tokenIn, vaults)
	if err != nil || len(base) == 0 {
		return base, err
	}

	// 1) marketMaker() + owner() for each candidate adapter, in one multicall.
	calls := make([]chain.Call, 0, len(base)*2)
	for _, inv := range base {
		calls = append(calls,
			chain.Call{Target: inv.Adapter, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
			chain.Call{Target: inv.Adapter, AllowFailure: true, Data: llAdapter.PackOwner()},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("authorization multicall: got %d results, want %d", len(res), len(calls))
	}

	type authz struct {
		marketMaker, owner common.Address
		resolved           bool
	}
	auths := make([]authz, len(base))
	var fillerChecks []int // base indices needing an isFiller delegation check
	for i := range base {
		mm, ow := res[i*2], res[i*2+1]
		if !mm.Success || !ow.Success {
			continue
		}
		marketMaker, e1 := llAdapter.UnpackMarketMaker(mm.ReturnData)
		owner, e2 := llAdapter.UnpackOwner(ow.ReturnData)
		if e1 != nil || e2 != nil {
			continue
		}
		auths[i] = authz{marketMaker: marketMaker, owner: owner, resolved: true}
		if marketMaker != executor && owner != executor {
			fillerChecks = append(fillerChecks, i)
		}
	}

	// 2) isFiller(marketMaker, executor) for the adapters not directly owned, in one multicall.
	delegated := make(map[int]bool, len(fillerChecks))
	if len(fillerChecks) > 0 {
		fcalls := make([]chain.Call, len(fillerChecks))
		for j, i := range fillerChecks {
			fcalls[j] = chain.Call{Target: base[i].Adapter, AllowFailure: true, Data: llAdapter.PackIsFiller(auths[i].marketMaker, executor)}
		}
		fres, ferr := r.chain.Multicall(ctx, fcalls)
		if ferr != nil {
			return nil, ferr
		}
		for j, i := range fillerChecks {
			if j < len(fres) && fres[j].Success {
				if ok, derr := llAdapter.UnpackIsFiller(fres[j].ReturnData); derr == nil && ok {
					delegated[i] = true
				}
			}
		}
	}

	out := make([]solverInventory, 0, len(base))
	for i, inv := range base {
		a := auths[i]
		if a.resolved && (a.marketMaker == executor || a.owner == executor || delegated[i]) {
			out = append(out, inv)
		}
	}
	return out, nil
}

// dedupeByAdapter keeps the first recovery vault per distinct adapter, matching the de-dup in
// readAdapterInventories (keyed by adapter).
func dedupeVaultsByAdapter(in []recoveryVault) []recoveryVault {
	seen := make(map[common.Address]bool, len(in))
	out := make([]recoveryVault, 0, len(in))
	for _, v := range in {
		if seen[v.Adapter] {
			continue
		}
		seen[v.Adapter] = true
		out = append(out, v)
	}
	return out
}
