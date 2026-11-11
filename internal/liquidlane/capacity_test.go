package liquidlane

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

type capacityBackend struct {
	t           *testing.T
	header      *types.Header
	values      map[string]chain.CallResult
	lens        common.Address
	lensCalls   int
	pinnedCalls int
	failPhase   int
	shortPhase  int
	headerErr   error
	onLens      func()
}

func (b *capacityBackend) ChainID() *big.Int { return big.NewInt(1) }

func (b *capacityBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return b.header, b.headerErr
}

func (b *capacityBackend) Multicall(_ context.Context, calls []chain.Call) ([]chain.CallResult, error) {
	return b.results(calls), nil
}

func (b *capacityBackend) MulticallAtHash(_ context.Context, calls []chain.Call, hash common.Hash) ([]chain.CallResult, error) {
	if hash != b.header.Hash() {
		b.t.Errorf("batch used block %s, want %s", hash, b.header.Hash())
	}
	b.pinnedCalls++
	if b.failPhase == b.pinnedCalls {
		return nil, errors.New("RPC unavailable")
	}
	results := b.results(calls)
	if b.shortPhase == b.pinnedCalls {
		results = results[:len(results)-1]
	}
	return results, nil
}

func (b *capacityBackend) results(calls []chain.Call) []chain.CallResult {
	out := make([]chain.CallResult, len(calls))
	for i, call := range calls {
		if call.Target == b.lens {
			b.lensCalls++
		}
		value, ok := b.values[capacityCallKey(call.Target, call.Data)]
		if !ok {
			b.t.Errorf("unexpected call to %s: %x", call.Target, call.Data)
		}
		if call.Target == b.lens && value.Success && b.onLens != nil {
			b.onLens()
		}
		out[i] = value
	}
	return out
}

func capacityCallKey(target common.Address, data []byte) string {
	return target.Hex() + common.Bytes2Hex(data)
}

func capacityFixture(t *testing.T, sameMaker bool, adapterIndex byte) (*Reader, *capacityBackend, []Route) {
	t.Helper()
	route := testReaderRoute(adapterIndex)
	routes := []Route{route, NewRoute(11155111, route.Adapter, route.Vault,
		common.HexToAddress("0x2222"), route.TokenOut, route.TokenInDecimals, route.TokenOutDecimals)}
	lensAddress := common.HexToAddress("0x9999")
	owner, maker := common.HexToAddress("0x1111"), common.HexToAddress("0x3333")
	if sameMaker {
		maker = owner
	}
	b := &capacityBackend{t: t, header: &types.Header{Number: big.NewInt(123)},
		values: make(map[string]chain.CallResult), lens: lensAddress}
	set := func(target common.Address, data []byte, result chain.CallResult) {
		b.values[capacityCallKey(target, data)] = result
	}
	set(route.Adapter, llAdapter.PackOwner(), successOutput(t, "owner", owner))
	set(route.Adapter, llAdapter.PackMarketMaker(), successOutput(t, "marketMaker", maker))
	set(lensAddress, lensB.PackGetMaxAssets(route.Adapter), successOutput(t, "getMaxAssets", big.NewInt(100)))
	for i, item := range routes {
		account := common.BigToAddress(big.NewInt(int64(900 + i)))
		set(item.Adapter, llAdapter.PackAccounts(item.TokenIn), successOutput(t, "accounts", account))
		set(item.Adapter, llAdapter.PackAcquireBalance(item.TokenIn, owner), successOutput(t, "acquireBalance", big.NewInt(5)))
		if !sameMaker {
			set(item.Adapter, llAdapter.PackAcquireBalance(item.TokenIn, maker), successOutput(t, "acquireBalance", big.NewInt(7)))
		}
		set(item.Adapter, llAdapter.PackLimit(item.TokenIn), successOutput(t, "limit", big.NewInt(int64(80-i*70))))
		// Account and ERC4626 totalAssets() have identical uint256 wire formats.
		set(account, erc4626b.PackTotalAssets(), successOutput(t, "totalAssets", big.NewInt(30)))
		set(item.Adapter, llAdapter.PackPaused(), successOutput(t, "paused", false))
		set(item.Adapter, llAdapter.PackGetMaxRate(item.TokenIn), successOutput(t, "getMaxRate", big.NewInt(1_000_000_000_000_000_000)))
		set(item.Adapter, llAdapter.PackMinDiscount(item.TokenIn), successOutput(t, "minDiscount", new(big.Int)))
		probe, _ := OraclePriceProbe(item.TokenInDecimals, item.TokenOutDecimals)
		set(item.Adapter, llAdapter.PackGetAmountOut(item.TokenIn, probe), successOutput(t, "getAmountOut", big.NewInt(1_000_000_000_000_000_000)))
		// Let the old path return valid results, so the regression fails on repeated execution.
		set(lensAddress, lensB.PackGetMaxAssets0(item.Adapter, item.TokenIn), successOutput(t, "getMaxAssets", big.NewInt(12)))
	}
	return &Reader{chain: b, log: logr.Discard(), lens: lensAddress}, b, routes
}

func TestInventoryReusesAdapterCapacityAtOneBlock(t *testing.T) {
	for _, sameMaker := range []bool{false, true} {
		r, backend, routes := capacityFixture(t, sameMaker, 1)
		inventory, err := r.ReadInventory(t.Context(), routes)
		if err != nil {
			t.Fatal(err)
		}
		want := []int64{62, 12}
		if sameMaker {
			want = []int64{55, 5}
		}
		if len(inventory) != 2 {
			t.Fatalf("inventory = %+v", inventory)
		}
		for i, item := range inventory {
			if item.MaxAssets.Cmp(big.NewInt(want[i])) != 0 {
				t.Errorf("route %d capacity = %s, want %d", i, item.MaxAssets, want[i])
			}
		}
		if backend.lensCalls != 1 || backend.pinnedCalls != 2 {
			t.Errorf("lens executions = %d, pinned batches = %d; want 1 and 2", backend.lensCalls, backend.pinnedCalls)
		}
	}
}

func TestInventoryCapacityFailuresStayScoped(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*capacityBackend, []Route)
		want []int64
	}{
		{name: "shared failure", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(b.lens, lensB.PackGetMaxAssets(routes[0].Adapter))] = chain.CallResult{}
		}},
		{name: "token limit failure", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(routes[0].Adapter, llAdapter.PackLimit(routes[0].TokenIn))] = chain.CallResult{}
		}, want: []int64{12}},
		{name: "malformed token limit", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(routes[0].Adapter, llAdapter.PackLimit(routes[0].TokenIn))] = chain.CallResult{Success: true, ReturnData: []byte{1}}
		}, want: []int64{12}},
		{name: "account failure", edit: func(b *capacityBackend, _ []Route) {
			b.values[capacityCallKey(common.BigToAddress(big.NewInt(900)), erc4626b.PackTotalAssets())] = chain.CallResult{}
		}, want: []int64{12}},
		{name: "acquired balance failure", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(routes[0].Adapter, llAdapter.PackAcquireBalance(routes[0].TokenIn, common.HexToAddress("0x3333")))] = chain.CallResult{}
		}, want: []int64{12}},
		{name: "malformed shared capacity", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(b.lens, lensB.PackGetMaxAssets(routes[0].Adapter))] = chain.CallResult{Success: true, ReturnData: []byte{1}}
		}},
		{name: "malformed owner", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(routes[0].Adapter, llAdapter.PackOwner())] = chain.CallResult{Success: true, ReturnData: []byte{1}}
		}},
		{name: "zero capacity still permits acquired assets", edit: func(b *capacityBackend, routes []Route) {
			b.values[capacityCallKey(b.lens, lensB.PackGetMaxAssets(routes[0].Adapter))] = successOutput(t, "getMaxAssets", new(big.Int))
			b.values[capacityCallKey(routes[0].Adapter, llAdapter.PackLimit(routes[0].TokenIn))] = chain.CallResult{}
			b.values[capacityCallKey(routes[0].Adapter, llAdapter.PackAccounts(routes[0].TokenIn))] = chain.CallResult{}
		}, want: []int64{12, 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, backend, routes := capacityFixture(t, false, 1)
			tc.edit(backend, routes)
			inventory, err := r.ReadInventory(t.Context(), routes)
			if err != nil || len(inventory) != len(tc.want) {
				t.Fatalf("inventory = %+v, error = %v, want %v", inventory, err, tc.want)
			}
			for i, item := range inventory {
				if !bytes.Equal(item.MaxAssets.Bytes(), big.NewInt(tc.want[i]).Bytes()) {
					t.Errorf("capacity = %s, want %d", item.MaxAssets, tc.want[i])
				}
			}
		})
	}
}

func TestInventoryCapacityFailureDoesNotHideAnotherAdapter(t *testing.T) {
	r, backend, routes := capacityFixture(t, false, 1)
	_, other, otherRoutes := capacityFixture(t, false, 2)
	for key, value := range other.values {
		backend.values[key] = value
	}
	backend.values[capacityCallKey(backend.lens, lensB.PackGetMaxAssets(routes[0].Adapter))] = chain.CallResult{}
	inventory, err := r.ReadInventory(t.Context(), append(routes, otherRoutes...))
	if err != nil || len(inventory) != 2 {
		t.Fatalf("inventory count = %d, error = %v", len(inventory), err)
	}
	for i, item := range inventory {
		if item.ID != otherRoutes[i].ID {
			t.Errorf("returned route %s, want %s", item.ID, otherRoutes[i].ID)
		}
	}
	if backend.lensCalls != 2 {
		t.Errorf("lens executions = %d, want 2", backend.lensCalls)
	}
}

func TestInventoryCapacityUsesPostSweepAccountAssets(t *testing.T) {
	r, backend, routes := capacityFixture(t, false, 1)
	backend.onLens = func() {
		// sweepPending moves realized account cash back to the vault. A pre-sweep
		// read would incorrectly retain the original 30 assets against the token limit.
		backend.values[capacityCallKey(common.BigToAddress(big.NewInt(900)), erc4626b.PackTotalAssets())] = successOutput(t, "totalAssets", new(big.Int))
	}
	inventory, err := r.ReadInventory(t.Context(), routes)
	if err != nil || len(inventory) != 2 || inventory[0].MaxAssets.Cmp(big.NewInt(92)) != 0 {
		t.Fatalf("inventory = %+v, error = %v; want post-sweep capacity 92", inventory, err)
	}
}

func TestInventoryCapacityTransportFailuresAbortSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fail, short int
		headerErr   error
		want        string
	}{
		{name: "head", headerErr: errors.New("head unavailable"), want: "capacity block"},
		{name: "metadata transport", fail: 1, want: "capacity metadata"},
		{name: "metadata incomplete", short: 1, want: "results, want"},
		{name: "inventory transport", fail: 2, want: "capacity inventory"},
		{name: "inventory incomplete", short: 2, want: "results, want"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, backend, routes := capacityFixture(t, false, 1)
			backend.failPhase, backend.shortPhase, backend.headerErr = tc.fail, tc.short, tc.headerErr
			inventory, err := r.ReadInventory(t.Context(), routes)
			if len(inventory) != 0 || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("inventory = %+v, error = %v, want %q", inventory, err, tc.want)
			}
		})
	}
}

func TestTokenCapacityBounds(t *testing.T) {
	maxUint := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for _, tc := range []struct {
		name                          string
		shared, acquired, limit, used *big.Int
		want                          string
	}{
		{name: "shared bound", shared: big.NewInt(40), acquired: big.NewInt(12), limit: big.NewInt(100), used: big.NewInt(30), want: "52"},
		{name: "token bound", shared: big.NewInt(100), acquired: big.NewInt(12), limit: big.NewInt(80), used: big.NewInt(30), want: "62"},
		{name: "saturating subtraction", shared: big.NewInt(100), acquired: big.NewInt(12), limit: big.NewInt(10), used: big.NewInt(30), want: "12"},
		{name: "zero shared short circuit", shared: new(big.Int), acquired: big.NewInt(12), want: "12"},
		{name: "missing shared", acquired: big.NewInt(12)},
		{name: "missing limit", shared: big.NewInt(100), acquired: big.NewInt(12), used: big.NewInt(30)},
		{name: "missing account", shared: big.NewInt(100), acquired: big.NewInt(12), limit: big.NewInt(80)},
		{name: "acquired overflow", shared: new(big.Int), acquired: new(big.Int).Add(maxUint, big.NewInt(1))},
		{name: "sum overflow", shared: big.NewInt(1), acquired: maxUint, limit: big.NewInt(1), used: new(big.Int)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenCapacity(tc.shared, tc.acquired, tc.limit, tc.used)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("capacity = %s, want unresolved", got)
				}
			} else if got == nil || got.String() != tc.want {
				t.Fatalf("capacity = %v, want %s", got, tc.want)
			}
		})
	}
}
