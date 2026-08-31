package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

type gasRouteGroup struct {
	adapter common.Address
	vault   common.Address
	routes  []Route
}

type gasAcquireRead struct {
	adapter common.Address
	token   common.Address
}

// ReadGasSnapshot returns the latest adapter-local acquire balances and shared vault liquidity needed
// to predict LiquidLane swap gas. Partially unread state remains absent and is priced as RouteUnknown.
func (r *Reader) ReadGasSnapshot(ctx context.Context, routes []Route) (*liquidlanegas.Snapshot, error) {
	routes = compactRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	groups, vaults := groupGasRoutes(routes)
	headCalls := gasHeadCalls(groups, vaults)
	headResults, err := r.chain.Multicall(ctx, headCalls)
	if err != nil {
		return nil, err
	}
	if len(headResults) != len(headCalls) {
		return nil, errors.Errorf("liquidlane: gas state head multicall: got %d results, want %d", len(headResults), len(headCalls))
	}
	states, vaultStates := decodeGasHeadResults(groups, vaults, headResults)
	acquireCalls, reads := gasAcquireCalls(groups, states)
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
	applyGasAcquireResults(states, reads, acquireResults)
	return gasSnapshot(states, vaultStates), nil
}

func groupGasRoutes(routes []Route) ([]*gasRouteGroup, []common.Address) {
	byAdapter := make(map[common.Address]*gasRouteGroup, len(routes))
	groups := make([]*gasRouteGroup, 0, len(routes))
	for _, route := range routes {
		group := byAdapter[route.Adapter]
		if group == nil {
			group = &gasRouteGroup{adapter: route.Adapter, vault: route.Vault}
			byAdapter[route.Adapter] = group
			groups = append(groups, group)
		}
		group.routes = append(group.routes, route)
	}
	vaults := make([]common.Address, 0, len(groups))
	seenVaults := make(map[common.Address]bool, len(groups))
	for _, group := range groups {
		if !seenVaults[group.vault] {
			seenVaults[group.vault] = true
			vaults = append(vaults, group.vault)
		}
	}
	return groups, vaults
}

func gasHeadCalls(groups []*gasRouteGroup, vaults []common.Address) []chain.Call {
	calls := make([]chain.Call, 0, len(groups)*2+len(vaults)*2)
	for _, group := range groups {
		calls = append(calls,
			chain.Call{Target: group.adapter, AllowFailure: true, Data: llAdapter.PackOwner()},
			chain.Call{Target: group.adapter, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
		)
	}
	for _, vault := range vaults {
		calls = append(calls,
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackFreeAssets()},
			chain.Call{Target: vault, AllowFailure: true, Data: vaultV2b.PackWithdrawable()},
		)
	}
	return calls
}

func decodeGasHeadResults(
	groups []*gasRouteGroup,
	vaults []common.Address,
	results []chain.CallResult,
) (map[common.Address]*gasAdapterState, map[common.Address]*liquidlanegas.VaultState) {
	states := make(map[common.Address]*gasAdapterState, len(groups))
	for i, group := range groups {
		ownerResult, makerResult := results[i*2], results[i*2+1]
		if !ownerResult.Success || !makerResult.Success {
			continue
		}
		owner, ownerErr := llAdapter.UnpackOwner(ownerResult.ReturnData)
		marketMaker, makerErr := llAdapter.UnpackMarketMaker(makerResult.ReturnData)
		if ownerErr != nil || makerErr != nil {
			continue
		}
		states[group.adapter] = &gasAdapterState{
			owner: owner, marketMaker: marketMaker,
			state: &liquidlanegas.AdapterState{
				Vault: group.vault, Acquire: make(map[common.Address]*big.Int, len(group.routes)),
			},
		}
	}
	vaultStates := make(map[common.Address]*liquidlanegas.VaultState, len(vaults))
	base := len(groups) * 2
	for i, vault := range vaults {
		freeResult, withdrawableResult := results[base+i*2], results[base+i*2+1]
		if !freeResult.Success || !withdrawableResult.Success {
			continue
		}
		freeAssets, freeErr := vaultV2b.UnpackFreeAssets(freeResult.ReturnData)
		withdrawable, withdrawableErr := vaultV2b.UnpackWithdrawable(withdrawableResult.ReturnData)
		if freeErr != nil || withdrawableErr != nil || freeAssets == nil || withdrawable == nil {
			continue
		}
		vaultStates[vault] = &liquidlanegas.VaultState{
			FreeAssets: new(big.Int).Set(freeAssets), Withdrawable: new(big.Int).Set(withdrawable),
		}
	}
	return states, vaultStates
}

func gasAcquireCalls(
	groups []*gasRouteGroup,
	states map[common.Address]*gasAdapterState,
) ([]chain.Call, []gasAcquireRead) {
	calls := make([]chain.Call, 0, len(groups)*2)
	reads := make([]gasAcquireRead, 0, len(groups)*2)
	for _, group := range groups {
		state := states[group.adapter]
		if state == nil {
			continue
		}
		for _, route := range group.routes {
			calls = append(calls, chain.Call{
				Target: group.adapter, AllowFailure: true,
				Data: llAdapter.PackAcquireBalance(route.TokenIn, state.owner),
			})
			reads = append(reads, gasAcquireRead{adapter: group.adapter, token: route.TokenIn})
			if state.marketMaker != state.owner {
				calls = append(calls, chain.Call{
					Target: group.adapter, AllowFailure: true,
					Data: llAdapter.PackAcquireBalance(route.TokenIn, state.marketMaker),
				})
				reads = append(reads, gasAcquireRead{adapter: group.adapter, token: route.TokenIn})
			}
		}
	}
	return calls, reads
}

func applyGasAcquireResults(
	states map[common.Address]*gasAdapterState,
	reads []gasAcquireRead,
	results []chain.CallResult,
) {
	for i, read := range reads {
		result := results[i]
		if !result.Success {
			continue
		}
		amount, err := llAdapter.UnpackAcquireBalance(result.ReturnData)
		if err != nil || amount == nil || amount.Sign() < 0 {
			continue
		}
		state := states[read.adapter]
		if state.state.Acquire[read.token] == nil {
			state.state.Acquire[read.token] = new(big.Int)
		}
		state.state.Acquire[read.token].Add(state.state.Acquire[read.token], amount)
	}
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
