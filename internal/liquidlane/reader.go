package liquidlane

import (
	"context"
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
	inventoryReadsPerRoute     = 4
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

func (r *Reader) ResolveAdapters(ctx context.Context, adapters []common.Address) ([]Adapter, error) {
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
		return nil, errors.Errorf(
			"liquidlane: vault multicall: got %d results, want %d",
			len(vaultResults),
			len(vaultCalls),
		)
	}

	out := make([]Adapter, len(adapters))
	assetCalls := make([]chain.Call, len(adapters))
	for i := range adapters {
		out[i].Adapter = adapters[i]
		if !vaultResults[i].Success {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: call failed", adapters[i].Hex())
		}
		vault, unpackErr := llAdapter.UnpackVault(vaultResults[i].ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: %w", adapters[i].Hex(), unpackErr)
		}
		if vault == (common.Address{}) {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: zero address", adapters[i].Hex())
		}
		out[i].Vault = vault
		assetCalls[i] = chain.Call{Target: out[i].Vault, AllowFailure: true, Data: erc4626b.PackAsset()}
	}
	assetResults, err := r.chain.Multicall(ctx, assetCalls)
	if err != nil {
		return nil, err
	}
	if len(assetResults) != len(assetCalls) {
		return nil, errors.Errorf(
			"liquidlane: asset multicall: got %d results, want %d",
			len(assetResults),
			len(assetCalls),
		)
	}

	for i := range out {
		if !assetResults[i].Success {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s vault %s asset: call failed",
				out[i].Adapter.Hex(),
				out[i].Vault.Hex(),
			)
		}
		asset, unpackErr := erc4626b.UnpackAsset(assetResults[i].ReturnData)
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
				"liquidlane: resolve adapter %s tokensToRedeem[%d]: call failed",
				resolvedAdapter.Adapter.Hex(),
				req.tokenIndex,
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
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("liquidlane: paused multicall: got %d results, want %d", len(results), len(calls))
	}
	out := make(map[common.Address]bool, len(adapters))
	for i, result := range results {
		if !result.Success {
			continue
		}
		paused, unpackErr := llAdapter.UnpackPaused(result.ReturnData)
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
	routes = compactRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, 0, len(routes)*inventoryReadsPerRoute)
	for _, route := range routes {
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
			r.maxAssetsCall(route.Adapter, route.TokenIn),
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetMaxRate(route.TokenIn)},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMinDiscount(route.TokenIn)},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("liquidlane: inventory multicall: got %d results, want %d", len(res), len(calls))
	}
	out := make([]Inventory, 0, len(routes))
	for i, route := range routes {
		base := i * inventoryReadsPerRoute
		paused, maxAssetsRes, maxRateRes, minDiscountRes := res[base], res[base+1], res[base+2], res[base+3]
		if !unpaused(paused) {
			continue
		}
		if !maxAssetsRes.Success || !maxRateRes.Success || !minDiscountRes.Success {
			continue
		}
		maxAssets, aerr := llAdapter.UnpackGetMaxAssets(maxAssetsRes.ReturnData)
		maxRate, rerr := llAdapter.UnpackGetMaxRate(maxRateRes.ReturnData)
		minDiscount, derr := llAdapter.UnpackMinDiscount(minDiscountRes.ReturnData)
		if aerr != nil || rerr != nil || derr != nil || maxAssets == nil || maxRate == nil || minDiscount == nil ||
			minDiscount.Sign() < 0 || minDiscount.Cmp(big.NewInt(DiscountPrecision)) > 0 {
			continue
		}
		if !keepZero && (maxAssets.Sign() <= 0 || maxRate.Sign() <= 0) {
			continue
		}
		inventory := DirectInventory(route, maxAssets, maxRate)
		inventory.AdapterMinDiscount = CloneBig(minDiscount)
		out = append(out, inventory)
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
		ownerRes, makerRes := headResults[base], headResults[base+1]
		if !ownerRes.Success || !makerRes.Success {
			continue
		}
		owner, ownerErr := llAdapter.UnpackOwner(ownerRes.ReturnData)
		marketMaker, makerErr := llAdapter.UnpackMarketMaker(makerRes.ReturnData)
		if ownerErr != nil || makerErr != nil {
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
		freeRes, withdrawableRes := headResults[base], headResults[base+1]
		if !freeRes.Success || !withdrawableRes.Success {
			continue
		}
		freeAssets, freeErr := vaultV2b.UnpackFreeAssets(freeRes.ReturnData)
		withdrawable, withdrawableErr := vaultV2b.UnpackWithdrawable(withdrawableRes.ReturnData)
		if freeErr != nil || withdrawableErr != nil || freeAssets == nil || withdrawable == nil {
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
		result := acquireResults[i]
		if !result.Success {
			continue
		}
		amount, unpackErr := llAdapter.UnpackAcquireBalance(result.ReturnData)
		if unpackErr != nil || amount == nil || amount.Sign() < 0 {
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
	pausedByAdapter, err := r.readPaused(ctx, []common.Address{adapterAddress})
	if err != nil {
		return AdapterSnapshot{}, err
	}
	paused, pausedResolved := pausedByAdapter[adapterAddress]
	if !pausedResolved {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter pause state unresolved")
	}
	auth, err := r.ReadAuth(ctx, []common.Address{adapterAddress}, filler)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	if len(auth) != 1 || auth[0].Adapter != adapterAddress {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter authorization unresolved")
	}
	gasState, err := r.ReadGasSnapshot(ctx, routes)
	if err != nil {
		return AdapterSnapshot{}, err
	}
	adapterState := gasState.Adapters[adapterAddress]
	vaultState := gasState.Vaults[routes[0].Vault]
	if adapterState == nil || vaultState == nil {
		return AdapterSnapshot{}, errors.New("liquidlane: adapter liquidity state unresolved")
	}

	inventoryByRoute := make(map[RouteID]Inventory, len(routes))
	if paused {
		for _, route := range routes {
			inventoryByRoute[route.ID] = DirectInventory(route, new(big.Int), new(big.Int))
		}
	} else {
		inventory, inventoryErr := r.readInventory(ctx, routes, true)
		if inventoryErr != nil {
			return AdapterSnapshot{}, inventoryErr
		}
		for _, item := range inventory {
			inventoryByRoute[item.ID] = item
		}
		if len(inventoryByRoute) != len(routes) {
			return AdapterSnapshot{}, errors.New("liquidlane: adapter inventory unresolved")
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

func (r *Reader) ReadFillQuotes(
	ctx context.Context,
	routes []Route,
	tokenIn common.Address,
	amountIn *big.Int,
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

	calls := make([]chain.Call, 0, len(candidates)*fillReadsPerRoute)
	for _, route := range candidates {
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
			r.maxAssetsCall(route.Adapter, route.TokenIn),
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetAmountOut(route.TokenIn, amountIn)},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMinDiscount(route.TokenIn)},
		)
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
		base := i * fillReadsPerRoute
		paused, maxAssetsRes, amountOutRes, discountRes := res[base], res[base+1], res[base+2], res[base+3]
		if !unpaused(paused) {
			continue
		}
		if !maxAssetsRes.Success || !amountOutRes.Success || !discountRes.Success {
			continue
		}
		maxAssets, aerr := llAdapter.UnpackGetMaxAssets(maxAssetsRes.ReturnData)
		grossAmountOut, oerr := llAdapter.UnpackGetAmountOut(amountOutRes.ReturnData)
		discount, derr := llAdapter.UnpackMinDiscount(discountRes.ReturnData)
		if aerr != nil || oerr != nil || derr != nil || maxAssets.Sign() <= 0 || grossAmountOut.Sign() <= 0 ||
			discount.Sign() < 0 || discount.Cmp(big.NewInt(DiscountPrecision)) > 0 {
			continue
		}
		maxAmountOut := AmountOutAfterDiscount(grossAmountOut, discount)
		if maxAmountOut.Sign() <= 0 {
			continue
		}
		maxRate := RateForAmountOut(maxAmountOut, amountIn, route.TokenInDecimals, route.TokenOutDecimals)
		inventory := DirectInventory(route, maxAssets, maxRate)
		inventory.AdapterMinDiscount = CloneBig(discount)
		out = append(out, FillQuote{
			Inventory:      inventory,
			AmountIn:       CloneBig(amountIn),
			GrossAmountOut: CloneBig(grossAmountOut),
			MaxAmountOut:   maxAmountOut,
			MinDiscount:    CloneBig(discount),
		})
	}
	return out, nil
}

func (r *Reader) FilterAuthorized(ctx context.Context, inv []Inventory, filler common.Address) ([]Inventory, error) {
	inv = compactInventory(inv)
	if len(inv) == 0 {
		return nil, nil
	}
	adapters := make([]common.Address, 0, len(inv))
	for _, item := range inv {
		adapters = append(adapters, item.Adapter)
	}
	authorized, err := r.authorizedAdapters(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	out := make([]Inventory, 0, len(inv))
	for _, item := range inv {
		if authorized[item.Adapter] {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *Reader) FilterAuthorizedRoutes(ctx context.Context, routes []Route, filler common.Address) ([]Route, error) {
	routes = compactRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	adapters := make([]common.Address, 0, len(routes))
	for _, route := range routes {
		adapters = append(adapters, route.Adapter)
	}
	authorized, err := r.authorizedAdapters(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	out := make([]Route, 0, len(routes))
	for _, route := range routes {
		if authorized[route.Adapter] {
			out = append(out, route)
		}
	}
	return out, nil
}

// FilterAuthorizedAdapterAddresses returns the adapter-wide authorization subset without requiring
// token routes to be resolved. Startup checks that are configured by adapter address (not token pair)
// must use this surface rather than manufacturing incomplete Routes, which compactRoutes correctly drops.
func (r *Reader) FilterAuthorizedAdapterAddresses(
	ctx context.Context,
	adapters []common.Address,
	filler common.Address,
) ([]common.Address, error) {
	adapters = dedupeAddresses(adapters)
	if len(adapters) == 0 {
		return nil, nil
	}
	authorized, err := r.authorizedAdapters(ctx, adapters, filler)
	if err != nil {
		return nil, err
	}
	out := make([]common.Address, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter != (common.Address{}) && authorized[adapter] {
			out = append(out, adapter)
		}
	}
	return out, nil
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

func (r *Reader) ReadAuth(ctx context.Context, adapters []common.Address, filler common.Address) ([]Auth, error) {
	adapters = dedupeAddresses(adapters)
	if len(adapters) == 0 || filler == (common.Address{}) {
		return nil, nil
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
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("liquidlane: authorization multicall: got %d results, want %d", len(res), len(calls))
	}

	auths := make([]Auth, len(adapters))
	resolved := make([]bool, len(adapters))
	var delegatedChecks []int
	for i := range adapters {
		mm, ow := res[i*2], res[i*2+1]
		if !mm.Success || !ow.Success {
			continue
		}
		marketMaker, e1 := llAdapter.UnpackMarketMaker(mm.ReturnData)
		owner, e2 := llAdapter.UnpackOwner(ow.ReturnData)
		if e1 != nil || e2 != nil {
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
			return nil, err
		}
		if len(delegationResults) != len(delegationCalls) {
			return nil, errors.Errorf(
				"liquidlane: filler authorization multicall: got %d results, want %d",
				len(delegationResults),
				len(delegationCalls),
			)
		}
		for j, i := range delegatedChecks {
			if delegationResults[j].Success {
				if ok, derr := llAdapter.UnpackIsFiller(delegationResults[j].ReturnData); derr == nil {
					auths[i].IsFiller = ok
					auths[i].Authorized = ok
				}
			}
		}
	}

	out := make([]Auth, 0, len(auths))
	for i, item := range auths {
		if resolved[i] {
			out = append(out, item)
		}
	}
	return out, nil
}

func unpaused(result chain.CallResult) bool {
	if !result.Success {
		return false
	}
	paused, err := llAdapter.UnpackPaused(result.ReturnData)
	return err == nil && !paused
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
				"liquidlane: resolve adapter %s tokensToRedeem length: call failed",
				adapters[i].Adapter.Hex(),
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
