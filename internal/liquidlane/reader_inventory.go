package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

const inventoryFields = 4

func (r *Reader) ReadInventory(ctx context.Context, routes []Route) ([]Inventory, error) {
	return r.inventory(ctx, routes, false)
}

func (r *Reader) inventory(ctx context.Context, routes []Route, includeEmpty bool) ([]Inventory, error) {
	routes = validRoutes(routes)
	if len(routes) == 0 {
		return nil, nil
	}

	calls := make([]chain.Call, 0, len(routes)*inventoryFields)
	for _, route := range routes {
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: adapterBinding.PackPaused()},
			r.maxAssets(route.Adapter, route.TokenIn),
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: adapterBinding.PackGetMaxRate(route.TokenIn)},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: adapterBinding.PackMinDiscount(route.TokenIn)},
		)
	}
	results, err := r.execute(ctx, "inventory", calls)
	if err != nil {
		return nil, err
	}

	inventory := make([]Inventory, 0, len(routes))
	for index, route := range routes {
		result := results[index*inventoryFields : (index+1)*inventoryFields]
		paused, valid := decodePaused(result[0])
		if !valid || paused || !result[1].Success || !result[2].Success || !result[3].Success {
			continue
		}
		maxAssets, assetsErr := adapterBinding.UnpackGetMaxAssets(result[1].ReturnData)
		maxRate, rateErr := adapterBinding.UnpackGetMaxRate(result[2].ReturnData)
		minDiscount, discountErr := adapterBinding.UnpackMinDiscount(result[3].ReturnData)
		if assetsErr != nil || rateErr != nil || discountErr != nil ||
			maxAssets == nil || maxRate == nil || !validDiscount(minDiscount) {
			continue
		}
		if !includeEmpty && (!positive(maxAssets) || !positive(maxRate)) {
			continue
		}
		item := DirectInventory(route, maxAssets, maxRate)
		item.AdapterMinDiscount = CloneBig(minDiscount)
		inventory = append(inventory, item)
	}
	return inventory, nil
}

func (r *Reader) ReadFillQuotes(
	ctx context.Context,
	routes []Route,
	token common.Address,
	amountIn *big.Int,
) ([]FillQuote, error) {
	if token == (common.Address{}) || !positive(amountIn) {
		return nil, nil
	}

	eligible := make([]Route, 0, len(routes))
	for _, route := range validRoutes(routes) {
		if route.TokenIn == token {
			eligible = append(eligible, route)
		}
	}
	if len(eligible) == 0 {
		return nil, nil
	}

	calls := make([]chain.Call, 0, len(eligible)*inventoryFields)
	for _, route := range eligible {
		calls = append(calls,
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: adapterBinding.PackPaused()},
			r.maxAssets(route.Adapter, route.TokenIn),
			chain.Call{
				Target: route.Adapter, AllowFailure: true,
				Data: adapterBinding.PackGetAmountOut(route.TokenIn, amountIn),
			},
			chain.Call{Target: route.Adapter, AllowFailure: true, Data: adapterBinding.PackMinDiscount(route.TokenIn)},
		)
	}
	results, err := r.execute(ctx, "fill", calls)
	if err != nil {
		return nil, err
	}

	quotes := make([]FillQuote, 0, len(eligible))
	for index, route := range eligible {
		result := results[index*inventoryFields : (index+1)*inventoryFields]
		paused, valid := decodePaused(result[0])
		if !valid || paused || !result[1].Success || !result[2].Success || !result[3].Success {
			continue
		}
		maxAssets, assetsErr := adapterBinding.UnpackGetMaxAssets(result[1].ReturnData)
		gross, outputErr := adapterBinding.UnpackGetAmountOut(result[2].ReturnData)
		discount, discountErr := adapterBinding.UnpackMinDiscount(result[3].ReturnData)
		if assetsErr != nil || outputErr != nil || discountErr != nil ||
			!positive(maxAssets) || !positive(gross) || !validDiscount(discount) {
			continue
		}
		net := AmountOutAfterDiscount(gross, discount)
		if !positive(net) {
			continue
		}
		item := DirectInventory(
			route,
			maxAssets,
			RateForAmountOut(net, amountIn, route.TokenInDecimals, route.TokenOutDecimals),
		)
		item.AdapterMinDiscount = CloneBig(discount)
		quotes = append(quotes, FillQuote{
			Inventory:      item,
			AmountIn:       CloneBig(amountIn),
			GrossAmountOut: CloneBig(gross),
			MaxAmountOut:   net,
			MinDiscount:    CloneBig(discount),
		})
	}
	return quotes, nil
}

func (r *Reader) pauseStates(ctx context.Context, addresses []common.Address) (map[common.Address]bool, error) {
	addresses = uniqueAddresses(addresses)
	if len(addresses) == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, len(addresses))
	for index, address := range addresses {
		calls[index] = chain.Call{Target: address, AllowFailure: true, Data: adapterBinding.PackPaused()}
	}
	results, err := r.execute(ctx, "paused", calls)
	if err != nil {
		return nil, err
	}
	states := make(map[common.Address]bool, len(addresses))
	for index, result := range results {
		paused, valid := decodePaused(result)
		if valid {
			states[addresses[index]] = paused
		}
	}
	return states, nil
}

func decodePaused(result chain.CallResult) (paused bool, valid bool) {
	if !result.Success {
		return false, false
	}
	paused, err := adapterBinding.UnpackPaused(result.ReturnData)
	return paused, err == nil
}

func validDiscount(discount *big.Int) bool {
	return discount != nil && discount.Sign() >= 0 && discount.Cmp(big.NewInt(DiscountPrecision)) <= 0
}
