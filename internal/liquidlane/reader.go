package liquidlane

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/lens"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	DefaultMaxTokensPerAdapter = 64
	inventoryReadsPerRoute     = 5
	fillReadsPerRoute          = 4
)

var (
	llAdapter = adapter.NewLiquidLaneAdapter()
	erc4626b  = erc4626.NewIERC4626()
	vaultV2b  = vaultv2.NewIVaultV2()
	lensB     = lens.NewFrontendLiquidityLens()
)

type Reader struct {
	chain liquidLaneBackend
	log   logr.Logger
	dec   decimalsReader

	chainID             int64
	maxTokensPerAdapter int
	// lens is the FrontendLiquidityLens address. When non-zero, swappable headroom is read from the lens's
	// cross-adapter deallocation-cascade estimate instead of the adapter's own getMaxAssets(tokenToRedeem);
	// zero falls back to the adapter getter.
	lens common.Address
}

type gasAdapterState struct {
	owner       common.Address
	marketMaker common.Address
	state       *liquidlanegas.AdapterState
}

type liquidLaneBackend interface {
	ChainID() *big.Int
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
}

type decimalsReader interface {
	Get(ctx context.Context, token common.Address) (int, error)
}

func NewReader(c *chain.Client, log logr.Logger, liquidityLens common.Address) *Reader {
	return &Reader{
		chain:               c,
		log:                 log,
		dec:                 chain.NewDecimals(c),
		chainID:             c.ChainID().Int64(),
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
		lens:                liquidityLens,
	}
}

// maxAssetsCall builds the getMaxAssets sub-call for a route: via the lens when configured (which models
// the delegator's cross-adapter deallocation cascade the adapter's own getter overstates), else the
// adapter itself. Both return a single uint256, so the result unpacks identically via
// llAdapter.UnpackGetMaxAssets regardless of source.
func (r *Reader) maxAssetsCall(adapterAddr, tokenToRedeem common.Address) chain.Call {
	if r.lens != (common.Address{}) {
		return chain.Call{Target: r.lens, AllowFailure: true, Data: lensB.PackGetMaxAssets0(adapterAddr, tokenToRedeem)}
	}
	return chain.Call{Target: adapterAddr, AllowFailure: true, Data: llAdapter.PackGetMaxAssets(tokenToRedeem)}
}

// TokenDecimals returns the cached ERC-20 decimals used to build typed routes
// when an upstream protocol omits input-token metadata.
func (r *Reader) TokenDecimals(ctx context.Context, token common.Address) (int, error) {
	return r.dec.Get(ctx, token)
}

// TokenMetadata shares the reader's existing chain-scoped decimals cache for optional log fields.
func (r *Reader) TokenMetadata() *chain.Decimals {
	if r == nil {
		return nil
	}
	cache, _ := r.dec.(*chain.Decimals)
	return cache
}

func (r *Reader) ResolveAdapters(ctx context.Context, adapters []common.Address) ([]Adapter, error) {
	return r.resolveAdapters(ctx, adapters, nil)
}

// DiscoverAdapters resolves unconfigured identities in batches. Invalid contract
// metadata skips only that adapter so it can retry on a later discovery poll;
// transport or malformed aggregate responses still fail the refresh. Each skipped adapter goes to trace.
func (r *Reader) DiscoverAdapters(ctx context.Context, adapters []common.Address, trace ...DropTrace) ([]Adapter, error) {
	return r.resolveAdapters(ctx, adapters, dropSink(ctx, trace))
}

// resolveAdapters fails on the first invalid adapter unless skip is set, which then receives it.
func (r *Reader) resolveAdapters(ctx context.Context, adapters []common.Address, skip DropTrace) ([]Adapter, error) {
	adapters = dedupeAddresses(adapters)
	if len(adapters) == 0 {
		return nil, nil
	}
	vaultCalls := make([]chain.Call, len(adapters))
	for i, a := range adapters {
		vaultCalls[i] = chain.Call{Target: a, AllowFailure: true, Data: llAdapter.PackVault()}
	}
	vaultResults, err := r.chain.Multicall(ctx, vaultCalls)
	if err != nil {
		return nil, err
	}
	if len(vaultResults) != len(vaultCalls) {
		return nil, errors.Errorf("liquidlane: vault multicall: got %d results, want %d", len(vaultResults), len(vaultCalls))
	}
	resolved := make([]Adapter, 0, len(adapters))
	assetCalls := make([]chain.Call, 0, len(adapters))
	for i, adapter := range adapters {
		vault, vaultErr := resolveAdapterVault(adapter, vaultResults[i])
		if vaultErr != nil {
			if skip == nil {
				return nil, vaultErr
			}
			skip(Drop{Adapter: adapter, Reason: vaultErr.Error()})
			continue
		}
		resolved = append(resolved, Adapter{Adapter: adapter, Vault: vault})
		assetCalls = append(assetCalls, chain.Call{Target: vault, AllowFailure: true, Data: erc4626b.PackAsset()})
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	assetResults, err := r.chain.Multicall(ctx, assetCalls)
	if err != nil {
		return nil, err
	}
	if len(assetResults) != len(assetCalls) {
		return nil, errors.Errorf("liquidlane: asset multicall: got %d results, want %d", len(assetResults), len(assetCalls))
	}
	out := make([]Adapter, 0, len(resolved))
	for i, adapter := range resolved {
		metadata, metadataErr := r.resolveAdapterAsset(ctx, adapter, assetResults[i])
		if metadataErr != nil {
			if skip == nil {
				return nil, metadataErr
			}
			skip(Drop{Adapter: adapter.Adapter, Reason: metadataErr.Error()})
			continue
		}
		out = append(out, metadata)
	}
	return out, nil
}

func resolveAdapterVault(adapter common.Address, result chain.CallResult) (common.Address, error) {
	if !result.Success {
		return common.Address{}, errors.Errorf("liquidlane: resolve adapter %s %s", adapter.Hex(), callFailure("vault", result))
	}
	vault, err := llAdapter.UnpackVault(result.ReturnData)
	if err != nil {
		return common.Address{}, errors.Errorf("liquidlane: resolve adapter %s vault: %w", adapter.Hex(), err)
	}
	if vault == (common.Address{}) {
		return common.Address{}, errors.Errorf("liquidlane: resolve adapter %s vault: zero address", adapter.Hex())
	}
	return vault, nil
}

func (r *Reader) resolveAdapterAsset(ctx context.Context, adapter Adapter, result chain.CallResult) (Adapter, error) {
	if !result.Success {
		return Adapter{}, errors.Errorf("liquidlane: resolve adapter %s vault %s %s",
			adapter.Adapter.Hex(), adapter.Vault.Hex(), callFailure("asset", result))
	}
	asset, err := erc4626b.UnpackAsset(result.ReturnData)
	if err != nil {
		return Adapter{}, errors.Errorf("liquidlane: resolve adapter %s vault %s asset: %w", adapter.Adapter.Hex(), adapter.Vault.Hex(), err)
	}
	if asset == (common.Address{}) {
		return Adapter{}, errors.Errorf("liquidlane: resolve adapter %s vault %s asset: zero address", adapter.Adapter.Hex(), adapter.Vault.Hex())
	}
	decimals, err := r.dec.Get(ctx, asset)
	if err != nil {
		return Adapter{}, errors.Errorf("liquidlane: resolve adapter %s tokenOut %s decimals: %w", adapter.Adapter.Hex(), asset.Hex(), err)
	}
	adapter.TokenOut, adapter.TokenOutDecimals = asset, decimals
	return adapter, nil
}

func (r *Reader) ResolveRoutes(ctx context.Context, adapters []common.Address) ([]Route, error) {
	resolved, err := r.ResolveAdapters(ctx, adapters)
	if err != nil {
		return nil, err
	}
	lengths, err := r.readTokenCounts(ctx, resolved)
	if err != nil {
		return nil, err
	}

	type tokenReq struct {
		adapterIndex int
		tokenIndex   int
	}
	var reqs []tokenReq
	var tokenCalls []chain.Call
	for i, n := range lengths {
		for j := range n {
			reqs = append(reqs, tokenReq{adapterIndex: i, tokenIndex: j})
			tokenCalls = append(tokenCalls, chain.Call{
				Target:       resolved[i].Adapter,
				AllowFailure: true,
				Data:         llAdapter.PackTokensToRedeem(big.NewInt(int64(j))),
			})
		}
	}
	if len(tokenCalls) == 0 {
		return nil, nil
	}
	res, err := r.chain.Multicall(ctx, tokenCalls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(tokenCalls) {
		return nil, errors.Errorf("liquidlane: tokensToRedeem multicall: got %d results, want %d", len(res), len(tokenCalls))
	}

	routes := make([]Route, 0, len(res))
	for i, call := range res {
		req := reqs[i]
		resolvedAdapter := resolved[req.adapterIndex]
		if !call.Success {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s %s",
				resolvedAdapter.Adapter.Hex(),
				callFailure(fmt.Sprintf("tokensToRedeem(%d)", req.tokenIndex), call),
			)
		}
		tokenIn, unpackErr := llAdapter.UnpackTokensToRedeem(call.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem[%d]: %w",
				resolvedAdapter.Adapter.Hex(),
				req.tokenIndex,
				unpackErr,
			)
		}
		if tokenIn == (common.Address{}) {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem[%d]: zero address",
				resolvedAdapter.Adapter.Hex(),
				req.tokenIndex,
			)
		}
		route, routeErr := r.resolveRouteForToken(ctx, resolvedAdapter, tokenIn)
		if routeErr != nil {
			return nil, routeErr
		}
		routes = append(routes, route)
	}
	return compactRoutes(routes), nil
}

// RoutesForToken resolves tokenIn on each adapter; each unresolved route is logged and goes to trace.
func (r *Reader) RoutesForToken(ctx context.Context, adapters []Adapter, tokenIn common.Address, trace ...DropTrace) []Route {
	out := make([]Route, 0, len(adapters))
	for _, a := range dedupeAdapters(adapters) {
		route, err := r.resolveRouteForToken(ctx, a, tokenIn)
		if err != nil {
			r.log.Error(err, "liquidlane: route unresolved",
				"adapter", a.Adapter.Hex(),
				"tokenIn", tokenIn.Hex(),
			)
			if len(trace) > 0 { // Already logged above, so no V(1) fallback.
				dropSink(ctx, trace)(Drop{Adapter: a.Adapter, TokenIn: tokenIn, Reason: err.Error()})
			}
			continue
		}
		out = append(out, route)
	}
	return compactRoutes(out)
}

// readPaused returns current pause state for each successfully decoded adapter; drop gets the rest.
func (r *Reader) readPaused(ctx context.Context, adapters []common.Address, drop DropTrace) (map[common.Address]bool, error) {
	adapters = dedupeAddresses(adapters)
	if len(adapters) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, len(adapters))
	for i, address := range adapters {
		calls[i] = chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackPaused()}
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("liquidlane: paused multicall: got %d results, want %d", len(results), len(calls))
	}
	out := make(map[common.Address]bool, len(adapters))
	for i, result := range results {
		paused, reason := decodeCall("paused", result, llAdapter.UnpackPaused)
		if reason != "" {
			drop(Drop{Adapter: adapters[i], Reason: reason})
			continue
		}
		out[adapters[i]] = paused
	}
	return out, nil
}

// ReadInventory reads executable inventory for routes; each route left out goes to trace.
func (r *Reader) ReadInventory(ctx context.Context, routes []Route, trace ...DropTrace) ([]Inventory, error) {
	return r.readInventory(ctx, routes, false, dropSink(ctx, trace))
}

func (r *Reader) readInventory(ctx context.Context, routes []Route, keepZero bool, drop DropTrace) ([]Inventory, error) {
	routes = compactRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, 0, len(routes)*inventoryReadsPerRoute)
	for _, route := range routes {
		probe, ok := OraclePriceProbe(route.TokenInDecimals, route.TokenOutDecimals)
		if !ok {
			probe = new(big.Int) // Keeps the read layout; the price stays unknown.
		}
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
			r.maxAssetsCall(route.Adapter, route.TokenIn),
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetMaxRate(route.TokenIn)},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMinDiscount(route.TokenIn)},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetAmountOut(route.TokenIn, probe)},
		)
	}
	res, lensFailures, err := r.inventoryCalls(ctx, routes, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("liquidlane: inventory multicall: got %d results, want %d", len(res), len(calls))
	}
	out := make([]Inventory, 0, len(routes))
	for i, route := range routes {
		base := i * inventoryReadsPerRoute
		inventory, reason := r.inventoryItem(route, res[base:base+inventoryReadsPerRoute], lensFailures[base+1], keepZero)
		if reason != "" {
			drop(Drop{Adapter: route.Adapter, TokenIn: route.TokenIn, Reason: reason})
			continue
		}
		out = append(out, inventory)
	}
	return out, nil
}

// inventoryItem decodes one route's inventory reads, or names the check that drops the route.
func (r *Reader) inventoryItem(route Route, res []chain.CallResult, lensFailure string, keepZero bool) (Inventory, string) {
	if reason := pauseReason(res[0]); reason != "" {
		return Inventory{}, reason
	}
	if lensFailure != "" {
		return Inventory{}, lensFailure
	}
	maxAssets, reason := decodeCall("getMaxAssets", res[1], llAdapter.UnpackGetMaxAssets)
	if reason != "" {
		return Inventory{}, reason
	}
	maxRate, reason := decodeCall("getMaxRate", res[2], llAdapter.UnpackGetMaxRate)
	if reason != "" {
		return Inventory{}, reason
	}
	minDiscount, reason := decodeCall("minDiscount", res[3], llAdapter.UnpackMinDiscount)
	if reason != "" {
		return Inventory{}, reason
	}
	if maxAssets == nil || maxRate == nil {
		return Inventory{}, "getMaxAssets or getMaxRate returned no value"
	}
	if reason = minDiscountReason(minDiscount); reason != "" {
		return Inventory{}, reason
	}
	if !keepZero && maxAssets.Sign() <= 0 {
		return Inventory{}, "getMaxAssets " + maxAssets.String() + " is not positive"
	}
	if !keepZero && maxRate.Sign() <= 0 {
		return Inventory{}, "getMaxRate " + maxRate.String() + " is not positive"
	}
	price, reason := r.oraclePrice(route, res[4], maxRate, minDiscount)
	if reason != "" {
		return Inventory{}, reason
	}
	inventory := DirectInventory(route, maxAssets, maxRate)
	inventory.AdapterMinDiscount = CloneBig(minDiscount)
	inventory.Price = price
	return inventory, ""
}

// inventoryCalls keeps capacity calculation in the lens and reads all inventory in one latest
// Multicall. Each adapter's token batch performs the shared sweep/cascade only once. Capacity slots
// the lens could not fill are keyed in the returned map with the reason.
func (r *Reader) inventoryCalls(
	ctx context.Context, routes []Route, calls []chain.Call,
) ([]chain.CallResult, map[int]string, error) {
	if r.lens == (common.Address{}) {
		results, err := r.chain.Multicall(ctx, calls)
		return results, nil, err
	}
	type lensBatch struct {
		adapter common.Address
		tokens  []common.Address
		slots   []int
	}
	var batches []lensBatch
	byAdapter := make(map[common.Address]int, len(routes))
	for i, route := range routes {
		index, ok := byAdapter[route.Adapter]
		if !ok {
			index = len(batches)
			byAdapter[route.Adapter] = index
			batches = append(batches, lensBatch{adapter: route.Adapter})
		}
		batches[index].tokens = append(batches[index].tokens, route.TokenIn)
		batches[index].slots = append(batches[index].slots, i*inventoryReadsPerRoute+1)
	}
	batchCalls := make([]chain.Call, 0, len(calls)+len(batches)-len(routes))
	for _, batch := range batches {
		batchCalls = append(batchCalls, chain.Call{
			Target: r.lens, AllowFailure: true, Data: lensB.PackGetMaxAssetsBatch(batch.adapter, batch.tokens),
		})
	}
	slots := make([]int, 0, len(calls)-len(routes))
	for i, call := range calls {
		if i%inventoryReadsPerRoute != 1 {
			batchCalls = append(batchCalls, call)
			slots = append(slots, i)
		}
	}
	results, err := r.chain.Multicall(ctx, batchCalls)
	if err != nil {
		return nil, nil, err
	}
	if len(results) != len(batchCalls) {
		return nil, nil, errors.Errorf("liquidlane: inventory lens multicall: got %d results, want %d", len(results), len(batchCalls))
	}
	out := make([]chain.CallResult, len(calls))
	for i, slot := range slots {
		out[slot] = results[len(batches)+i]
	}
	failures := make(map[int]string)
	for i, batch := range batches {
		capacities, reason := decodeCall("lens getMaxAssetsBatch", results[i], lensB.UnpackGetMaxAssetsBatch)
		if reason == "" && len(capacities) != len(batch.tokens) {
			reason = fmt.Sprintf("lens getMaxAssetsBatch returned %d capacities for %d tokens", len(capacities), len(batch.tokens))
		}
		for j, slot := range batch.slots {
			switch {
			case reason != "":
				failures[slot] = reason
			case !capacities[j].Success || capacities[j].MaxAssets == nil:
				failures[slot] = "lens getMaxAssetsBatch: getMaxAssets failed inside the lens"
			default:
				out[slot] = chain.CallResult{Success: true, ReturnData: capacities[j].MaxAssets.FillBytes(make([]byte, 32))}
			}
		}
	}
	return out, failures, nil
}

// oraclePrice decodes the oracle price read through getAmountOut at the price probe. getMaxRate
// derives from the same price in the same block, so a mismatch means the adapter no longer pays
// like DiscountedAmountOut and the route fails closed. A nil price with no reason leaves the route
// on rate-based pricing when its decimals cannot express the probe.
func (r *Reader) oraclePrice(route Route, result chain.CallResult, maxRate, minDiscount *big.Int) (*big.Int, string) {
	if _, ok := OraclePriceProbe(route.TokenInDecimals, route.TokenOutDecimals); !ok {
		return nil, ""
	}
	price, reason := decodeCall("getAmountOut(price probe)", result, llAdapter.UnpackGetAmountOut)
	if reason != "" {
		return nil, reason
	}
	if price == nil || price.Sign() <= 0 {
		return nil, fmt.Sprintf("oracle price %v is not positive", price)
	}
	if expected := DiscountedRate(price, minDiscount); expected.Cmp(maxRate) != 0 {
		r.log.Error(errors.New("adapter max rate does not match its oracle price"), "liquidlane: route skipped",
			"adapter", route.Adapter.Hex(),
			"tokenIn", route.TokenIn.Hex(),
			"price", price.String(),
			"minDiscountPpm", minDiscount.String(),
			"maxRate", maxRate.String(),
			"expectedMaxRate", expected.String(),
		)
		return nil, fmt.Sprintf("getMaxRate %s does not match oracle price %s at minDiscount %s (expected %s)",
			maxRate, price, minDiscount, expected)
	}
	return price, ""
}

// ReadGasSnapshot returns the latest adapter-local acquire balances and shared vault liquidity needed
// to predict LiquidLane swap gas. Partially unread state remains absent and is priced as RouteUnknown;
// each unread piece goes to trace.
func (r *Reader) ReadGasSnapshot(ctx context.Context, routes []Route, trace ...DropTrace) (*liquidlanegas.Snapshot, error) {
	return r.readGasSnapshot(ctx, routes, dropSink(ctx, trace))
}

func (r *Reader) readGasSnapshot(ctx context.Context, routes []Route, drop DropTrace) (*liquidlanegas.Snapshot, error) {
	routes = compactRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	type adapterRoutes struct {
		adapter common.Address
		vault   common.Address
		routes  []Route
	}
	byAdapter := make(map[common.Address]*adapterRoutes, len(routes))
	ordered := make([]*adapterRoutes, 0, len(routes))
	for _, route := range routes {
		entry := byAdapter[route.Adapter]
		if entry == nil {
			entry = &adapterRoutes{adapter: route.Adapter, vault: route.Vault}
			byAdapter[route.Adapter] = entry
			ordered = append(ordered, entry)
		}
		entry.routes = append(entry.routes, route)
	}

	headCalls := make([]chain.Call, 0, len(ordered)*2)
	for _, entry := range ordered {
		headCalls = append(headCalls,
			chain.Call{Target: entry.adapter, AllowFailure: true, Data: llAdapter.PackOwner()},
			chain.Call{Target: entry.adapter, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
		)
	}
	vaults := make([]common.Address, 0, len(ordered))
	seenVaults := make(map[common.Address]bool, len(ordered))
	for _, entry := range ordered {
		if !seenVaults[entry.vault] {
			seenVaults[entry.vault] = true
			vaults = append(vaults, entry.vault)
		}
	}
	for _, vault := range vaults {
		headCalls = append(headCalls,
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackFreeAssets()},
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackWithdrawable()},
		)
	}
	headResults, err := r.chain.Multicall(ctx, headCalls)
	if err != nil {
		return nil, err
	}
	if len(headResults) != len(headCalls) {
		return nil, errors.Errorf("liquidlane: gas state head multicall: got %d results, want %d", len(headResults), len(headCalls))
	}

	states := make(map[common.Address]*gasAdapterState, len(ordered))
	for i, entry := range ordered {
		base := i * 2
		owner, reason := decodeCall("owner", headResults[base], llAdapter.UnpackOwner)
		if reason != "" {
			drop(Drop{Adapter: entry.adapter, Reason: "gas state: " + reason})
			continue
		}
		marketMaker, reason := decodeCall("marketMaker", headResults[base+1], llAdapter.UnpackMarketMaker)
		if reason != "" {
			drop(Drop{Adapter: entry.adapter, Reason: "gas state: " + reason})
			continue
		}
		states[entry.adapter] = &gasAdapterState{
			owner: owner, marketMaker: marketMaker,
			state: &liquidlanegas.AdapterState{
				Vault: entry.vault, Acquire: make(map[common.Address]*big.Int, len(entry.routes)),
			},
		}
	}
	vaultStates := make(map[common.Address]*liquidlanegas.VaultState, len(vaults))
	vaultBase := len(ordered) * 2
	for i, vault := range vaults {
		base := vaultBase + i*2
		freeAssets, reason := decodeCall("freeAssets", headResults[base], vaultV2b.UnpackFreeAssets)
		withdrawable, withdrawableReason := decodeCall("withdrawable", headResults[base+1], vaultV2b.UnpackWithdrawable)
		if reason == "" {
			reason = withdrawableReason
		}
		if reason == "" && (freeAssets == nil || withdrawable == nil) {
			reason = "freeAssets or withdrawable returned no value"
		}
		if reason != "" {
			for _, entry := range ordered {
				if entry.vault == vault {
					drop(Drop{Adapter: entry.adapter, Reason: "gas state: vault " + vault.Hex() + " " + reason})
				}
			}
			continue
		}
		vaultStates[vault] = &liquidlanegas.VaultState{
			FreeAssets: new(big.Int).Set(freeAssets), Withdrawable: new(big.Int).Set(withdrawable),
		}
	}

	type acquireRead struct {
		adapter common.Address
		token   common.Address
		holder  common.Address
	}
	acquireCalls := make([]chain.Call, 0, len(routes)*2)
	reads := make([]acquireRead, 0, len(routes)*2)
	for _, entry := range ordered {
		state := states[entry.adapter]
		if state == nil {
			continue
		}
		for _, route := range entry.routes {
			acquireCalls = append(acquireCalls, chain.Call{
				Target: entry.adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(route.TokenIn, state.owner),
			})
			reads = append(reads, acquireRead{adapter: entry.adapter, token: route.TokenIn, holder: state.owner})
			if state.marketMaker != state.owner {
				acquireCalls = append(acquireCalls, chain.Call{
					Target: entry.adapter, AllowFailure: true,
					Data: llAdapter.PackAcquireBalance(route.TokenIn, state.marketMaker),
				})
				reads = append(reads, acquireRead{
					adapter: entry.adapter,
					token:   route.TokenIn,
					holder:  state.marketMaker,
				})
			}
		}
	}
	if len(acquireCalls) == 0 {
		return gasSnapshot(states, vaultStates), nil
	}
	acquireResults, err := r.chain.Multicall(ctx, acquireCalls)
	if err != nil {
		return nil, err
	}
	if len(acquireResults) != len(acquireCalls) {
		return nil, errors.Errorf("liquidlane: gas state acquire multicall: got %d results, want %d", len(acquireResults), len(acquireCalls))
	}
	for i, read := range reads {
		amount, reason := decodeCall("acquireBalance", acquireResults[i], llAdapter.UnpackAcquireBalance)
		if reason == "" && (amount == nil || amount.Sign() < 0) {
			reason = fmt.Sprintf("acquireBalance %v is negative", amount)
		}
		if reason != "" {
			drop(Drop{Adapter: read.adapter, TokenIn: read.token, Reason: "gas state: holder " + read.holder.Hex() + " " + reason})
			continue
		}
		state := states[read.adapter]
		if state == nil {
			return nil, errors.Errorf("liquidlane: missing gas state for adapter %s", read.adapter.Hex())
		}
		if state.state.Acquire[read.token] == nil {
			state.state.Acquire[read.token] = new(big.Int)
		}
		state.state.Acquire[read.token].Add(state.state.Acquire[read.token], amount)
	}
	return gasSnapshot(states, vaultStates), nil
}

func gasSnapshot(
	in map[common.Address]*gasAdapterState,
	vaults map[common.Address]*liquidlanegas.VaultState,
) *liquidlanegas.Snapshot {
	adapters := make(map[common.Address]*liquidlanegas.AdapterState, len(in))
	for adapter, state := range in {
		adapters[adapter] = state.state
	}
	return &liquidlanegas.Snapshot{Adapters: adapters, Vaults: vaults}
}

// ReadAdapterSnapshot reads one complete LiquidLane adapter view for solvers that consume all routes.
// An unresolved pause, authorization, liquidity or inventory read fails with the reads that came back
// unusable. A failed isFiller or acquire-balance read only leaves the adapter unauthorized or the
// balance unset; every drop is logged at V(1).
func (r *Reader) ReadAdapterSnapshot(
	ctx context.Context,
	adapterAddress common.Address,
	filler common.Address,
) (AdapterSnapshot, error) {
	routes, err := r.ResolveRoutes(ctx, []common.Address{adapterAddress})
	if err != nil {
		return AdapterSnapshot{}, err
	}
	if len(routes) == 0 {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter has no resolved routes")
	}
	drop, drops := collectDrops(ctx)
	pausedByAdapter, err := r.readPaused(ctx, []common.Address{adapterAddress}, drop)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	paused, pausedResolved := pausedByAdapter[adapterAddress]
	if !pausedResolved {
		return AdapterSnapshot{}, errors.Errorf("liquidlane: adapter pause state unresolved: %s", drops())
	}
	drop, drops = collectDrops(ctx)
	auth, err := r.ReadAuth(ctx, []common.Address{adapterAddress}, filler, drop)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	if len(auth) != 1 || auth[0].Adapter != adapterAddress {
		return AdapterSnapshot{}, errors.Errorf("liquidlane: adapter authorization unresolved: %s", drops())
	}
	drop, drops = collectDrops(ctx)
	gasState, err := r.readGasSnapshot(ctx, routes, drop)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	adapterState := gasState.Adapters[adapterAddress]
	vaultState := gasState.Vaults[routes[0].Vault]
	if adapterState == nil || vaultState == nil {
		return AdapterSnapshot{}, errors.Errorf("liquidlane: adapter liquidity state unresolved: %s", drops())
	}

	inventoryByRoute := make(map[RouteID]Inventory, len(routes))
	if paused {
		for _, route := range routes {
			inventoryByRoute[route.ID] = DirectInventory(route, new(big.Int), new(big.Int))
		}
	} else {
		drop, drops = collectDrops(ctx)
		inventory, inventoryErr := r.readInventory(ctx, routes, true, drop)
		if inventoryErr != nil {
			return AdapterSnapshot{}, inventoryErr
		}
		for _, item := range inventory {
			inventoryByRoute[item.ID] = item
		}
		if len(inventoryByRoute) != len(routes) {
			return AdapterSnapshot{}, errors.Errorf("liquidlane: adapter inventory unresolved: %s", drops())
		}
	}

	first := routes[0]
	out := AdapterSnapshot{
		Adapter: Adapter{
			Adapter: first.Adapter, Vault: first.Vault,
			TokenOut: first.TokenOut, TokenOutDecimals: first.TokenOutDecimals,
		},
		Paused: paused, Authorized: auth[0].Authorized,
		FreeAssets: CloneBig(vaultState.FreeAssets), Withdrawable: CloneBig(vaultState.Withdrawable),
		Routes: make([]RouteSnapshot, 0, len(routes)),
	}
	for _, route := range routes {
		item := inventoryByRoute[route.ID]
		out.Routes = append(out.Routes, RouteSnapshot{
			Route:     route,
			MaxAssets: CloneBig(item.MaxAssets), MaxRate: CloneBig(item.MaxRate),
			AcquireBalance: CloneBig(adapterState.Acquire[route.TokenIn]),
		})
	}
	return out, nil
}

// ReadFillQuotes reads amount-specific quotes for routes of tokenIn; each route left out goes to trace.
func (r *Reader) ReadFillQuotes(
	ctx context.Context,
	routes []Route,
	tokenIn common.Address,
	amountIn *big.Int,
	trace ...DropTrace,
) ([]FillQuote, error) {
	return r.readFillQuotes(ctx, routes, tokenIn, amountIn, false, dropSink(ctx, trace))
}

// ReadPricedFillQuotes reads the oracle price in the same multicall as the
// amount-specific payout so a fill can also price smaller legs exactly.
func (r *Reader) ReadPricedFillQuotes(
	ctx context.Context, routes []Route, tokenIn common.Address, amountIn *big.Int, trace ...DropTrace,
) ([]FillQuote, error) {
	return r.readFillQuotes(ctx, routes, tokenIn, amountIn, true, dropSink(ctx, trace))
}

func (r *Reader) readFillQuotes(
	ctx context.Context, routes []Route, tokenIn common.Address, amountIn *big.Int, withPrice bool, drop DropTrace,
) ([]FillQuote, error) {
	if tokenIn == (common.Address{}) || amountIn == nil || amountIn.Sign() <= 0 {
		return nil, nil
	}
	candidates := make([]Route, 0, len(routes))
	for _, route := range compactRoutes(routes) {
		if route.TokenIn == tokenIn {
			candidates = append(candidates, route)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	readsPerRoute := fillReadsPerRoute
	if withPrice {
		readsPerRoute++
	}
	calls := make([]chain.Call, 0, len(candidates)*readsPerRoute)
	for _, route := range candidates {
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
			r.maxAssetsCall(route.Adapter, route.TokenIn),
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetAmountOut(route.TokenIn, amountIn)},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMinDiscount(route.TokenIn)},
		)
		if withPrice {
			probe, ok := OraclePriceProbe(route.TokenInDecimals, route.TokenOutDecimals)
			if !ok {
				probe = amountIn // Keep the layout for routes whose amount quote already identifies the price.
			}
			calls = append(calls, chain.Call{
				Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetAmountOut(route.TokenIn, probe),
			})
		}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("liquidlane: fill multicall: got %d results, want %d", len(res), len(calls))
	}
	out := make([]FillQuote, 0, len(candidates))
	for i, route := range candidates {
		quote, reason := fillQuote(route, res[i*readsPerRoute:(i+1)*readsPerRoute], amountIn, withPrice)
		if reason != "" {
			drop(Drop{Adapter: route.Adapter, TokenIn: route.TokenIn, Reason: reason})
			continue
		}
		out = append(out, quote)
	}
	return out, nil
}

// fillQuote decodes one route's fill reads, or names the check that drops the route.
func fillQuote(route Route, res []chain.CallResult, amountIn *big.Int, withPrice bool) (FillQuote, string) {
	if reason := pauseReason(res[0]); reason != "" {
		return FillQuote{}, reason
	}
	maxAssets, reason := decodeCall("getMaxAssets", res[1], llAdapter.UnpackGetMaxAssets)
	if reason != "" {
		return FillQuote{}, reason
	}
	grossAmountOut, reason := decodeCall("getAmountOut", res[2], llAdapter.UnpackGetAmountOut)
	if reason != "" {
		return FillQuote{}, reason
	}
	discount, reason := decodeCall("minDiscount", res[3], llAdapter.UnpackMinDiscount)
	if reason != "" {
		return FillQuote{}, reason
	}
	if maxAssets.Sign() <= 0 {
		return FillQuote{}, "getMaxAssets " + maxAssets.String() + " is not positive"
	}
	if grossAmountOut.Sign() <= 0 {
		return FillQuote{}, "getAmountOut " + grossAmountOut.String() + " for amountIn " + amountIn.String() + " is not positive"
	}
	if reason = minDiscountReason(discount); reason != "" {
		return FillQuote{}, reason
	}
	maxAmountOut := AmountOutAfterDiscount(grossAmountOut, discount)
	if maxAmountOut.Sign() <= 0 {
		return FillQuote{}, fmt.Sprintf("amount out %s after minDiscount %s is not positive", maxAmountOut, discount)
	}
	maxRate := RateForAmountOut(maxAmountOut, amountIn, route.TokenInDecimals, route.TokenOutDecimals)
	inventory := DirectInventory(route, maxAssets, maxRate)
	inventory.AdapterMinDiscount = CloneBig(discount)
	if withPrice {
		inventory.Price, reason = fillQuotePrice(route, res[fillReadsPerRoute], amountIn, grossAmountOut)
		if reason != "" {
			return FillQuote{}, reason
		}
	}
	return FillQuote{
		Inventory:      inventory,
		AmountIn:       CloneBig(amountIn),
		GrossAmountOut: CloneBig(grossAmountOut),
		MaxAmountOut:   maxAmountOut,
		MinDiscount:    CloneBig(discount),
	}, ""
}

func fillQuotePrice(route Route, result chain.CallResult, amountIn, gross *big.Int) (*big.Int, string) {
	if _, ok := OraclePriceProbe(route.TokenInDecimals, route.TokenOutDecimals); !ok {
		// One price unit changes the gross by more than one output unit, so the
		// full amount quote uniquely identifies the integer price.
		return ImpliedPrice(amountIn, gross, route.TokenInDecimals, route.TokenOutDecimals), ""
	}
	price, reason := decodeCall("getAmountOut(price probe)", result, llAdapter.UnpackGetAmountOut)
	if reason != "" {
		return nil, reason
	}
	if implied := AmountOutForRate(amountIn, price, route.TokenInDecimals, route.TokenOutDecimals); implied.Cmp(gross) != 0 {
		return nil, fmt.Sprintf("oracle price %s pays %s for amountIn %s, getAmountOut pays %s", price, implied, amountIn, gross)
	}
	return price, ""
}

// FilterAuthorized keeps inventory whose adapter authorizes filler; each item left out goes to trace.
func (r *Reader) FilterAuthorized(
	ctx context.Context, inv []Inventory, filler common.Address, trace ...DropTrace,
) ([]Inventory, error) {
	inv = compactInventory(inv)
	if len(inv) == 0 {
		return nil, nil
	}
	adapters := make([]common.Address, 0, len(inv))
	for _, item := range inv {
		adapters = append(adapters, item.Adapter)
	}
	unauthorized, err := r.unauthorizedAdapters(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	drop := dropSink(ctx, trace)
	out := make([]Inventory, 0, len(inv))
	for _, item := range inv {
		if reason, denied := unauthorized[item.Adapter]; denied {
			drop(Drop{Adapter: item.Adapter, TokenIn: item.TokenIn, Reason: reason})
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// FilterAuthorizedRoutes filters by the adapter-wide marketMaker/owner/isFiller authorization and
// preserves every non-zero-adapter input route. It intentionally accepts adapter-only projections so
// startup validation does not depend on whether a solver has already resolved token-pair metadata.
// Each route left out goes to trace.
func (r *Reader) FilterAuthorizedRoutes(
	ctx context.Context, routes []Route, filler common.Address, trace ...DropTrace,
) ([]Route, error) {
	if len(routes) == 0 {
		return nil, nil
	}
	adapters := make([]common.Address, 0, len(routes))
	for _, route := range routes {
		if route.Adapter != (common.Address{}) {
			adapters = append(adapters, route.Adapter)
		}
	}
	if len(adapters) == 0 {
		return nil, nil
	}
	unauthorized, err := r.unauthorizedAdapters(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	drop := dropSink(ctx, trace)
	out := make([]Route, 0, len(routes))
	for _, route := range routes {
		if route.Adapter == (common.Address{}) {
			continue
		}
		if reason, denied := unauthorized[route.Adapter]; denied {
			drop(Drop{Adapter: route.Adapter, TokenIn: route.TokenIn, Reason: reason})
			continue
		}
		out = append(out, route)
	}
	return out, nil
}

// unauthorizedAdapters maps each adapter that does not authorize filler to the reason.
func (r *Reader) unauthorizedAdapters(
	ctx context.Context,
	adapters []common.Address,
	filler common.Address,
) (map[common.Address]string, error) {
	auth, failures, err := r.readAuth(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	byAdapter := make(map[common.Address]Auth, len(auth))
	for _, item := range auth {
		byAdapter[item.Adapter] = item
	}
	for _, adapter := range adapters {
		if _, failed := failures[adapter]; failed {
			continue
		}
		item, read := byAdapter[adapter]
		switch {
		case !read: // readAuth skips every read only for a zero filler.
			failures[adapter] = "filler is the zero address"
		case !item.Authorized:
			failures[adapter] = fmt.Sprintf("filler %s is not marketMaker %s, owner %s, or a delegated filler",
				filler.Hex(), item.MarketMaker.Hex(), item.Owner.Hex())
		}
	}
	return failures, nil
}

// ReadAuth reads filler authorization per adapter. An adapter whose roles cannot be read is left
// out, and one whose isFiller read fails stays unauthorized; both go to trace.
func (r *Reader) ReadAuth(
	ctx context.Context, adapters []common.Address, filler common.Address, trace ...DropTrace,
) ([]Auth, error) {
	auth, failures, err := r.readAuth(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	if len(failures) > 0 {
		drop := dropSink(ctx, trace)
		for _, adapter := range dedupeAddresses(adapters) {
			if reason, failed := failures[adapter]; failed {
				drop(Drop{Adapter: adapter, Reason: reason})
			}
		}
	}
	return auth, nil
}

// readAuth also returns why each failed adapter read failed.
func (r *Reader) readAuth(
	ctx context.Context, adapters []common.Address, filler common.Address,
) ([]Auth, map[common.Address]string, error) {
	adapters = dedupeAddresses(adapters)
	failures := make(map[common.Address]string)
	if len(adapters) == 0 || filler == (common.Address{}) {
		return nil, failures, nil
	}
	calls := make([]chain.Call, 0, len(adapters)*2)
	for _, adapterAddr := range adapters {
		calls = append(calls,
			chain.Call{Target: adapterAddr, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
			chain.Call{Target: adapterAddr, AllowFailure: true, Data: llAdapter.PackOwner()},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, nil, err
	}
	if len(res) != len(calls) {
		return nil, nil, errors.Errorf("liquidlane: authorization multicall: got %d results, want %d", len(res), len(calls))
	}

	auths := make([]Auth, len(adapters))
	resolved := make([]bool, len(adapters))
	var delegatedChecks []int
	for i := range adapters {
		marketMaker, reason := decodeCall("marketMaker", res[i*2], llAdapter.UnpackMarketMaker)
		if reason != "" {
			failures[adapters[i]] = "authorization: " + reason
			continue
		}
		owner, reason := decodeCall("owner", res[i*2+1], llAdapter.UnpackOwner)
		if reason != "" {
			failures[adapters[i]] = "authorization: " + reason
			continue
		}
		auths[i] = Auth{Adapter: adapters[i], MarketMaker: marketMaker, Owner: owner}
		resolved[i] = true
		auths[i].Authorized = marketMaker == filler || owner == filler
		if !auths[i].Authorized {
			delegatedChecks = append(delegatedChecks, i)
		}
	}

	if len(delegatedChecks) > 0 {
		delegationCalls := make([]chain.Call, len(delegatedChecks))
		for j, i := range delegatedChecks {
			// Delegation is keyed by the adapter's exact current marketMaker value; zero is valid.
			delegationCalls[j] = chain.Call{
				Target: adapters[i], AllowFailure: true,
				Data: llAdapter.PackIsFiller(auths[i].MarketMaker, filler),
			}
		}
		delegationResults, err := r.chain.Multicall(ctx, delegationCalls)
		if err != nil {
			return nil, nil, err
		}
		if len(delegationResults) != len(delegationCalls) {
			return nil, nil, errors.Errorf(
				"liquidlane: filler authorization multicall: got %d results, want %d",
				len(delegationResults),
				len(delegationCalls),
			)
		}
		for j, i := range delegatedChecks {
			ok, reason := decodeCall("isFiller", delegationResults[j], llAdapter.UnpackIsFiller)
			if reason != "" {
				failures[adapters[i]] = "authorization: " + reason
				continue
			}
			auths[i].IsFiller = ok
			auths[i].Authorized = ok
		}
	}

	out := make([]Auth, 0, len(auths))
	for i, item := range auths {
		if resolved[i] {
			out = append(out, item)
		}
	}
	return out, failures, nil
}

// pauseReason names why a paused() read blocks the route, or returns "" when it is unpaused.
func pauseReason(result chain.CallResult) string {
	paused, reason := decodeCall("paused", result, llAdapter.UnpackPaused)
	if reason == "" && paused {
		return "adapter is paused"
	}
	return reason
}

func minDiscountReason(discount *big.Int) string {
	if discount == nil || discount.Sign() < 0 || discount.Cmp(big.NewInt(DiscountPrecision)) > 0 {
		return fmt.Sprintf("minDiscount %v is outside [0, %d]", discount, DiscountPrecision)
	}
	return ""
}

func (r *Reader) readTokenCounts(ctx context.Context, adapters []Adapter) ([]int, error) {
	calls := make([]chain.Call, len(adapters))
	for i, a := range adapters {
		calls[i] = chain.Call{Target: a.Adapter, AllowFailure: true, Data: llAdapter.PackGetTokensToRedeemLength()}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("liquidlane: tokensToRedeem length multicall: got %d results, want %d", len(res), len(calls))
	}
	out := make([]int, len(adapters))
	for i, call := range res {
		if !call.Success {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s %s",
				adapters[i].Adapter.Hex(),
				callFailure("getTokensToRedeemLength", call),
			)
		}
		n, unpackErr := llAdapter.UnpackGetTokensToRedeemLength(call.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem length: %w",
				adapters[i].Adapter.Hex(),
				unpackErr,
			)
		}
		if !n.IsInt64() || n.Sign() < 0 {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem length: invalid value %s",
				adapters[i].Adapter.Hex(),
				n,
			)
		}
		if n.Sign() == 0 {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s: tokensToRedeem is empty",
				adapters[i].Adapter.Hex(),
			)
		}
		if n.Cmp(big.NewInt(int64(r.maxTokensPerAdapter))) > 0 {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem length %s exceeds cap %d",
				adapters[i].Adapter.Hex(),
				n,
				r.maxTokensPerAdapter,
			)
		}
		out[i] = int(n.Int64())
	}
	return out, nil
}

func (r *Reader) resolveRouteForToken(ctx context.Context, adapter Adapter, tokenIn common.Address) (Route, error) {
	if tokenIn == (common.Address{}) {
		return Route{}, errors.Errorf("liquidlane: resolve adapter %s tokenIn: zero address", adapter.Adapter.Hex())
	}
	tokenInDecimals, err := r.dec.Get(ctx, tokenIn)
	if err != nil {
		return Route{}, errors.Errorf(
			"liquidlane: resolve adapter %s tokenIn %s decimals: %w",
			adapter.Adapter.Hex(),
			tokenIn.Hex(),
			err,
		)
	}
	return NewRoute(
		r.chainID,
		adapter.Adapter,
		adapter.Vault,
		tokenIn,
		adapter.TokenOut,
		tokenInDecimals,
		adapter.TokenOutDecimals,
	), nil
}

func dedupeAddresses(in []common.Address) []common.Address {
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

func dedupeAdapters(in []Adapter) []Adapter {
	seen := make(map[common.Address]bool, len(in))
	out := make([]Adapter, 0, len(in))
	for _, a := range in {
		if seen[a.Adapter] {
			continue
		}
		seen[a.Adapter] = true
		out = append(out, a)
	}
	return out
}

func compactRoutes(in []Route) []Route {
	seen := make(map[RouteID]bool, len(in))
	out := make([]Route, 0, len(in))
	for _, route := range in {
		if route.Adapter == (common.Address{}) || route.TokenIn == (common.Address{}) || route.TokenOut == (common.Address{}) {
			continue
		}
		if seen[route.ID] {
			continue
		}
		seen[route.ID] = true
		out = append(out, route)
	}
	return out
}

func compactInventory(in []Inventory) []Inventory {
	seen := make(map[CandidateID]bool, len(in))
	out := make([]Inventory, 0, len(in))
	for _, item := range in {
		if item.Adapter == (common.Address{}) || item.TokenIn == (common.Address{}) || item.TokenOut == (common.Address{}) {
			continue
		}
		if item.MaxAssets == nil || item.MaxAssets.Sign() <= 0 {
			continue
		}
		id := NewCandidateID(item.Route, item.DiscountID)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, item)
	}
	return out
}
