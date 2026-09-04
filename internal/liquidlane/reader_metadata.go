package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func (r *Reader) ResolveAdapters(ctx context.Context, addresses []common.Address) ([]Adapter, error) {
	addresses = uniqueAddresses(addresses)
	if len(addresses) == 0 {
		return nil, nil
	}

	vaultCalls := make([]chain.Call, len(addresses))
	for index, address := range addresses {
		vaultCalls[index] = chain.Call{Target: address, AllowFailure: true, Data: adapterBinding.PackVault()}
	}
	vaultResults, err := r.execute(ctx, "vault", vaultCalls)
	if err != nil {
		return nil, err
	}

	adapters := make([]Adapter, len(addresses))
	assetCalls := make([]chain.Call, len(addresses))
	for index, result := range vaultResults {
		address := addresses[index]
		if !result.Success {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: call failed", address.Hex())
		}
		vault, unpackErr := adapterBinding.UnpackVault(result.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: %w", address.Hex(), unpackErr)
		}
		if vault == (common.Address{}) {
			return nil, errors.Errorf("liquidlane: resolve adapter %s vault: zero address", address.Hex())
		}
		adapters[index] = Adapter{Adapter: address, Vault: vault}
		assetCalls[index] = chain.Call{Target: vault, AllowFailure: true, Data: erc4626Binding.PackAsset()}
	}

	assetResults, err := r.execute(ctx, "asset", assetCalls)
	if err != nil {
		return nil, err
	}
	for index, result := range assetResults {
		resolved := &adapters[index]
		if !result.Success {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s vault %s asset: call failed",
				resolved.Adapter.Hex(), resolved.Vault.Hex(),
			)
		}
		asset, unpackErr := erc4626Binding.UnpackAsset(result.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s vault %s asset: %w",
				resolved.Adapter.Hex(), resolved.Vault.Hex(), unpackErr,
			)
		}
		if asset == (common.Address{}) {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s vault %s asset: zero address",
				resolved.Adapter.Hex(), resolved.Vault.Hex(),
			)
		}
		decimals, decimalsErr := r.dec.Get(ctx, asset)
		if decimalsErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokenOut %s decimals: %w",
				resolved.Adapter.Hex(), asset.Hex(), decimalsErr,
			)
		}
		resolved.TokenOut = asset
		resolved.TokenOutDecimals = decimals
	}
	return adapters, nil
}

func (r *Reader) ResolveRoutes(ctx context.Context, addresses []common.Address) ([]Route, error) {
	adapters, err := r.ResolveAdapters(ctx, addresses)
	if err != nil || len(adapters) == 0 {
		return nil, err
	}

	countCalls := make([]chain.Call, len(adapters))
	for index, adapter := range adapters {
		countCalls[index] = chain.Call{
			Target: adapter.Adapter, AllowFailure: true, Data: adapterBinding.PackGetTokensToRedeemLength(),
		}
	}
	countResults, err := r.execute(ctx, "tokensToRedeem length", countCalls)
	if err != nil {
		return nil, err
	}

	type tokenPosition struct{ adapter, token int }
	positions := make([]tokenPosition, 0)
	tokenCalls := make([]chain.Call, 0)
	for adapterIndex, result := range countResults {
		address := adapters[adapterIndex].Adapter
		if !result.Success {
			return nil, errors.Errorf("liquidlane: resolve adapter %s tokensToRedeem length: call failed", address.Hex())
		}
		count, unpackErr := adapterBinding.UnpackGetTokensToRedeemLength(result.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf("liquidlane: resolve adapter %s tokensToRedeem length: %w", address.Hex(), unpackErr)
		}
		if count == nil || !count.IsInt64() || count.Sign() < 0 {
			return nil, errors.Errorf("liquidlane: resolve adapter %s tokensToRedeem length: invalid value %v", address.Hex(), count)
		}
		if count.Sign() == 0 {
			return nil, errors.Errorf("liquidlane: resolve adapter %s: tokensToRedeem is empty", address.Hex())
		}
		if count.Cmp(big.NewInt(int64(r.maxTokensPerAdapter))) > 0 {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem length %s exceeds cap %d",
				address.Hex(), count, r.maxTokensPerAdapter,
			)
		}
		for tokenIndex := range int(count.Int64()) {
			positions = append(positions, tokenPosition{adapter: adapterIndex, token: tokenIndex})
			tokenCalls = append(tokenCalls, chain.Call{
				Target: address, AllowFailure: true,
				Data: adapterBinding.PackTokensToRedeem(big.NewInt(int64(tokenIndex))),
			})
		}
	}

	tokenResults, err := r.execute(ctx, "tokensToRedeem", tokenCalls)
	if err != nil {
		return nil, err
	}
	routes := make([]Route, 0, len(tokenResults))
	for index, result := range tokenResults {
		position := positions[index]
		resolved := adapters[position.adapter]
		if !result.Success {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem[%d]: call failed",
				resolved.Adapter.Hex(), position.token,
			)
		}
		token, unpackErr := adapterBinding.UnpackTokensToRedeem(result.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf(
				"liquidlane: resolve adapter %s tokensToRedeem[%d]: %w",
				resolved.Adapter.Hex(), position.token, unpackErr,
			)
		}
		route, routeErr := r.routeFor(ctx, resolved, token)
		if routeErr != nil {
			if token == (common.Address{}) {
				return nil, errors.Errorf(
					"liquidlane: resolve adapter %s tokensToRedeem[%d]: zero address",
					resolved.Adapter.Hex(), position.token,
				)
			}
			return nil, routeErr
		}
		routes = append(routes, route)
	}
	return validRoutes(routes), nil
}

func (r *Reader) RoutesForToken(ctx context.Context, adapters []Adapter, token common.Address) []Route {
	routes := make([]Route, 0, len(adapters))
	for _, resolved := range uniqueAdapters(adapters) {
		route, err := r.routeFor(ctx, resolved, token)
		if err != nil {
			r.log.Error(err, "liquidlane: route unresolved", "adapter", resolved.Adapter.Hex(), "tokenIn", token.Hex())
			continue
		}
		routes = append(routes, route)
	}
	return validRoutes(routes)
}

func (r *Reader) routeFor(ctx context.Context, adapter Adapter, token common.Address) (Route, error) {
	if token == (common.Address{}) {
		return Route{}, errors.Errorf("liquidlane: resolve adapter %s tokenIn: zero address", adapter.Adapter.Hex())
	}
	decimals, err := r.dec.Get(ctx, token)
	if err != nil {
		return Route{}, errors.Errorf(
			"liquidlane: resolve adapter %s tokenIn %s decimals: %w",
			adapter.Adapter.Hex(), token.Hex(), err,
		)
	}
	return NewRoute(
		r.chainID, adapter.Adapter, adapter.Vault, token, adapter.TokenOut, decimals, adapter.TokenOutDecimals,
	), nil
}
