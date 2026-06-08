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

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/curatorregistry"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Parsed ABIs for packing Multicall3 sub-calls.
var (
	adapterABI         = mustABI(adapter.InstantRedemptionAdapterMetaData)
	vaultABI           = mustABI(vaultv2.IVaultV2MetaData)
	curatorRegistryABI = mustABI(curatorregistry.ICuratorRegistryMetaData)
	// erc20DecimalsABI is a minimal ERC-20 fragment — the quote path only needs decimals().
	erc20DecimalsABI = mustParseABI(`[{"inputs":[],"name":"decimals","outputs":[{"type":"uint8"}],` +
		`"stateMutability":"view","type":"function"}]`)
)

// readsPerVault is the number of Multicall3 sub-calls readVaultInventories issues per vault.
const readsPerVault = 6

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

// reader performs the quote-path on-chain reads, batching via Multicall3. Token decimals are cached;
// the HTTP server serves quotes concurrently, so the cache is mutex-guarded.
type reader struct {
	chain   *chain.Client
	adapter common.Address
	log     logr.Logger

	mu       sync.Mutex
	decimals map[common.Address]int
}

func newReader(c *chain.Client, adapterAddr common.Address, log logr.Logger) *reader {
	return &reader{chain: c, adapter: adapterAddr, log: log, decimals: make(map[common.Address]int)}
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

// amountsOut fetches adapter.getAmountOut(tokenIn, asset, amount) for each distinct asset in one
// multicall. A reverting sub-call is omitted from the result (that asset is skipped by
// the selector), so the map only holds successfully-priced assets.
func (r *reader) amountsOut(
	ctx context.Context, tokenIn common.Address, assets []common.Address, amount *big.Int,
) (map[common.Address]*big.Int, error) {
	uniq := dedupeAddrs(assets)
	if len(uniq) == 0 {
		return map[common.Address]*big.Int{}, nil
	}
	calls := make([]chain.Call, len(uniq))
	for i, c := range uniq {
		calls[i] = chain.Call{
			Target:       r.adapter,
			AllowFailure: true,
			Data:         mustPack(adapterABI, "getAmountOut", tokenIn, c, amount),
		}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	out := make(map[common.Address]*big.Int, len(uniq))
	for i, rr := range res {
		if !rr.Success {
			r.log.V(1).Info("getAmountOut reverted; asset left unpriced", "asset", uniq[i].Hex())
			continue
		}
		amt, derr := unpackBig(adapterABI, "getAmountOut", rr.ReturnData)
		if derr != nil {
			r.log.V(1).Error(derr, "getAmountOut decode failed; asset left unpriced", "asset", uniq[i].Hex())
			continue
		}
		out[uniq[i]] = amt
	}
	return out, nil
}

// readVaultInventories reads, for each vault in ONE multicall, the adapter views the strategy
// selector needs (isPaused, collateral, getMaxAssets, limit, allocated, getMaxRate), and resolves
// collateral decimals (cached). Used for strategy recovery when the quote-time strategy isn't
// cached (e.g. after a restart). Paused / failing / zero-liquidity vaults are dropped. Direct legs
// only (discountId nil); permissioned/discount inventories are P3.
func (r *reader) readVaultInventories(
	ctx context.Context, tokenIn common.Address, vaults []common.Address,
) ([]solverInventory, error) {
	vaults = dedupeAddrs(vaults)
	if len(vaults) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, 0, len(vaults)*readsPerVault)
	for _, v := range vaults {
		calls = append(calls,
			chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "isPaused", v)},
			chain.Call{Target: v, AllowFailure: true, Data: mustPack(vaultABI, "collateral")},
			chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "getMaxAssets", v)},
			chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "limit", v, tokenIn)},
			chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "allocated", v, tokenIn)},
			chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "getMaxRate", v, tokenIn)},
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
		base := i * readsPerVault
		paused, coll, maxA := res[base], res[base+1], res[base+2]
		lim, alloc, mr := res[base+3], res[base+4], res[base+5]
		if !coll.Success || !maxA.Success || !lim.Success || !alloc.Success || !mr.Success {
			continue
		}
		if p, perr := unpackBool(adapterABI, "isPaused", paused.ReturnData); paused.Success && perr == nil && p {
			continue
		}
		asset, cerr := unpackAddress(vaultABI, "collateral", coll.ReturnData)
		maxAssets, e1 := unpackBig(adapterABI, "getMaxAssets", maxA.ReturnData)
		limit, e2 := unpackBig(adapterABI, "limit", lim.ReturnData)
		allocated, e3 := unpackBig(adapterABI, "allocated", alloc.ReturnData)
		maxRate, e4 := unpackBig(adapterABI, "getMaxRate", mr.ReturnData)
		if cerr != nil || e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			continue
		}
		maxOut := capAssetsByTokenLimit(maxAssets, limit, allocated)
		if maxOut.Sign() <= 0 || maxRate.Sign() <= 0 {
			continue
		}
		decimals, derr := r.tokenDecimals(ctx, asset)
		if derr != nil {
			continue
		}
		// The on-chain Swap takes the vault address in its "adapter"/vault slot, so Adapter == v.
		out = append(out, solverInventory{
			Adapter: v, Asset: asset, AssetDecimals: decimals,
			MaxAssets: maxOut, MaxRate: maxRate, DiscountID: nil,
		})
	}
	return out, nil
}

// readPermissionedVaultInventories returns the subset of readVaultInventories the executor is
// authorized to fill through: vault marketMaker == executor, or curatorRegistry curator == executor,
// or the marketMaker has delegated via isFiller(marketMaker, executor). Used in recovery so we never
// build a fill against an unauthorized vault. With no curatorRegistry configured it returns nothing.
func (r *reader) readPermissionedVaultInventories(
	ctx context.Context, executor, curatorRegistry, tokenIn common.Address, vaults []common.Address,
) ([]solverInventory, error) {
	if curatorRegistry == (common.Address{}) {
		return nil, nil
	}
	base, err := r.readVaultInventories(ctx, tokenIn, vaults)
	if err != nil || len(base) == 0 {
		return base, err
	}

	// 1) marketMaker(vault) + curatorRegistry.getCurator(vault) for each candidate, in one multicall.
	calls := make([]chain.Call, 0, len(base)*2)
	for _, inv := range base {
		calls = append(calls,
			chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "marketMaker", inv.Adapter)},
			chain.Call{Target: curatorRegistry, AllowFailure: true, Data: mustPack(curatorRegistryABI, "getCurator", inv.Adapter)},
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
		marketMaker, curator common.Address
		resolved             bool
	}
	auths := make([]authz, len(base))
	var fillerChecks []int // base indices needing an isFiller delegation check
	for i := range base {
		mm, cu := res[i*2], res[i*2+1]
		if !mm.Success || !cu.Success {
			continue
		}
		marketMaker, e1 := unpackAddress(adapterABI, "marketMaker", mm.ReturnData)
		curator, e2 := unpackAddress(curatorRegistryABI, "getCurator", cu.ReturnData)
		if e1 != nil || e2 != nil {
			continue
		}
		auths[i] = authz{marketMaker: marketMaker, curator: curator, resolved: true}
		if marketMaker != executor && curator != executor {
			fillerChecks = append(fillerChecks, i)
		}
	}

	// 2) isFiller(marketMaker, executor) for the vaults not directly owned, in one multicall.
	delegated := make(map[int]bool, len(fillerChecks))
	if len(fillerChecks) > 0 {
		fcalls := make([]chain.Call, len(fillerChecks))
		for j, i := range fillerChecks {
			fcalls[j] = chain.Call{Target: r.adapter, AllowFailure: true, Data: mustPack(adapterABI, "isFiller", auths[i].marketMaker, executor)}
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
		if a.resolved && (a.marketMaker == executor || a.curator == executor || delegated[i]) {
			out = append(out, inv)
		}
	}
	return out, nil
}

// capAssetsByTokenLimit = min(maxAssets, max(limit-allocated, 0)); limit==0 means uncapped.
func capAssetsByTokenLimit(maxAssets, limit, allocated *big.Int) *big.Int {
	if maxAssets.Sign() <= 0 {
		return new(big.Int)
	}
	if limit.Sign() == 0 {
		return new(big.Int).Set(maxAssets)
	}
	remaining := new(big.Int).Sub(limit, allocated)
	if remaining.Sign() < 0 {
		remaining = new(big.Int)
	}
	return minBig(remaining, maxAssets)
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func dedupeAddrs(in []common.Address) []common.Address {
	seen := make(map[common.Address]bool, len(in))
	out := make([]common.Address, 0, len(in))
	for _, a := range in {
		if seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
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
