package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

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

func unpaused(result chain.CallResult) bool {
	if !result.Success {
		return false
	}
	paused, err := llAdapter.UnpackPaused(result.ReturnData)
	return err == nil && !paused
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
