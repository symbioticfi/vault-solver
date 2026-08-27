package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

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
