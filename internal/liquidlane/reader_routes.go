package liquidlane

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

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
