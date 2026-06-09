package rfq

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/adapter"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Parsed ABIs for packing Multicall3 sub-calls.
var (
	adapterABI = mustABI(adapter.LiquidLaneAdapterMetaData)
	vaultABI   = mustABI(erc4626.IERC4626MetaData)
	// erc20DecimalsABI is a minimal ERC-20 fragment — the quote path only needs decimals().
	erc20DecimalsABI = mustParseABI(`[{"inputs":[],"name":"decimals","outputs":[{"type":"uint8"}],` +
		`"stateMutability":"view","type":"function"}]`)
)

// readsPerAdapter is the number of Multicall3 sub-calls readVaultInventories issues per adapter
// (paused, vault, asset, getMaxAssets, getMaxRate) — mirrors readAdapterInventories in inventories.ts.
const readsPerAdapter = 5

func mustABI(md *bind.MetaData) abi.ABI {
	parsed, err := md.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("rfq: parse ABI: %v", err))
	}
	return *parsed
}

func mustParseABI(j string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(j))
	if err != nil {
		panic(fmt.Sprintf("rfq: parse ABI json: %v", err))
	}
	return parsed
}

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

// recoveryVault is one configured recovery candidate: a LiquidLane adapter, its vault, and the
// expected collateral hint. Mirrors InventorySource in inventories.ts.
type recoveryVault struct {
	Adapter   common.Address
	Vault     common.Address
	AssetHint common.Address
}

// tokenDecimals returns the ERC-20 decimals for token, caching the result.
func (r *reader) tokenDecimals(ctx context.Context, token common.Address) (int, error) {
	r.mu.Lock()
	if d, ok := r.decimals[token]; ok {
		r.mu.Unlock()
		return d, nil
	}
	r.mu.Unlock()

	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: token, Data: mustPack(erc20DecimalsABI, "decimals")}})
	if err != nil {
		return 0, err
	}
	if len(res) != 1 || !res[0].Success {
		return 0, errors.Errorf("erc20.decimals() reverted for %s", token)
	}
	vals, err := erc20DecimalsABI.Unpack("decimals", res[0].ReturnData)
	if err != nil {
		return 0, errors.Errorf("unpack decimals: %w", err)
	}
	d, ok := vals[0].(uint8)
	if !ok {
		return 0, errors.Errorf("decimals: unexpected type %T", vals[0])
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
			Data:         mustPack(adapterABI, "getAmountOut", tokenIn, amount),
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
		amt, derr := unpackBig(adapterABI, "getAmountOut", rr.ReturnData)
		if derr != nil {
			r.log.V(1).Error(derr, "getAmountOut decode failed; asset left unpriced", "asset", groups[i].asset.Hex())
			continue
		}
		out[groups[i].asset] = amt
	}
	return out, nil
}

// readVaultInventories reads, per adapter in ONE multicall, the LiquidLane views the strategy
// selector needs (paused, vault, IERC4626(vault).asset, getMaxAssets(tokenIn), getMaxRate(tokenIn)),
// and resolves asset decimals (cached). Used for strategy recovery when the quote-time strategy isn't
// cached (e.g. after a restart). Paused / failing / zero-liquidity adapters, and any whose vault()
// doesn't match the configured vault, are dropped. Direct legs only (DiscountID nil). Mirrors
// readAdapterInventories in inventories.ts.
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
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: mustPack(adapterABI, "paused")},
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: mustPack(adapterABI, "vault")},
			chain.Call{Target: v.Vault, AllowFailure: true, Data: mustPack(vaultABI, "asset")},
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: mustPack(adapterABI, "getMaxAssets", tokenIn)},
			chain.Call{Target: v.Adapter, AllowFailure: true, Data: mustPack(adapterABI, "getMaxRate", tokenIn)},
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
		paused, vaultAddr, assetRes := res[base], res[base+1], res[base+2]
		maxA, mr := res[base+3], res[base+4]
		if !vaultAddr.Success || !assetRes.Success || !maxA.Success || !mr.Success {
			continue
		}
		if p, perr := unpackBool(adapterABI, "paused", paused.ReturnData); paused.Success && perr == nil && p {
			continue
		}
		gotVault, verr := unpackAddress(adapterABI, "vault", vaultAddr.ReturnData)
		if verr != nil || gotVault != v.Vault {
			continue
		}
		asset, aerr := unpackAddress(vaultABI, "asset", assetRes.ReturnData)
		maxAssets, e1 := unpackBig(adapterABI, "getMaxAssets", maxA.ReturnData)
		maxRate, e2 := unpackBig(adapterABI, "getMaxRate", mr.ReturnData)
		if aerr != nil || e1 != nil || e2 != nil {
			continue
		}
		if maxAssets.Sign() <= 0 || maxRate.Sign() <= 0 {
			continue
		}
		decimals, derr := r.tokenDecimals(ctx, asset)
		if derr != nil {
			continue
		}
		out = append(out, solverInventory{
			Adapter: v.Adapter, Asset: asset, AssetDecimals: decimals,
			MaxAssets: maxAssets, MaxRate: maxRate, DiscountID: nil,
		})
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
			chain.Call{Target: inv.Adapter, AllowFailure: true, Data: mustPack(adapterABI, "marketMaker")},
			chain.Call{Target: inv.Adapter, AllowFailure: true, Data: mustPack(adapterABI, "owner")},
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
		marketMaker, e1 := unpackAddress(adapterABI, "marketMaker", mm.ReturnData)
		owner, e2 := unpackAddress(adapterABI, "owner", ow.ReturnData)
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
			fcalls[j] = chain.Call{Target: base[i].Adapter, AllowFailure: true, Data: mustPack(adapterABI, "isFiller", auths[i].marketMaker, executor)}
		}
		fres, ferr := r.chain.Multicall(ctx, fcalls)
		if ferr != nil {
			return nil, ferr
		}
		for j, i := range fillerChecks {
			if j < len(fres) && fres[j].Success {
				if ok, derr := unpackBool(adapterABI, "isFiller", fres[j].ReturnData); derr == nil && ok {
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

/* ───────── ABI pack/unpack helpers for multicall ───────── */

func mustPack(a abi.ABI, method string, args ...interface{}) []byte {
	data, err := a.Pack(method, args...)
	if err != nil {
		panic(fmt.Sprintf("rfq: pack %s: %v", method, err))
	}
	return data
}

func unpackBig(a abi.ABI, method string, data []byte) (*big.Int, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return nil, errors.Errorf("unpack %s: %w", method, err)
	}
	n, ok := vals[0].(*big.Int)
	if !ok {
		return nil, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return n, nil
}

func unpackBool(a abi.ABI, method string, data []byte) (bool, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return false, errors.Errorf("unpack %s: %w", method, err)
	}
	b, ok := vals[0].(bool)
	if !ok {
		return false, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return b, nil
}

func unpackAddress(a abi.ABI, method string, data []byte) (common.Address, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return common.Address{}, errors.Errorf("unpack %s: %w", method, err)
	}
	addr, ok := vals[0].(common.Address)
	if !ok {
		return common.Address{}, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return addr, nil
}
