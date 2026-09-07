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

// ReadGasSnapshot returns the latest adapter-local acquire balances and shared vault liquidity needed
// to predict LiquidLane swap gas. Partially unread state remains absent and is priced as RouteUnknown.
func (r *Reader) ReadGasSnapshot(ctx context.Context, routes []Route) (*liquidlanegas.Snapshot, error) {
	routes = compactRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	var adapters []Route
	var vaults []common.Address
	seenAdapters, seenVaults := make(map[common.Address]bool), make(map[common.Address]bool)
	for _, route := range routes {
		if !seenAdapters[route.Adapter] {
			adapters = append(adapters, route)
			seenAdapters[route.Adapter] = true
		}
		if !seenVaults[route.Vault] {
			vaults = append(vaults, route.Vault)
			seenVaults[route.Vault] = true
		}
	}
	calls := make([]chain.Call, 0, 2*(len(adapters)+len(vaults)))
	for _, route := range adapters {
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackOwner()},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMarketMaker()})
	}
	for _, vault := range vaults {
		calls = append(calls,
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackFreeAssets()},
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackWithdrawable()})
	}
	results, err := r.checkedMulticall(ctx, "gas state head", calls)
	if err != nil {
		return nil, err
	}
	out := &liquidlanegas.Snapshot{Adapters: make(map[common.Address]*liquidlanegas.AdapterState), Vaults: make(map[common.Address]*liquidlanegas.VaultState)}
	holders := make(map[common.Address][]common.Address)
	for index, route := range adapters {
		ownerResult, makerResult := results[2*index], results[2*index+1]
		owner, ownerErr := chain.Decode(ownerResult, llAdapter.UnpackOwner)
		maker, makerErr := chain.Decode(makerResult, llAdapter.UnpackMarketMaker)
		if ownerErr != nil || makerErr != nil {
			continue
		}
		holders[route.Adapter] = []common.Address{owner}
		// The zero market-maker key is valid; deduplicate equality, never nonzero-ness.
		if maker != owner {
			holders[route.Adapter] = append(holders[route.Adapter], maker)
		}
		out.Adapters[route.Adapter] = &liquidlanegas.AdapterState{Vault: route.Vault, Acquire: make(map[common.Address]*big.Int)}
	}
	for index, vault := range vaults {
		base := 2 * (len(adapters) + index)
		freeResult, withdrawResult := results[base], results[base+1]
		free, freeErr := chain.Decode(freeResult, vaultV2b.UnpackFreeAssets)
		withdrawable, withdrawErr := chain.Decode(withdrawResult, vaultV2b.UnpackWithdrawable)
		if freeErr != nil || withdrawErr != nil || free == nil || withdrawable == nil {
			continue
		}
		out.Vaults[vault] = &liquidlanegas.VaultState{FreeAssets: free, Withdrawable: withdrawable}
	}
	calls = nil
	var owners []Route
	for _, route := range routes {
		for _, holder := range holders[route.Adapter] {
			calls = append(calls, chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(route.TokenIn, holder)})
			owners = append(owners, route)
		}
	}
	if len(calls) == 0 {
		return out, nil
	}
	results, err = r.checkedMulticall(ctx, "gas state acquire", calls)
	if err != nil {
		return nil, err
	}
	for index, result := range results {
		amount, err := chain.Decode(result, llAdapter.UnpackAcquireBalance)
		if err != nil || amount == nil || amount.Sign() < 0 {
			continue
		}
		route := owners[index]
		balances := out.Adapters[route.Adapter].Acquire
		if balances[route.TokenIn] == nil {
			balances[route.TokenIn] = new(big.Int)
		}
		balances[route.TokenIn].Add(balances[route.TokenIn], amount)
	}
	return out, nil
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
	auth, err := r.ReadAuth(ctx, addresses, filler)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	if len(auth) != 1 || auth[0].Adapter != address {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter authorization unresolved")
	}
	gas, err := r.ReadGasSnapshot(ctx, routes)
	if err != nil {
		return AdapterSnapshot{}, err
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
	addresses = dedupeAddresses(addresses)
	if filler == (common.Address{}) || len(addresses) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, 0, 2*len(addresses))
	for _, address := range addresses {
		calls = append(calls, chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
			chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackOwner()})
	}
	rows, err := r.checkedMulticall(ctx, "authorization", calls)
	if err != nil {
		return nil, err
	}
	authorized := make([]Auth, 0, len(addresses))
	var delegated []int
	var checks []chain.Call
	for index, address := range addresses {
		makerResult, ownerResult := rows[2*index], rows[2*index+1]
		maker, makerErr := chain.Decode(makerResult, llAdapter.UnpackMarketMaker)
		owner, ownerErr := chain.Decode(ownerResult, llAdapter.UnpackOwner)
		if makerErr != nil || ownerErr != nil {
			continue
		}
		entry := Auth{Adapter: address, MarketMaker: maker, Owner: owner, Authorized: filler == maker || filler == owner}
		if !entry.Authorized {
			delegated = append(delegated, len(authorized))
			// Zero marketMaker remains a valid delegation key; never replace it with owner.
			checks = append(checks, chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackIsFiller(maker, filler)})
		}
		authorized = append(authorized, entry)
	}
	if len(checks) == 0 {
		return authorized, nil
	}
	rows, err = r.checkedMulticall(ctx, "filler authorization", checks)
	if err != nil {
		return nil, err
	}
	for index, row := range rows {
		allowed, err := chain.Decode(row, llAdapter.UnpackIsFiller)
		if err == nil {
			entry := &authorized[delegated[index]]
			entry.Authorized, entry.IsFiller = allowed, allowed
		}
	}
	return authorized, nil
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
