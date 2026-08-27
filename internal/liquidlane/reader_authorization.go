package liquidlane

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

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
