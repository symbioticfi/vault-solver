package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

type capacityAdapter struct {
	owner       common.Address
	marketMaker common.Address
	valid       bool
	lensIndex   int
}

type capacityRoute struct {
	account     common.Address
	ownerIndex  int
	makerIndex  int
	assetsIndex int
}

// inventoryCalls avoids running the lens's sweepPending/deallocation cascade once per token.
// Metadata and the final batch use the same block hash. Only call-local results are reused.
func (r *Reader) inventoryCalls(ctx context.Context, routes []Route, calls []chain.Call) ([]chain.CallResult, error) {
	adapters := make([]common.Address, 0, len(routes))
	for _, route := range routes {
		adapters = append(adapters, route.Adapter)
	}
	adapters = dedupeAddresses(adapters)
	if r.lens == (common.Address{}) || len(adapters) == len(routes) {
		return r.chain.Multicall(ctx, calls)
	}
	header, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, errors.Errorf("liquidlane: capacity block: %w", err)
	}
	if header == nil {
		return nil, errors.New("liquidlane: capacity block missing")
	}
	block := header.Hash()
	metadata := capacityMetadataCalls(adapters, routes)
	results, err := r.chain.MulticallAtHash(ctx, metadata, block)
	if err != nil {
		return nil, errors.Errorf("liquidlane: capacity metadata: %w", err)
	}
	if len(results) != len(metadata) {
		return nil, errors.Errorf("liquidlane: capacity metadata: got %d results, want %d", len(results), len(metadata))
	}
	states, tokens := decodeCapacityMetadata(adapters, routes, results)
	batch := r.capacityCalls(routes, states, tokens)
	offset := len(batch)
	// The max-assets slot becomes the token limit; after decoding it is replaced with the
	// combined capacity so the inventory's existing pause, rate, and price checks stay intact.
	for i, route := range routes {
		calls[i*inventoryReadsPerRoute+1] = chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackLimit(route.TokenIn)}
	}
	batch = append(batch, calls...)
	results, err = r.chain.MulticallAtHash(ctx, batch, block)
	if err != nil {
		return nil, errors.Errorf("liquidlane: capacity inventory: %w", err)
	}
	if len(results) != len(batch) {
		return nil, errors.Errorf("liquidlane: capacity inventory: got %d results, want %d", len(results), len(batch))
	}
	for i, route := range routes {
		index := offset + i*inventoryReadsPerRoute + 1
		amount := decodeTokenCapacity(states[route.Adapter], tokens[i], results, results[index])
		results[index] = chain.CallResult{}
		if amount != nil {
			results[index] = chain.CallResult{Success: true, ReturnData: amount.FillBytes(make([]byte, 32))}
		}
	}
	return results[offset:], nil
}

func capacityMetadataCalls(adapters []common.Address, routes []Route) []chain.Call {
	calls := make([]chain.Call, 0, len(adapters)*2+len(routes))
	for _, address := range adapters {
		calls = append(calls,
			chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackOwner()},
			chain.Call{Target: address, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
		)
	}
	for _, route := range routes {
		calls = append(calls, chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackAccounts(route.TokenIn)})
	}
	return calls
}

func decodeCapacityMetadata(adapters []common.Address, routes []Route, results []chain.CallResult) (map[common.Address]*capacityAdapter, []capacityRoute) {
	states := make(map[common.Address]*capacityAdapter, len(adapters))
	for i, address := range adapters {
		owner, maker := results[i*2], results[i*2+1]
		state := &capacityAdapter{}
		states[address] = state
		if owner.Success && maker.Success {
			var ownerErr, makerErr error
			state.owner, ownerErr = llAdapter.UnpackOwner(owner.ReturnData)
			state.marketMaker, makerErr = llAdapter.UnpackMarketMaker(maker.ReturnData)
			state.valid = ownerErr == nil && makerErr == nil
		}
	}
	tokens := make([]capacityRoute, len(routes))
	for i := range routes {
		tokens[i] = capacityRoute{ownerIndex: -1, makerIndex: -1, assetsIndex: -1}
		result := results[len(adapters)*2+i]
		if result.Success {
			account, err := llAdapter.UnpackAccounts(result.ReturnData)
			if err == nil {
				tokens[i].account = account
			}
		}
	}
	return states, tokens
}

func (r *Reader) capacityCalls(routes []Route, states map[common.Address]*capacityAdapter, tokens []capacityRoute) []chain.Call {
	var calls []chain.Call
	// Acquire balances are storage reads and do not change during sweepPending. Read them
	// before the sweep, as the lens's token overload does, and never count the same owner twice.
	for i, route := range routes {
		state := states[route.Adapter]
		if !state.valid {
			continue
		}
		tokens[i].ownerIndex = len(calls)
		calls = append(calls, chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(route.TokenIn, state.owner)})
		if state.marketMaker != state.owner {
			tokens[i].makerIndex = len(calls)
			calls = append(calls, chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(route.TokenIn, state.marketMaker)})
		}
	}
	for _, route := range routes {
		state := states[route.Adapter]
		if !state.valid || state.lensIndex != 0 {
			continue
		}
		// Store a 1-based index so an unbuilt/failed adapter has no lens result.
		state.lensIndex = len(calls) + 1
		calls = append(calls, chain.Call{Target: r.lens, AllowFailure: true, Data: lensB.PackGetMaxAssets(route.Adapter)})
	}
	for i, token := range tokens {
		if token.account != (common.Address{}) {
			tokens[i].assetsIndex = len(calls)
			// IAccount.totalAssets() uses the same selector and uint256 result as IERC4626.
			calls = append(calls, chain.Call{Target: token.account, AllowFailure: true, Data: erc4626b.PackTotalAssets()})
		}
	}
	return calls
}

func decodeTokenCapacity(state *capacityAdapter, token capacityRoute, results []chain.CallResult, limitResult chain.CallResult) *big.Int {
	if !state.valid || state.lensIndex == 0 {
		return nil
	}
	shared := capacityUint(results, state.lensIndex-1)
	acquired := capacityUint(results, token.ownerIndex)
	if acquired == nil {
		return nil
	}
	if token.makerIndex >= 0 {
		maker := capacityUint(results, token.makerIndex)
		if maker == nil {
			return nil
		}
		acquired.Add(acquired, maker)
	}
	var limit *big.Int
	if limitResult.Success {
		var err error
		limit, err = llAdapter.UnpackLimit(limitResult.ReturnData)
		if err != nil {
			limit = nil
		}
	}
	return tokenCapacity(shared, acquired, limit, capacityUint(results, token.assetsIndex))
}

func capacityUint(results []chain.CallResult, index int) *big.Int {
	if index < 0 || !results[index].Success {
		return nil
	}
	// All capacity quantities have the same uint256 wire format.
	value, err := erc4626b.UnpackTotalAssets(results[index].ReturnData)
	if err != nil {
		return nil
	}
	return value
}

// tokenCapacity mirrors FrontendLiquidityLens.getMaxAssets(adapter, token): acquired assets
// plus min(shared capacity, saturatingSub(token limit, post-sweep account totalAssets)).
func tokenCapacity(shared, acquired, limit, allocated *big.Int) *big.Int {
	if shared == nil || acquired == nil || acquired.BitLen() > 256 {
		return nil
	}
	amount := new(big.Int).Set(acquired)
	if shared.Sign() != 0 {
		if limit == nil || allocated == nil {
			return nil
		}
		headroom := new(big.Int).Sub(limit, allocated)
		if headroom.Sign() > 0 {
			if headroom.Cmp(shared) > 0 {
				headroom.Set(shared)
			}
			amount.Add(amount, headroom)
		}
	}
	if amount.BitLen() > 256 {
		return nil
	}
	return amount
}
