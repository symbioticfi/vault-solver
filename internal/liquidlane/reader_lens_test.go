package liquidlane

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/lens"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

type lensInventoryBackend struct {
	calls   [][]chain.Call
	results []chain.CallResult
	err     error
}

func (b *lensInventoryBackend) ChainID() *big.Int { return big.NewInt(11155111) }

func (b *lensInventoryBackend) Multicall(_ context.Context, calls []chain.Call) ([]chain.CallResult, error) {
	b.calls = append(b.calls, calls)
	return b.results, b.err
}

func batchLensOutput(t *testing.T, values []lens.FrontendLiquidityLensMaxAssetsResult) chain.CallResult {
	t.Helper()
	parsed, err := lens.FrontendLiquidityLensMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	data, err := parsed.Methods["getMaxAssetsBatch"].Outputs.Pack(values)
	if err != nil {
		t.Fatal(err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func lensInventoryFixture(t *testing.T) (*Reader, *lensInventoryBackend, []Route) {
	t.Helper()
	first, second, third := testReaderRoute(1), testReaderRoute(2), testReaderRoute(3)
	third = NewRoute(11155111, first.Adapter, first.Vault, third.TokenIn, first.TokenOut, 18, 6)
	routes := []Route{first, second, third}
	backend := &lensInventoryBackend{results: []chain.CallResult{
		batchLensOutput(t, []lens.FrontendLiquidityLensMaxAssetsResult{
			{Success: true, MaxAssets: big.NewInt(62)}, {Success: false, MaxAssets: new(big.Int)},
		}),
		batchLensOutput(t, []lens.FrontendLiquidityLensMaxAssetsResult{{Success: true, MaxAssets: big.NewInt(12)}}),
	}}
	for range routes {
		backend.results = append(backend.results,
			successOutput(t, "paused", false),
			successOutput(t, "getMaxRate", big.NewInt(1_000_000_000_000_000_000)),
			successOutput(t, "minDiscount", new(big.Int)),
			successOutput(t, "getAmountOut", big.NewInt(1_000_000_000_000_000_000)),
		)
	}
	return &Reader{chain: backend, log: logr.Discard(), lens: common.HexToAddress("0x9999")}, backend, routes
}

func TestInventoryUsesOneLensBatchPerAdapterInOneMulticall(t *testing.T) {
	r, backend, routes := lensInventoryFixture(t)
	inventory, err := r.ReadInventory(t.Context(), append(routes, routes[0]))
	if err != nil || len(inventory) != 2 {
		t.Fatalf("inventory count = %d, error = %v", len(inventory), err)
	}
	if inventory[0].ID != routes[0].ID || inventory[0].MaxAssets.Int64() != 62 ||
		inventory[1].ID != routes[1].ID || inventory[1].MaxAssets.Int64() != 12 {
		t.Fatalf("route order/capacity changed: %+v", inventory)
	}
	if len(backend.calls) != 1 || len(backend.calls[0]) != 14 {
		t.Fatalf("want one multicall with 2 lens and 12 ordinary reads, got %+v", backend.calls)
	}
	for i, tokens := range [][]common.Address{{routes[0].TokenIn, routes[2].TokenIn}, {routes[1].TokenIn}} {
		call := backend.calls[0][i]
		if call.Target != r.lens || !call.AllowFailure || !bytes.Equal(call.Data, lensB.PackGetMaxAssetsBatch(routes[i].Adapter, tokens)) {
			t.Errorf("incorrect lens batch %d: %+v", i, call)
		}
	}
}

func TestInventoryLensBatchFailuresStayScoped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result chain.CallResult
	}{
		{name: "reverted"},
		{name: "malformed", result: chain.CallResult{Success: true, ReturnData: []byte{1}}},
		{name: "wrong token count", result: batchLensOutput(t, []lens.FrontendLiquidityLensMaxAssetsResult{{Success: true, MaxAssets: big.NewInt(62)}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, backend, routes := lensInventoryFixture(t)
			backend.results[0] = tc.result
			inventory, err := r.ReadInventory(t.Context(), routes)
			if err != nil || len(inventory) != 1 || inventory[0].ID != routes[1].ID {
				t.Fatalf("unaffected adapter lost: inventory %+v, error %v", inventory, err)
			}
		})
	}
}

func TestInventoryPreservesDistinctNonadjacentTokenCapacities(t *testing.T) {
	r, backend, routes := lensInventoryFixture(t)
	backend.results[0] = batchLensOutput(t, []lens.FrontendLiquidityLensMaxAssetsResult{
		{Success: true, MaxAssets: big.NewInt(62)}, {Success: true, MaxAssets: big.NewInt(37)},
	})
	inventory, err := r.ReadInventory(t.Context(), routes)
	if err != nil || len(inventory) != 3 {
		t.Fatalf("inventory count = %d, error = %v", len(inventory), err)
	}
	for i, want := range []int64{62, 12, 37} {
		if inventory[i].ID != routes[i].ID || inventory[i].MaxAssets.Cmp(big.NewInt(want)) != 0 {
			t.Errorf("route %d: %+v, want capacity %d", i, inventory[i], want)
		}
	}
}

func TestInventoryLensTransportFailureAbortsRead(t *testing.T) {
	for _, incomplete := range []bool{false, true} {
		r, backend, routes := lensInventoryFixture(t)
		want := "RPC unavailable"
		backend.err = errors.New(want)
		if incomplete {
			backend.err = nil
			backend.results = backend.results[:len(backend.results)-1]
			want = "results, want"
		}
		inventory, err := r.ReadInventory(t.Context(), routes)
		if len(inventory) != 0 || err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("inventory %+v, error %v; want %q", inventory, err, want)
		}
	}
}
