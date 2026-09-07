package liquidlane

import (
	"context"
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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
)

var (
	llAdapter = adapter.NewLiquidLaneAdapter()
	erc4626b  = erc4626.NewIERC4626()
	vaultV2b  = vaultv2.NewIVaultV2()
	lensB     = lens.NewFrontendLiquidityLens()
)

type Reader struct {
	chain chain.Multicaller
	log   logr.Logger
	dec   decimalsReader

	chainID int64
	// lens is the FrontendLiquidityLens address. When non-zero, swappable headroom is read from the lens's
	// cross-adapter deallocation-cascade estimate instead of the adapter's own getMaxAssets(tokenToRedeem);
	// zero falls back to the adapter getter.
	lens common.Address
}

type decimalsReader interface {
	Get(ctx context.Context, token common.Address) (int, error)
}

func NewReader(c *chain.Client, log logr.Logger, liquidityLens common.Address) *Reader {
	return &Reader{
		chain:   c,
		log:     log,
		dec:     chain.NewDecimals(c),
		chainID: c.ChainID().Int64(),
		lens:    liquidityLens,
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

func (r *Reader) ResolveAdapters(ctx context.Context, adapters []common.Address) ([]Adapter, error) {
	adapters = dedupeAddresses(adapters)
	if len(adapters) == 0 {
		return nil, nil
	}

	vaultCalls := make([]chain.Call, len(adapters))
	for i, a := range adapters {
		vaultCalls[i] = chain.Call{Target: a, AllowFailure: true, Data: llAdapter.PackVault()}
	}
	vaultResults, err := r.checkedMulticall(ctx, "vault", vaultCalls)
	if err != nil {
		return nil, err
	}

	out := make([]Adapter, len(adapters))
	assetCalls := make([]chain.Call, len(adapters))
	for i := range adapters {
		out[i].Adapter = adapters[i]
		vault, unpackErr := chain.Decode(vaultResults[i], llAdapter.UnpackVault)
		if unpackErr != nil {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: %w", adapters[i].Hex(), unpackErr)
		}
		if vault == (common.Address{}) {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: zero address", adapters[i].Hex())
		}
		out[i].Vault = vault
		assetCalls[i] = chain.Call{Target: out[i].Vault, AllowFailure: true, Data: erc4626b.PackAsset()}
	}
	assetResults, err := r.checkedMulticall(ctx, "asset", assetCalls)
	if err != nil {
		return nil, err
	}

	for i := range out {
		asset, unpackErr := chain.Decode(assetResults[i], erc4626b.UnpackAsset)
		if unpackErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s vault %s asset: %w",
				out[i].Adapter.Hex(),
				out[i].Vault.Hex(),
				unpackErr,
			)
		}
		if asset == (common.Address{}) {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s vault %s asset: zero address",
				out[i].Adapter.Hex(),
				out[i].Vault.Hex(),
			)
		}
		out[i].TokenOut = asset
		decimals, decimalsErr := r.dec.Get(ctx, asset)
		if decimalsErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokenOut %s decimals: %w",
				out[i].Adapter.Hex(),
				asset.Hex(),
				decimalsErr,
			)
		}
		out[i].TokenOutDecimals = decimals
	}
	return out, nil
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
	res, err := r.checkedMulticall(ctx, "tokensToRedeem", tokenCalls)
	if err != nil {
		return nil, err
	}

	routes := make([]Route, 0, len(res))
	for i, call := range res {
		req := reqs[i]
		resolvedAdapter := resolved[req.adapterIndex]
		tokenIn, unpackErr := chain.Decode(call, llAdapter.UnpackTokensToRedeem)
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

func (r *Reader) RoutesForToken(ctx context.Context, adapters []Adapter, tokenIn common.Address) []Route {
	out := make([]Route, 0, len(adapters))
	for _, a := range dedupeAdapters(adapters) {
		route, err := r.resolveRouteForToken(ctx, a, tokenIn)
		if err != nil {
			r.log.Error(err, "liquidlane: route unresolved",
				"adapter", a.Adapter.Hex(),
				"tokenIn", tokenIn.Hex(),
			)
			continue
		}
		out = append(out, route)
	}
	return compactRoutes(out)
}

// readPaused returns current pause state for each successfully decoded adapter.
func (r *Reader) readPaused(ctx context.Context, adapters []common.Address) (map[common.Address]bool, error) {
	adapters = dedupeAddresses(adapters)
	if len(adapters) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, len(adapters))
	for i, address := range adapters {
		calls[i] = chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackPaused()}
	}
	results, err := r.checkedMulticall(ctx, "paused", calls)
	if err != nil {
		return nil, err
	}
	out := make(map[common.Address]bool, len(adapters))
	for i, result := range results {
		paused, unpackErr := chain.Decode(result, llAdapter.UnpackPaused)
		if unpackErr == nil {
			out[adapters[i]] = paused
		}
	}
	return out, nil
}

func (r *Reader) ReadInventory(ctx context.Context, routes []Route) ([]Inventory, error) {
	return r.readInventory(ctx, routes, false)
}

func (r *Reader) readInventory(ctx context.Context, routes []Route, keepZero bool) ([]Inventory, error) {
	rows, err := r.readLiquidity(ctx, compactRoutes(routes), nil)
	if err != nil {
		return nil, err
	}
	out := make([]Inventory, 0, len(rows))
	for _, row := range rows {
		if !keepZero && (row.capacity.Sign() <= 0 || row.price.Sign() <= 0) {
			continue
		}
		out = append(out, Inventory{Route: row.route, MaxAssets: row.capacity, MaxRate: row.price, AdapterMinDiscount: row.discount})
	}
	return out, nil
}

type routeLiquidity struct {
	route                     Route
	capacity, discount, price *big.Int
}

// Inventory and execution use the same pause, capacity and discount observation.
// Only the pricing call differs: a fixed rate for inventory, an amount-specific quote for execution.
func (r *Reader) readLiquidity(ctx context.Context, routes []Route, amountIn *big.Int) ([]routeLiquidity, error) {
	if len(routes) == 0 {
		return nil, nil
	}
	const width = 4
	calls := make([]chain.Call, 0, len(routes)*width)
	for _, route := range routes {
		pricing := llAdapter.PackGetMaxRate(route.TokenIn)
		if amountIn != nil {
			pricing = llAdapter.PackGetAmountOut(route.TokenIn, amountIn)
		}
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
			r.maxAssetsCall(route.Adapter, route.TokenIn),
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: pricing},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMinDiscount(route.TokenIn)})
	}
	results, err := r.checkedMulticall(ctx, "liquidity", calls)
	if err != nil {
		return nil, err
	}
	out := make([]routeLiquidity, 0, len(routes))
	for index, route := range routes {
		row := results[index*width : (index+1)*width]
		if !unpaused(row[0]) {
			continue
		}
		capacity, capErr := chain.Decode(row[1], llAdapter.UnpackGetMaxAssets)
		discount, discountErr := chain.Decode(row[3], llAdapter.UnpackMinDiscount)
		if capErr != nil || discountErr != nil || capacity == nil || discount == nil ||
			capacity.Sign() < 0 || discount.Sign() < 0 || discount.Cmp(big.NewInt(DiscountPrecision)) > 0 {
			continue
		}
		item := routeLiquidity{route: route, capacity: capacity, discount: discount}
		var price *big.Int
		var priceErr error
		if amountIn == nil {
			price, priceErr = chain.Decode(row[2], llAdapter.UnpackGetMaxRate)
		} else {
			price, priceErr = chain.Decode(row[2], llAdapter.UnpackGetAmountOut)
		}
		if priceErr != nil || price == nil || price.Sign() < 0 {
			continue
		}
		item.price = price
		out = append(out, item)
	}
	return out, nil
}

// ReadAdapterState shares one owner/market-maker observation between direct authorization
// and gas accounting. Only addresses need authorization; gasRoutes may cover a wider universe.
// Failed rows remain absent, and nil gasRoutes disables gas reads entirely. No state is cached.
func (r *Reader) ReadAdapterState(ctx context.Context, addresses []common.Address, filler common.Address,
	gasRoutes []Route,
) ([]Auth, *liquidlanegas.Snapshot, error) {
	addresses = dedupeAddresses(addresses)
	if filler == (common.Address{}) {
		addresses = nil
	}
	gasRoutes = compactRoutes(gasRoutes)
	adapters := append([]common.Address(nil), addresses...)
	var vaults []common.Address
	for _, route := range gasRoutes {
		adapters = append(adapters, route.Adapter)
		vaults = append(vaults, route.Vault)
	}
	adapters, vaults = dedupeAddresses(adapters), dedupeAddresses(vaults)
	if len(adapters) == 0 {
		return nil, nil, nil
	}
	calls := make([]chain.Call, 0, 2*(len(adapters)+len(vaults)))
	for _, address := range adapters {
		calls = append(calls,
			chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
			chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackOwner()})
	}
	for _, vault := range vaults {
		calls = append(calls,
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackFreeAssets()},
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackWithdrawable()})
	}
	results, err := r.checkedMulticall(ctx, "adapter state", calls)
	if err != nil {
		return nil, nil, err
	}
	roles := make(map[common.Address]Auth, len(adapters))
	for i, address := range adapters {
		maker, makerErr := chain.Decode(results[2*i], llAdapter.UnpackMarketMaker)
		owner, ownerErr := chain.Decode(results[2*i+1], llAdapter.UnpackOwner)
		if makerErr == nil && ownerErr == nil {
			roles[address] = Auth{Adapter: address, MarketMaker: maker, Owner: owner, Authorized: filler == maker || filler == owner}
		}
	}
	var gas *liquidlanegas.Snapshot
	if len(gasRoutes) > 0 {
		gas = &liquidlanegas.Snapshot{Adapters: make(map[common.Address]*liquidlanegas.AdapterState), Vaults: make(map[common.Address]*liquidlanegas.VaultState)}
		for i, vault := range vaults {
			offset := 2 * (len(adapters) + i)
			free, freeErr := chain.Decode(results[offset], vaultV2b.UnpackFreeAssets)
			withdrawable, withdrawErr := chain.Decode(results[offset+1], vaultV2b.UnpackWithdrawable)
			if freeErr == nil && withdrawErr == nil && free != nil && withdrawable != nil {
				gas.Vaults[vault] = &liquidlanegas.VaultState{FreeAssets: free, Withdrawable: withdrawable}
			}
		}
	}
	calls = nil
	var auth []Auth
	var delegated []int
	for _, address := range addresses {
		role, ok := roles[address]
		if !ok {
			continue
		}
		if !role.Authorized {
			delegated = append(delegated, len(auth))
			// Zero marketMaker remains a valid delegation key; never replace it with owner.
			calls = append(calls, chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackIsFiller(role.MarketMaker, filler)})
		}
		auth = append(auth, role)
	}
	var owners []Route
	for _, route := range gasRoutes {
		role, ok := roles[route.Adapter]
		if !ok {
			continue
		}
		if gas.Adapters[route.Adapter] == nil {
			gas.Adapters[route.Adapter] = &liquidlanegas.AdapterState{Vault: route.Vault, Acquire: make(map[common.Address]*big.Int)}
		}
		// Equality is the only duplicate case: the zero market-maker key is a valid holder.
		for _, holder := range dedupeHolders(role.Owner, role.MarketMaker) {
			calls = append(calls, chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(route.TokenIn, holder)})
			owners = append(owners, route)
		}
	}
	if len(calls) == 0 {
		return auth, gas, nil
	}
	results, err = r.checkedMulticall(ctx, "adapter access and acquire", calls)
	if err != nil {
		return nil, nil, err
	}
	for i, index := range delegated {
		allowed, err := chain.Decode(results[i], llAdapter.UnpackIsFiller)
		if err == nil {
			auth[index].Authorized, auth[index].IsFiller = allowed, allowed
		}
	}
	for i, route := range owners {
		amount, err := chain.Decode(results[len(delegated)+i], llAdapter.UnpackAcquireBalance)
		if err != nil || amount == nil || amount.Sign() < 0 {
			continue
		}
		balances := gas.Adapters[route.Adapter].Acquire
		if balances[route.TokenIn] == nil {
			balances[route.TokenIn] = new(big.Int)
		}
		balances[route.TokenIn].Add(balances[route.TokenIn], amount)
	}
	return auth, gas, nil
}

func dedupeHolders(owner, maker common.Address) []common.Address {
	if owner == maker {
		return []common.Address{owner}
	}
	return []common.Address{owner, maker}
}

func (r *Reader) checkedMulticall(ctx context.Context, operation string, calls []chain.Call) ([]chain.CallResult, error) {
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("liquidlane: %s multicall: got %d results, want %d", operation, len(results), len(calls))
	}
	return results, nil
}

// ReadAdapterSnapshot reads one complete LiquidLane adapter view for solvers that consume all routes.
func (r *Reader) ReadAdapterSnapshot(ctx context.Context, address, filler common.Address) (AdapterSnapshot, error) {
	addresses := []common.Address{address}
	routes, err := r.ResolveRoutes(ctx, addresses)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	if len(routes) == 0 {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter has no resolved routes")
	}
	paused, err := r.readPaused(ctx, addresses)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	isPaused, known := paused[address]
	if !known {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter pause state unresolved")
	}
	auth, gas, err := r.ReadAdapterState(ctx, addresses, filler, routes)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	if len(auth) != 1 || auth[0].Adapter != address {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter authorization unresolved")
	}
	adapter, vault := gas.Adapters[address], gas.Vaults[routes[0].Vault]
	if adapter == nil || vault == nil {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter liquidity state unresolved")
	}
	inventory := make(map[RouteID]Inventory)
	if !isPaused {
		items, err := r.readInventory(ctx, routes, true)
		if err != nil {
			return AdapterSnapshot{}, err
		}
		for _, item := range items {
			inventory[item.ID] = item
		}
	}
	first := routes[0]
	snapshot := AdapterSnapshot{Adapter: Adapter{Adapter: first.Adapter, Vault: first.Vault, TokenOut: first.TokenOut, TokenOutDecimals: first.TokenOutDecimals},
		Paused: isPaused, Authorized: auth[0].Authorized, FreeAssets: bigmath.Clone(vault.FreeAssets), Withdrawable: bigmath.Clone(vault.Withdrawable),
		Routes: make([]RouteSnapshot, len(routes))}
	for index, route := range routes {
		row := RouteSnapshot{Route: route, AcquireBalance: bigmath.Clone(adapter.Acquire[route.TokenIn]), MaxAssets: new(big.Int), MaxRate: new(big.Int)}
		if !isPaused {
			item, present := inventory[route.ID]
			if !present {
				return AdapterSnapshot{}, errors.New("liquidlane: adapter inventory unresolved")
			}
			row.MaxAssets, row.MaxRate = bigmath.Clone(item.MaxAssets), bigmath.Clone(item.MaxRate)
		}
		snapshot.Routes[index] = row
	}
	return snapshot, nil
}

func (r *Reader) ReadFillQuotes(ctx context.Context, routes []Route, tokenIn common.Address, amountIn *big.Int) ([]FillQuote, error) {
	if tokenIn == (common.Address{}) || amountIn == nil || amountIn.Sign() <= 0 {
		return nil, nil
	}
	var candidates []Route
	for _, route := range compactRoutes(routes) {
		if route.TokenIn == tokenIn {
			candidates = append(candidates, route)
		}
	}
	rows, err := r.readLiquidity(ctx, candidates, amountIn)
	if err != nil {
		return nil, err
	}
	out := make([]FillQuote, 0, len(rows))
	for _, row := range rows {
		if row.capacity.Sign() <= 0 {
			continue
		}
		amountOut := AmountOutAfterDiscount(row.price, row.discount)
		if amountOut.Sign() <= 0 {
			continue
		}
		rate := RateForAmountOut(amountOut, amountIn, row.route.TokenInDecimals, row.route.TokenOutDecimals)
		inventory := Inventory{Route: row.route, MaxAssets: row.capacity, MaxRate: rate, AdapterMinDiscount: bigmath.Clone(row.discount)}
		out = append(out, FillQuote{Inventory: inventory, AmountIn: bigmath.Clone(amountIn),
			GrossAmountOut: row.price, MaxAmountOut: amountOut, MinDiscount: row.discount})
	}
	return out, nil
}

func (r *Reader) FilterAuthorized(ctx context.Context, inventory []Inventory, filler common.Address) ([]Inventory, error) {
	return filterAuthorized(ctx, r, compactInventory(inventory), filler, func(item Inventory) common.Address { return item.Adapter })
}

// FilterAuthorizedRoutes filters by the adapter-wide marketMaker/owner/isFiller authorization and
// preserves every non-zero-adapter input route. It intentionally accepts adapter-only projections so
// startup validation does not depend on whether a solver has already resolved token-pair metadata.
func (r *Reader) FilterAuthorizedRoutes(ctx context.Context, routes []Route, filler common.Address) ([]Route, error) {
	return filterAuthorized(ctx, r, routes, filler, func(route Route) common.Address { return route.Adapter })
}

func filterAuthorized[T any](ctx context.Context, reader *Reader, values []T, filler common.Address, adapterOf func(T) common.Address) ([]T, error) {
	addresses := make([]common.Address, 0, len(values))
	for _, value := range values {
		if address := adapterOf(value); address != (common.Address{}) {
			addresses = append(addresses, address)
		}
	}
	if len(addresses) == 0 {
		return nil, nil
	}
	authorized, err := reader.authorizedAdapters(ctx, addresses, filler)
	if err != nil {
		return nil, err
	}
	selected := make([]T, 0, len(values))
	for _, value := range values {
		if authorized[adapterOf(value)] {
			selected = append(selected, value)
		}
	}
	return selected, nil
}

func (r *Reader) authorizedAdapters(
	ctx context.Context,
	adapters []common.Address,
	filler common.Address,
) (map[common.Address]bool, error) {
	auth, err := r.ReadAuth(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	authorized := make(map[common.Address]bool, len(auth))
	for _, item := range auth {
		authorized[item.Adapter] = item.Authorized
	}
	return authorized, nil
}

func (r *Reader) ReadAuth(ctx context.Context, addresses []common.Address, filler common.Address) ([]Auth, error) {
	auth, _, err := r.ReadAdapterState(ctx, addresses, filler, nil)
	return auth, err
}

func unpaused(result chain.CallResult) bool {
	paused, err := chain.Decode(result, llAdapter.UnpackPaused)
	return err == nil && !paused
}

func (r *Reader) readTokenCounts(ctx context.Context, adapters []Adapter) ([]int, error) {
	calls := make([]chain.Call, len(adapters))
	for i, a := range adapters {
		calls[i] = chain.Call{Target: a.Adapter, AllowFailure: true, Data: llAdapter.PackGetTokensToRedeemLength()}
	}
	res, err := r.checkedMulticall(ctx, "tokensToRedeem length", calls)
	if err != nil {
		return nil, err
	}
	out := make([]int, len(adapters))
	for i, call := range res {
		n, unpackErr := chain.Decode(call, llAdapter.UnpackGetTokensToRedeemLength)
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
		if n.Cmp(big.NewInt(int64(DefaultMaxTokensPerAdapter))) > 0 {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem length %s exceeds cap %d",
				adapters[i].Adapter.Hex(),
				n,
				DefaultMaxTokensPerAdapter,
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
