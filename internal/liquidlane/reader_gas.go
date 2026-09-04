package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

type adapterRoutes struct {
	address common.Address
	vault   common.Address
	routes  []Route
}

type adapterRoles struct {
	owner       common.Address
	marketMaker common.Address
	state       *liquidlanegas.AdapterState
}

type acquirePosition struct {
	adapter common.Address
	token   common.Address
}

func (r *Reader) ReadGasSnapshot(ctx context.Context, routes []Route) (*liquidlanegas.Snapshot, error) {
	routes = validRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}

	groups, vaults := groupRoutes(routes)
	headCalls := make([]chain.Call, 0, len(groups)*2+len(vaults)*2)
	for _, group := range groups {
		headCalls = append(headCalls,
			chain.Call{Target: group.address, AllowFailure: true, Data: adapterBinding.PackOwner()},
			chain.Call{Target: group.address, AllowFailure: true, Data: adapterBinding.PackMarketMaker()},
		)
	}
	for _, vault := range vaults {
		headCalls = append(headCalls,
			chain.Call{Target: vault, AllowFailure: true, Data: vaultBinding.PackFreeAssets()},
			chain.Call{Target: vault, AllowFailure: true, Data: vaultBinding.PackWithdrawable()},
		)
	}
	headResults, err := r.execute(ctx, "gas state head", headCalls)
	if err != nil {
		return nil, err
	}

	roles := decodeRoles(groups, headResults[:len(groups)*2])
	vaultState := decodeVaults(vaults, headResults[len(groups)*2:])
	acquireCalls, positions := acquireBalanceCalls(groups, roles)
	if len(acquireCalls) > 0 {
		acquireResults, acquireErr := r.execute(ctx, "gas state acquire", acquireCalls)
		if acquireErr != nil {
			return nil, acquireErr
		}
		applyAcquireBalances(roles, positions, acquireResults)
	}

	adapters := make(map[common.Address]*liquidlanegas.AdapterState, len(roles))
	for address, role := range roles {
		adapters[address] = role.state
	}
	return &liquidlanegas.Snapshot{Adapters: adapters, Vaults: vaultState}, nil
}

func groupRoutes(routes []Route) ([]adapterRoutes, []common.Address) {
	groups := make([]adapterRoutes, 0, len(routes))
	indexByAdapter := make(map[common.Address]int, len(routes))
	vaults := make([]common.Address, 0, len(routes))
	seenVaults := make(map[common.Address]struct{}, len(routes))
	for _, route := range routes {
		index, exists := indexByAdapter[route.Adapter]
		if !exists {
			index = len(groups)
			indexByAdapter[route.Adapter] = index
			groups = append(groups, adapterRoutes{address: route.Adapter, vault: route.Vault})
		}
		groups[index].routes = append(groups[index].routes, route)
		if _, vaultExists := seenVaults[route.Vault]; !vaultExists {
			seenVaults[route.Vault] = struct{}{}
			vaults = append(vaults, route.Vault)
		}
	}
	return groups, vaults
}

func decodeRoles(groups []adapterRoutes, results []chain.CallResult) map[common.Address]*adapterRoles {
	roles := make(map[common.Address]*adapterRoles, len(groups))
	for index, group := range groups {
		ownerResult, makerResult := results[index*2], results[index*2+1]
		if !ownerResult.Success || !makerResult.Success {
			continue
		}
		owner, ownerErr := adapterBinding.UnpackOwner(ownerResult.ReturnData)
		maker, makerErr := adapterBinding.UnpackMarketMaker(makerResult.ReturnData)
		if ownerErr != nil || makerErr != nil {
			continue
		}
		roles[group.address] = &adapterRoles{
			owner: owner, marketMaker: maker,
			state: &liquidlanegas.AdapterState{
				Vault: group.vault, Acquire: make(map[common.Address]*big.Int, len(group.routes)),
			},
		}
	}
	return roles
}

func decodeVaults(
	vaults []common.Address,
	results []chain.CallResult,
) map[common.Address]*liquidlanegas.VaultState {
	state := make(map[common.Address]*liquidlanegas.VaultState, len(vaults))
	for index, vault := range vaults {
		freeResult, withdrawableResult := results[index*2], results[index*2+1]
		if !freeResult.Success || !withdrawableResult.Success {
			continue
		}
		free, freeErr := vaultBinding.UnpackFreeAssets(freeResult.ReturnData)
		withdrawable, withdrawableErr := vaultBinding.UnpackWithdrawable(withdrawableResult.ReturnData)
		if freeErr != nil || withdrawableErr != nil || free == nil || withdrawable == nil {
			continue
		}
		state[vault] = &liquidlanegas.VaultState{FreeAssets: CloneBig(free), Withdrawable: CloneBig(withdrawable)}
	}
	return state
}

func acquireBalanceCalls(
	groups []adapterRoutes,
	roles map[common.Address]*adapterRoles,
) ([]chain.Call, []acquirePosition) {
	calls := make([]chain.Call, 0)
	positions := make([]acquirePosition, 0)
	for _, group := range groups {
		role := roles[group.address]
		if role == nil {
			continue
		}
		for _, route := range group.routes {
			keys := []common.Address{role.owner}
			if role.marketMaker != role.owner {
				keys = append(keys, role.marketMaker)
			}
			for _, key := range keys {
				calls = append(calls, chain.Call{
					Target: group.address, AllowFailure: true,
					Data: adapterBinding.PackAcquireBalance(route.TokenIn, key),
				})
				positions = append(positions, acquirePosition{adapter: group.address, token: route.TokenIn})
			}
		}
	}
	return calls, positions
}

func applyAcquireBalances(
	roles map[common.Address]*adapterRoles,
	positions []acquirePosition,
	results []chain.CallResult,
) {
	for index, position := range positions {
		result := results[index]
		if !result.Success {
			continue
		}
		amount, err := adapterBinding.UnpackAcquireBalance(result.ReturnData)
		if err != nil || amount == nil || amount.Sign() < 0 {
			continue
		}
		current := roles[position.adapter].state.Acquire[position.token]
		if current == nil {
			current = new(big.Int)
			roles[position.adapter].state.Acquire[position.token] = current
		}
		current.Add(current, amount)
	}
}

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

	pauses, err := r.pauseStates(ctx, []common.Address{adapterAddress})
	if err != nil {
		return AdapterSnapshot{}, err
	}
	paused, resolved := pauses[adapterAddress]
	if !resolved {
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

	byRoute := make(map[RouteID]Inventory, len(routes))
	if paused {
		for _, route := range routes {
			byRoute[route.ID] = DirectInventory(route, new(big.Int), new(big.Int))
		}
	} else {
		inventory, inventoryErr := r.inventory(ctx, routes, true)
		if inventoryErr != nil {
			return AdapterSnapshot{}, inventoryErr
		}
		for _, item := range inventory {
			byRoute[item.ID] = item
		}
		if len(byRoute) != len(routes) {
			return AdapterSnapshot{}, errors.New("liquidlane: adapter inventory unresolved")
		}
	}

	first := routes[0]
	snapshot := AdapterSnapshot{
		Adapter: Adapter{
			Adapter: first.Adapter, Vault: first.Vault,
			TokenOut: first.TokenOut, TokenOutDecimals: first.TokenOutDecimals,
		},
		Paused:       paused,
		Authorized:   auth[0].Authorized,
		FreeAssets:   CloneBig(vaultState.FreeAssets),
		Withdrawable: CloneBig(vaultState.Withdrawable),
		Routes:       make([]RouteSnapshot, 0, len(routes)),
	}
	for _, route := range routes {
		item := byRoute[route.ID]
		snapshot.Routes = append(snapshot.Routes, RouteSnapshot{
			Route: route, MaxAssets: CloneBig(item.MaxAssets), MaxRate: CloneBig(item.MaxRate),
			AcquireBalance: CloneBig(adapterState.Acquire[route.TokenIn]),
		})
	}
	return snapshot, nil
}
