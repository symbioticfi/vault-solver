package liquidlane

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func (r *Reader) ReadAuth(ctx context.Context, addresses []common.Address, filler common.Address) ([]Auth, error) {
	addresses = uniqueAddresses(addresses)
	if len(addresses) == 0 || filler == (common.Address{}) {
		return nil, nil
	}

	roleCalls := make([]chain.Call, 0, len(addresses)*2)
	for _, address := range addresses {
		roleCalls = append(roleCalls,
			chain.Call{Target: address, AllowFailure: true, Data: adapterBinding.PackMarketMaker()},
			chain.Call{Target: address, AllowFailure: true, Data: adapterBinding.PackOwner()},
		)
	}
	roleResults, err := r.execute(ctx, "authorization", roleCalls)
	if err != nil {
		return nil, err
	}

	resolved := make([]Auth, len(addresses))
	available := make([]bool, len(addresses))
	delegated := make([]int, 0, len(addresses))
	for index, address := range addresses {
		makerResult, ownerResult := roleResults[index*2], roleResults[index*2+1]
		if !makerResult.Success || !ownerResult.Success {
			continue
		}
		maker, makerErr := adapterBinding.UnpackMarketMaker(makerResult.ReturnData)
		owner, ownerErr := adapterBinding.UnpackOwner(ownerResult.ReturnData)
		if makerErr != nil || ownerErr != nil {
			continue
		}
		auth := Auth{Adapter: address, MarketMaker: maker, Owner: owner}
		auth.Authorized = maker == filler || owner == filler
		resolved[index] = auth
		available[index] = true
		if !auth.Authorized {
			delegated = append(delegated, index)
		}
	}

	if len(delegated) > 0 {
		fillerCalls := make([]chain.Call, len(delegated))
		for callIndex, authIndex := range delegated {
			auth := resolved[authIndex]
			fillerCalls[callIndex] = chain.Call{
				Target: auth.Adapter, AllowFailure: true,
				Data: adapterBinding.PackIsFiller(auth.MarketMaker, filler),
			}
		}
		fillerResults, fillerErr := r.execute(ctx, "filler authorization", fillerCalls)
		if fillerErr != nil {
			return nil, fillerErr
		}
		for callIndex, authIndex := range delegated {
			result := fillerResults[callIndex]
			if !result.Success {
				continue
			}
			isFiller, unpackErr := adapterBinding.UnpackIsFiller(result.ReturnData)
			if unpackErr == nil {
				resolved[authIndex].IsFiller = isFiller
				resolved[authIndex].Authorized = isFiller
			}
		}
	}

	auth := make([]Auth, 0, len(addresses))
	for index, value := range resolved {
		if available[index] {
			auth = append(auth, value)
		}
	}
	return auth, nil
}

func (r *Reader) FilterAuthorized(ctx context.Context, inventory []Inventory, filler common.Address) ([]Inventory, error) {
	inventory = validInventory(inventory)
	if len(inventory) == 0 {
		return nil, nil
	}
	addresses := make([]common.Address, len(inventory))
	for index, item := range inventory {
		addresses[index] = item.Adapter
	}
	allowed, err := r.authorizationMap(ctx, addresses, filler)
	if err != nil {
		return nil, err
	}
	filtered := make([]Inventory, 0, len(inventory))
	for _, item := range inventory {
		if allowed[item.Adapter] {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (r *Reader) FilterAuthorizedRoutes(ctx context.Context, routes []Route, filler common.Address) ([]Route, error) {
	routes = validRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}
	addresses := make([]common.Address, len(routes))
	for index, route := range routes {
		addresses[index] = route.Adapter
	}
	allowed, err := r.authorizationMap(ctx, addresses, filler)
	if err != nil {
		return nil, err
	}
	filtered := make([]Route, 0, len(routes))
	for _, route := range routes {
		if allowed[route.Adapter] {
			filtered = append(filtered, route)
		}
	}
	return filtered, nil
}

func (r *Reader) authorizationMap(
	ctx context.Context,
	addresses []common.Address,
	filler common.Address,
) (map[common.Address]bool, error) {
	auth, err := r.ReadAuth(ctx, addresses, filler)
	if err != nil {
		return nil, errors.Errorf("liquidlane: read authorization: %w", err)
	}
	allowed := make(map[common.Address]bool, len(auth))
	for _, value := range auth {
		allowed[value.Adapter] = value.Authorized
	}
	return allowed, nil
}
