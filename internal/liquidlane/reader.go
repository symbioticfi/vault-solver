package liquidlane

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/lens"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const DefaultMaxTokensPerAdapter = 64

var (
	adapterBinding = adapter.NewLiquidLaneAdapter()
	erc4626Binding = erc4626.NewIERC4626()
	vaultBinding   = vaultv2.NewIVaultV2()
	lensBinding    = lens.NewFrontendLiquidityLens()
)

type liquidLaneBackend interface {
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
}

type decimalsReader interface {
	Get(ctx context.Context, token common.Address) (int, error)
}

// Reader owns the complete on-chain read boundary for LiquidLane.
type Reader struct {
	chain liquidLaneBackend
	log   logr.Logger
	dec   decimalsReader
	gas   priceReader

	chainID             int64
	maxTokensPerAdapter int
	lens                common.Address
}

func NewReader(
	client *chain.Client,
	log logr.Logger,
	liquidityLens common.Address,
	gasConfig *liquidlanegas.OracleConfig,
) (*Reader, error) {
	reader := &Reader{
		chain:               client,
		log:                 log,
		dec:                 chain.NewDecimals(client),
		chainID:             client.ChainID().Int64(),
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
		lens:                liquidityLens,
	}
	if gasConfig != nil {
		prices, err := liquidlanegas.NewOracleReader(client, *gasConfig)
		if err != nil {
			return nil, err
		}
		reader.gas = prices
	}
	return reader, nil
}

func (r *Reader) TokenDecimals(ctx context.Context, token common.Address) (int, error) {
	return r.dec.Get(ctx, token)
}

func (r *Reader) execute(ctx context.Context, label string, calls []chain.Call) ([]chain.CallResult, error) {
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, errors.Errorf("liquidlane: %s multicall: %w", label, err)
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("liquidlane: %s multicall: got %d results, want %d", label, len(results), len(calls))
	}
	return results, nil
}

func (r *Reader) maxAssets(adapterAddress, token common.Address) chain.Call {
	if r.lens != (common.Address{}) {
		return chain.Call{
			Target:       r.lens,
			AllowFailure: true,
			Data:         lensBinding.PackGetMaxAssets0(adapterAddress, token),
		}
	}
	return chain.Call{
		Target:       adapterAddress,
		AllowFailure: true,
		Data:         adapterBinding.PackGetMaxAssets(token),
	}
}

func uniqueAddresses(values []common.Address) []common.Address {
	seen := make(map[common.Address]struct{}, len(values))
	unique := make([]common.Address, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func uniqueAdapters(values []Adapter) []Adapter {
	seen := make(map[common.Address]struct{}, len(values))
	unique := make([]Adapter, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value.Adapter]; exists {
			continue
		}
		seen[value.Adapter] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func validRoutes(values []Route) []Route {
	seen := make(map[RouteID]struct{}, len(values))
	valid := make([]Route, 0, len(values))
	for _, route := range values {
		if route.Adapter == (common.Address{}) || route.TokenIn == (common.Address{}) ||
			route.TokenOut == (common.Address{}) {
			continue
		}
		if _, exists := seen[route.ID]; exists {
			continue
		}
		seen[route.ID] = struct{}{}
		valid = append(valid, route)
	}
	return valid
}

func validInventory(values []Inventory) []Inventory {
	seen := make(map[CandidateID]struct{}, len(values))
	valid := make([]Inventory, 0, len(values))
	for _, inventory := range values {
		if len(validRoutes([]Route{inventory.Route})) == 0 || !positive(inventory.MaxAssets) {
			continue
		}
		id := NewCandidateID(inventory.Route, inventory.DiscountID)
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		valid = append(valid, inventory)
	}
	return valid
}
