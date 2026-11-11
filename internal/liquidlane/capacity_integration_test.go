//go:build integration

package liquidlane

import (
	"bytes"
	"context"
	"math/big"
	"os"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

type historicalCapacityBackend struct {
	*chain.Client

	header *types.Header
	batch  []chain.Call
}

func (b *historicalCapacityBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return b.header, nil
}

func (b *historicalCapacityBackend) Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error) {
	return b.MulticallAtHash(ctx, calls, b.header.Hash())
}

func (b *historicalCapacityBackend) MulticallAtHash(ctx context.Context, calls []chain.Call, hash common.Hash) ([]chain.CallResult, error) {
	b.batch = slices.Clone(calls)
	return b.Client.MulticallAtHash(ctx, calls, hash)
}

// Reproduce the inventory preceding VAULT-SOLVER-1B on both deployed LI.FI adapters.
// This is a read-only archive-RPC test; the addresses and block are historical fixtures.
func TestInventoryCapacityIncidentParity(t *testing.T) {
	endpoint := os.Getenv("LIQUIDLANE_FORK_RPC_URL")
	if endpoint == "" {
		t.Skip("set LIQUIDLANE_FORK_RPC_URL to an Ethereum archive RPC")
	}
	client, err := chain.Dial(t.Context(), []string{endpoint}, "", "0xcA11bde05977b3631167028862bE2a173976CA11", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	header, err := client.HeaderByNumber(t.Context(), big.NewInt(26_390_430))
	if err != nil {
		t.Fatal(err)
	}
	backend := &historicalCapacityBackend{Client: client, header: header}
	lensAddress := common.HexToAddress("0x0A7e3aDb47840779F4D85e7406c5Fb65205F0f63")
	r := NewReader(client, logr.Discard(), lensAddress)
	r.chain = backend
	for _, address := range []string{"0x97C0BAefCF688A8389108a0893144ae0a1408B3a", "0x59CDDE0D345c0eE6ED453FE7d3fb365Fe0721E85"} {
		t.Run(address, func(t *testing.T) {
			routes, resolveErr := r.ResolveRoutes(t.Context(), []common.Address{common.HexToAddress(address)})
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			var calls []chain.Call
			for _, route := range routes {
				probe, _ := OraclePriceProbe(route.TokenInDecimals, route.TokenOutDecimals)
				calls = append(calls,
					chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
					r.maxAssetsCall(route.Adapter, route.TokenIn),
					chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetMaxRate(route.TokenIn)},
					chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackMinDiscount(route.TokenIn)},
					chain.Call{Target: route.Adapter, AllowFailure: true, Data: llAdapter.PackGetAmountOut(route.TokenIn, probe)},
				)
			}
			legacy, callErr := backend.Multicall(t.Context(), calls)
			if callErr != nil {
				t.Fatal(callErr)
			}
			optimized, callErr := r.inventoryCalls(t.Context(), routes, calls)
			if callErr != nil || len(optimized) != len(legacy) {
				t.Fatalf("optimized inventory: %v, %d results, want %d", callErr, len(optimized), len(legacy))
			}
			for i := range legacy {
				if !legacy[i].Success || !optimized[i].Success || !bytes.Equal(legacy[i].ReturnData, optimized[i].ReturnData) {
					t.Errorf("result %d differs: legacy %+v, optimized %+v", i, legacy[i], optimized[i])
				}
			}
			lensCalls := 0
			for _, call := range backend.batch {
				if call.Target == lensAddress {
					lensCalls++
				}
			}
			if lensCalls != 1 {
				t.Errorf("lens calls = %d, want 1", lensCalls)
			}
			t.Logf("%d routes: all %d inventory results match; lens executions %d -> %d", len(routes), len(legacy), len(routes), lensCalls)
		})
	}
}
