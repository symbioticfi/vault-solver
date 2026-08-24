package liquidlane

import (
	"context"
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

type scriptedLiquidLaneBackend struct {
	latest [][]chain.CallResult
}

func (b *scriptedLiquidLaneBackend) ChainID() *big.Int { return big.NewInt(11155111) }

func (b *scriptedLiquidLaneBackend) Multicall(_ context.Context, _ []chain.Call) ([]chain.CallResult, error) {
	result := b.latest[0]
	b.latest = b.latest[1:]
	return result, nil
}

type fixedDecimals map[common.Address]int

func (d fixedDecimals) Get(_ context.Context, token common.Address) (int, error) {
	return d[token], nil
}

type failingDecimals struct {
	err error
}

func (d failingDecimals) Get(_ context.Context, _ common.Address) (int, error) {
	return 0, d.err
}

type selectiveDecimals struct {
	values fixedDecimals
	token  common.Address
	err    error
}

func (d selectiveDecimals) Get(_ context.Context, token common.Address) (int, error) {
	if token == d.token {
		return 0, d.err
	}
	return d.values[token], nil
}

func TestReaderResolveAdaptersFailsClosedForConfiguredAdapterMetadata(t *testing.T) {
	route := testReaderRoute(1)
	decimalsErr := errors.New("temporary decimals failure")
	tests := map[string]struct {
		results [][]chain.CallResult
		dec     decimalsReader
		want    string
	}{
		"vault call": {
			results: [][]chain.CallResult{{{}}},
			dec:     fixedDecimals{},
			want:    "vault: call failed",
		},
		"vault decode": {
			results: [][]chain.CallResult{{{Success: true, ReturnData: []byte{0xff}}}},
			dec:     fixedDecimals{},
			want:    "vault:",
		},
		"zero vault": {
			results: [][]chain.CallResult{{successOutput(t, "vault", common.Address{})}},
			dec:     fixedDecimals{},
			want:    "vault: zero address",
		},
		"asset call": {
			results: [][]chain.CallResult{
				{successOutput(t, "vault", route.Vault)},
				{{}},
			},
			dec:  fixedDecimals{},
			want: "asset: call failed",
		},
		"asset decode": {
			results: [][]chain.CallResult{
				{successOutput(t, "vault", route.Vault)},
				{{Success: true, ReturnData: []byte{0xff}}},
			},
			dec:  fixedDecimals{},
			want: "asset:",
		},
		"zero asset": {
			results: [][]chain.CallResult{
				{successOutput(t, "vault", route.Vault)},
				{successAssetOutput(t, common.Address{})},
			},
			dec:  fixedDecimals{},
			want: "asset: zero address",
		},
		"decimals": {
			results: [][]chain.CallResult{
				{successOutput(t, "vault", route.Vault)},
				{successAssetOutput(t, route.TokenOut)},
			},
			dec:  failingDecimals{err: decimalsErr},
			want: "decimals",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Reader{
				chain:               &scriptedLiquidLaneBackend{latest: test.results},
				log:                 logr.Discard(),
				dec:                 test.dec,
				chainID:             11155111,
				maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
			}

			_, err := r.ResolveAdapters(context.Background(), []common.Address{route.Adapter})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveAdapters error = %v, want %q", err, test.want)
			}
			if name == "decimals" && !errors.Is(err, decimalsErr) {
				t.Fatalf("ResolveAdapters error = %v, want wrapped decimals error", err)
			}
		})
	}
}

func TestReaderResolveRoutesFailsClosedForConfiguredAdapterRoutes(t *testing.T) {
	route := testReaderRoute(1)
	decimalsErr := errors.New("temporary token decimals failure")
	baseResults := func(extra ...[]chain.CallResult) [][]chain.CallResult {
		results := [][]chain.CallResult{
			{successOutput(t, "vault", route.Vault)},
			{successAssetOutput(t, route.TokenOut)},
		}
		return append(results, extra...)
	}
	tests := map[string]struct {
		results [][]chain.CallResult
		dec     decimalsReader
		want    string
	}{
		"length call": {
			results: baseResults([]chain.CallResult{{}}),
			dec:     fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want:    "tokensToRedeem length: call failed",
		},
		"length decode": {
			results: baseResults([]chain.CallResult{{Success: true, ReturnData: []byte{0xff}}}),
			dec:     fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want:    "tokensToRedeem length:",
		},
		"empty route list": {
			results: baseResults([]chain.CallResult{successOutput(t, "getTokensToRedeemLength", new(big.Int))}),
			dec:     fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want:    "tokensToRedeem is empty",
		},
		"route cap": {
			results: baseResults([]chain.CallResult{successOutput(
				t,
				"getTokensToRedeemLength",
				big.NewInt(DefaultMaxTokensPerAdapter+1),
			)}),
			dec:  fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want: "exceeds cap",
		},
		"token call": {
			results: baseResults(
				[]chain.CallResult{successOutput(t, "getTokensToRedeemLength", big.NewInt(1))},
				[]chain.CallResult{{}},
			),
			dec:  fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want: "tokensToRedeem[0]: call failed",
		},
		"token decode": {
			results: baseResults(
				[]chain.CallResult{successOutput(t, "getTokensToRedeemLength", big.NewInt(1))},
				[]chain.CallResult{{Success: true, ReturnData: []byte{0xff}}},
			),
			dec:  fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want: "tokensToRedeem[0]:",
		},
		"zero token": {
			results: baseResults(
				[]chain.CallResult{successOutput(t, "getTokensToRedeemLength", big.NewInt(1))},
				[]chain.CallResult{successOutput(t, "tokensToRedeem", common.Address{})},
			),
			dec:  fixedDecimals{route.TokenOut: route.TokenOutDecimals},
			want: "tokensToRedeem[0]: zero address",
		},
		"token decimals": {
			results: baseResults(
				[]chain.CallResult{successOutput(t, "getTokensToRedeemLength", big.NewInt(1))},
				[]chain.CallResult{successOutput(t, "tokensToRedeem", route.TokenIn)},
			),
			dec: selectiveDecimals{
				values: fixedDecimals{route.TokenOut: route.TokenOutDecimals},
				token:  route.TokenIn,
				err:    decimalsErr,
			},
			want: "tokenIn " + route.TokenIn.Hex() + " decimals",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Reader{
				chain:               &scriptedLiquidLaneBackend{latest: test.results},
				log:                 logr.Discard(),
				dec:                 test.dec,
				chainID:             11155111,
				maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
			}

			_, err := r.ResolveRoutes(t.Context(), []common.Address{route.Adapter})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveRoutes error = %v, want %q", err, test.want)
			}
			if name == "token decimals" && !errors.Is(err, decimalsErr) {
				t.Fatalf("ResolveRoutes error = %v, want wrapped decimals error", err)
			}
		})
	}
}

func TestReaderReadInventoryUsesLatestAndFailsClosedPerRoute(t *testing.T) {
	backend := &scriptedLiquidLaneBackend{
		latest: [][]chain.CallResult{{
			successOutput(t, "paused", false),
			successOutput(t, "getMaxAssets", big.NewInt(100)),
			successOutput(t, "getMaxRate", big.NewInt(1_000_000_000_000_000_000)),
			successOutput(t, "minDiscount", big.NewInt(100_000)),
			{Success: true, ReturnData: []byte{0xff}},
			successOutput(t, "getMaxAssets", big.NewInt(200)),
			successOutput(t, "getMaxRate", big.NewInt(1_000_000_000_000_000_000)),
			successOutput(t, "minDiscount", big.NewInt(100_000)),
		}},
	}
	r := &Reader{
		chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111,
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
	}
	routes := []Route{testReaderRoute(1), testReaderRoute(2)}

	inventory, err := r.ReadInventory(context.Background(), routes)
	if err != nil {
		t.Fatalf("ReadInventory: %v", err)
	}
	if len(inventory) != 1 || inventory[0].ID != routes[0].ID {
		t.Fatalf("inventory = %+v", inventory)
	}
	if inventory[0].MaxRate.String() != "1000000000000000000" ||
		inventory[0].AdapterMinDiscount.String() != "100000" {
		t.Fatalf("executable inventory = %+v", inventory[0])
	}
}

func TestReaderReadFillQuotesFiltersTokenAtLatest(t *testing.T) {
	tokenIn := common.HexToAddress("0x0000000000000000000000000000000000000101")
	backend := &scriptedLiquidLaneBackend{
		latest: [][]chain.CallResult{{
			successOutput(t, "paused", false),
			successOutput(t, "getMaxAssets", big.NewInt(1_000)),
			successOutput(t, "getAmountOut", big.NewInt(900)),
			successOutput(t, "minDiscount", big.NewInt(100_000)),
		}},
	}
	r := &Reader{
		chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111,
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
	}
	matching := testReaderRoute(1)
	matching.TokenIn = tokenIn
	nonMatching := testReaderRoute(2)
	amountIn := big.NewInt(500)

	quotes, err := r.ReadFillQuotes(context.Background(), []Route{matching, nonMatching}, tokenIn, amountIn)
	if err != nil {
		t.Fatalf("ReadFillQuotes: %v", err)
	}
	if len(quotes) != 1 || quotes[0].GrossAmountOut.String() != "900" ||
		quotes[0].MaxAmountOut.String() != "810" || quotes[0].MinDiscount.String() != "100000" ||
		quotes[0].MaxRate.String() != "1620000000000000000000000000000" {
		t.Fatalf("quotes = %+v", quotes)
	}
	amountIn.SetInt64(1)
	if quotes[0].AmountIn.String() != "500" {
		t.Fatalf("amountIn was not cloned: %s", quotes[0].AmountIn)
	}
}

func TestReaderReadFillQuotesKeepsAmountSpecificFillWhenRateRoundsToZero(t *testing.T) {
	tokenIn := common.HexToAddress("0x0000000000000000000000000000000000000101")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{{
		successOutput(t, "paused", false),
		successOutput(t, "getMaxAssets", big.NewInt(1)),
		successOutput(t, "getAmountOut", big.NewInt(1)),
		successOutput(t, "minDiscount", big.NewInt(0)),
	}}}
	r := &Reader{
		chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111,
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
	}
	route := testReaderRoute(1)
	route.TokenIn = tokenIn
	amountIn := new(big.Int).Exp(big.NewInt(10), big.NewInt(37), nil)

	quotes, err := r.ReadFillQuotes(context.Background(), []Route{route}, tokenIn, amountIn)
	if err != nil {
		t.Fatalf("ReadFillQuotes: %v", err)
	}
	if len(quotes) != 1 || quotes[0].MaxAmountOut.String() != "1" || quotes[0].MaxRate.Sign() != 0 {
		t.Fatalf("quotes = %+v", quotes)
	}
}

func TestReaderReadGasSnapshotCombinesAcquireAndDeduplicatesVaultState(t *testing.T) {
	route := testReaderRoute(1)
	secondRoute := testReaderRoute(2)
	secondRoute.Vault = route.Vault
	secondRoute.CapacityID = route.CapacityID
	owner := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "owner", owner),
			successOutput(t, "marketMaker", marketMaker),
			successOutput(t, "owner", owner),
			successOutput(t, "marketMaker", marketMaker),
			successVaultOutput(t, "freeAssets", big.NewInt(200)),
			successVaultOutput(t, "withdrawable", big.NewInt(150)),
		},
		{
			successOutput(t, "acquireBalance", big.NewInt(30)),
			successOutput(t, "acquireBalance", big.NewInt(70)),
			successOutput(t, "acquireBalance", big.NewInt(20)),
			successOutput(t, "acquireBalance", big.NewInt(10)),
		},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}
	snapshot, err := r.ReadGasSnapshot(context.Background(), []Route{route, secondRoute})
	if err != nil {
		t.Fatalf("ReadGasSnapshot: %v", err)
	}
	if len(snapshot.Vaults) != 1 || snapshot.Vaults[route.Vault].FreeAssets.String() != "200" ||
		snapshot.Vaults[route.Vault].Withdrawable.String() != "150" {
		t.Fatalf("gas vault state = %+v", snapshot.Vaults)
	}
	if snapshot.Adapters[route.Adapter].Acquire[route.TokenIn].String() != "100" ||
		snapshot.Adapters[secondRoute.Adapter].Acquire[secondRoute.TokenIn].String() != "30" {
		t.Fatalf("gas adapter state = %+v", snapshot.Adapters)
	}
}

func TestReaderReadGasSnapshotReadsZeroMarketMakerKey(t *testing.T) {
	route := testReaderRoute(1)
	owner := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "owner", owner),
			successOutput(t, "marketMaker", common.Address{}),
			successVaultOutput(t, "freeAssets", big.NewInt(200)),
			successVaultOutput(t, "withdrawable", big.NewInt(150)),
		},
		{
			successOutput(t, "acquireBalance", big.NewInt(30)),
			successOutput(t, "acquireBalance", new(big.Int)),
		},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

	snapshot, err := r.ReadGasSnapshot(t.Context(), []Route{route})
	if err != nil {
		t.Fatalf("ReadGasSnapshot: %v", err)
	}
	if got := snapshot.Adapters[route.Adapter].Acquire[route.TokenIn]; got == nil || got.String() != "30" {
		t.Fatalf("acquire balance = %v, want 30", got)
	}
}

func TestReaderReadGasSnapshotTreatsInvalidAcquireBalanceAsUnavailable(t *testing.T) {
	route := testReaderRoute(1)
	owner := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	tests := map[string]chain.CallResult{
		"failed call":      {},
		"malformed result": {Success: true, ReturnData: []byte{0xff}},
	}
	for name, acquireResult := range tests {
		t.Run(name, func(t *testing.T) {
			backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
				{
					successOutput(t, "owner", owner),
					successOutput(t, "marketMaker", owner),
					successVaultOutput(t, "freeAssets", big.NewInt(200)),
					successVaultOutput(t, "withdrawable", big.NewInt(150)),
				},
				{acquireResult},
			}}
			r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

			snapshot, err := r.ReadGasSnapshot(context.Background(), []Route{route})
			if err != nil {
				t.Fatalf("ReadGasSnapshot: %v", err)
			}
			if amount := snapshot.Adapters[route.Adapter].Acquire[route.TokenIn]; amount != nil {
				t.Fatalf("acquire balance = %v, want unavailable", amount)
			}
		})
	}
}

func TestReaderReadAdapterSnapshotCombinesSharedFacts(t *testing.T) {
	route := testReaderRoute(1)
	owner := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{successOutput(t, "vault", route.Vault)},
		{successAssetOutput(t, route.TokenOut)},
		{successOutput(t, "getTokensToRedeemLength", big.NewInt(1))},
		{successOutput(t, "tokensToRedeem", route.TokenIn)},
		{successOutput(t, "paused", false)},
		{successOutput(t, "marketMaker", owner), successOutput(t, "owner", owner)},
		{
			successOutput(t, "owner", owner), successOutput(t, "marketMaker", owner),
			successVaultOutput(t, "freeAssets", big.NewInt(200)),
			successVaultOutput(t, "withdrawable", big.NewInt(150)),
		},
		{successOutput(t, "acquireBalance", big.NewInt(30))},
		{
			successOutput(t, "paused", false),
			successOutput(t, "getMaxAssets", big.NewInt(120)),
			successOutput(t, "getMaxRate", big.NewInt(900)),
			successOutput(t, "minDiscount", big.NewInt(100_000)),
		},
	}}
	r := &Reader{
		chain: backend, log: logr.Discard(), chainID: 11155111,
		dec:                 fixedDecimals{route.TokenIn: route.TokenInDecimals, route.TokenOut: route.TokenOutDecimals},
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
	}

	snapshot, err := r.ReadAdapterSnapshot(context.Background(), route.Adapter, owner)
	if err != nil {
		t.Fatalf("ReadAdapterSnapshot: %v", err)
	}
	if !snapshot.Authorized || snapshot.Paused || snapshot.Vault != route.Vault || snapshot.TokenOut != route.TokenOut {
		t.Fatalf("adapter snapshot = %+v", snapshot)
	}
	if snapshot.FreeAssets.String() != "200" || snapshot.Withdrawable.String() != "150" || len(snapshot.Routes) != 1 {
		t.Fatalf("adapter liquidity = %+v", snapshot)
	}
	gotRoute := snapshot.Routes[0]
	if gotRoute.MaxAssets.String() != "120" || gotRoute.MaxRate.String() != "900" ||
		gotRoute.AcquireBalance.String() != "30" {
		t.Fatalf("route snapshot = %+v", gotRoute)
	}
}

func TestReaderReadAdapterSnapshotKeepsZeroCapacityRoutes(t *testing.T) {
	first := testReaderRoute(1)
	second := testReaderRoute(2)
	owner := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{successOutput(t, "vault", first.Vault)},
		{successAssetOutput(t, first.TokenOut)},
		{successOutput(t, "getTokensToRedeemLength", big.NewInt(2))},
		{
			successOutput(t, "tokensToRedeem", first.TokenIn),
			successOutput(t, "tokensToRedeem", second.TokenIn),
		},
		{successOutput(t, "paused", false)},
		{successOutput(t, "marketMaker", owner), successOutput(t, "owner", owner)},
		{
			successOutput(t, "owner", owner), successOutput(t, "marketMaker", owner),
			successVaultOutput(t, "freeAssets", big.NewInt(200)),
			successVaultOutput(t, "withdrawable", big.NewInt(150)),
		},
		{
			successOutput(t, "acquireBalance", big.NewInt(0)),
			successOutput(t, "acquireBalance", big.NewInt(30)),
		},
		{
			successOutput(t, "paused", false),
			successOutput(t, "getMaxAssets", big.NewInt(0)),
			successOutput(t, "getMaxRate", big.NewInt(900)),
			successOutput(t, "minDiscount", big.NewInt(100_000)),
			successOutput(t, "paused", false),
			successOutput(t, "getMaxAssets", big.NewInt(120)),
			successOutput(t, "getMaxRate", big.NewInt(800)),
			successOutput(t, "minDiscount", big.NewInt(100_000)),
		},
	}}
	r := &Reader{
		chain: backend, log: logr.Discard(), chainID: 11155111,
		dec: fixedDecimals{
			first.TokenIn: first.TokenInDecimals, second.TokenIn: second.TokenInDecimals,
			first.TokenOut: first.TokenOutDecimals,
		},
		maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
	}

	snapshot, err := r.ReadAdapterSnapshot(context.Background(), first.Adapter, owner)
	if err != nil {
		t.Fatalf("ReadAdapterSnapshot: %v", err)
	}
	if len(snapshot.Routes) != 2 {
		t.Fatalf("routes = %+v", snapshot.Routes)
	}
	if snapshot.Routes[0].MaxAssets == nil || snapshot.Routes[0].MaxAssets.Sign() != 0 ||
		snapshot.Routes[0].MaxRate == nil || snapshot.Routes[0].MaxRate.String() != "900" {
		t.Fatalf("zero-cap route = %+v", snapshot.Routes[0])
	}
	if snapshot.Routes[1].MaxAssets.String() != "120" || snapshot.Routes[1].MaxRate.String() != "800" {
		t.Fatalf("healthy route = %+v", snapshot.Routes[1])
	}
}

func TestReaderReadAuthUsesDirectRolesAndDelegatedFiller(t *testing.T) {
	filler := common.HexToAddress("0x0000000000000000000000000000000000000f11")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	adapters := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000011"),
		common.HexToAddress("0x0000000000000000000000000000000000000012"),
		common.HexToAddress("0x0000000000000000000000000000000000000013"),
	}
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "marketMaker", filler), successOutput(t, "owner", owner),
			successOutput(t, "marketMaker", marketMaker), successOutput(t, "owner", filler),
			successOutput(t, "marketMaker", marketMaker), successOutput(t, "owner", owner),
		},
		{successOutput(t, "isFiller", true)},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

	auth, err := r.ReadAuth(context.Background(), adapters, filler)
	if err != nil {
		t.Fatalf("ReadAuth: %v", err)
	}
	if len(auth) != 3 || !auth[0].Authorized || !auth[1].Authorized || !auth[2].Authorized || !auth[2].IsFiller {
		t.Fatalf("auth = %+v", auth)
	}
}

func TestReaderFilterAuthorizedAdapterAddressesDoesNotRequireTokenRoutes(t *testing.T) {
	filler := common.HexToAddress("0x0000000000000000000000000000000000000f11")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	adapters := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000011"),
		common.HexToAddress("0x0000000000000000000000000000000000000012"),
	}
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "marketMaker", filler), successOutput(t, "owner", owner),
			successOutput(t, "marketMaker", marketMaker), successOutput(t, "owner", owner),
		},
		{successOutput(t, "isFiller", true)},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

	got, err := r.FilterAuthorizedAdapterAddresses(t.Context(), adapters, filler)
	if err != nil {
		t.Fatalf("FilterAuthorizedAdapterAddresses: %v", err)
	}
	if !slices.Equal(got, adapters) {
		t.Fatalf("authorized adapters = %v, want %v", got, adapters)
	}
}

func TestReaderReadAuthAcceptsDelegatedFillerForZeroMarketMaker(t *testing.T) {
	filler := common.HexToAddress("0x0000000000000000000000000000000000000f11")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	adapterAddress := common.HexToAddress("0x0000000000000000000000000000000000000011")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{successOutput(t, "marketMaker", common.Address{}), successOutput(t, "owner", owner)},
		{successOutput(t, "isFiller", true)},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

	auth, err := r.ReadAuth(t.Context(), []common.Address{adapterAddress}, filler)
	if err != nil {
		t.Fatalf("ReadAuth: %v", err)
	}
	if len(auth) != 1 || auth[0].MarketMaker != (common.Address{}) || auth[0].Owner != owner ||
		!auth[0].Authorized || !auth[0].IsFiller {
		t.Fatalf("auth = %+v", auth)
	}
}

func TestReaderReadAuthRejectsIncompleteFillerResults(t *testing.T) {
	filler := common.HexToAddress("0x0000000000000000000000000000000000000f11")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	adapterAddress := common.HexToAddress("0x0000000000000000000000000000000000000011")
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{successOutput(t, "marketMaker", marketMaker), successOutput(t, "owner", owner)},
		{},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

	if _, err := r.ReadAuth(context.Background(), []common.Address{adapterAddress}, filler); err == nil {
		t.Fatal("expected incomplete filler multicall error")
	}
}

func TestReaderFilterAuthorizedRoutesDropsUnauthorizedAdapters(t *testing.T) {
	filler := common.HexToAddress("0x0000000000000000000000000000000000000f11")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	routes := []Route{testReaderRoute(1), testReaderRoute(2)}
	backend := &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "marketMaker", filler), successOutput(t, "owner", owner),
			successOutput(t, "marketMaker", marketMaker), successOutput(t, "owner", owner),
		},
		{successOutput(t, "isFiller", false)},
	}}
	r := &Reader{chain: backend, log: logr.Discard(), dec: fixedDecimals{}, chainID: 11155111}

	got, err := r.FilterAuthorizedRoutes(context.Background(), routes, filler)
	if err != nil {
		t.Fatalf("FilterAuthorizedRoutes: %v", err)
	}
	if len(got) != 1 || got[0].ID != routes[0].ID {
		t.Fatalf("authorized routes = %+v", got)
	}
}

func testReaderRoute(index byte) Route {
	return NewRoute(
		11155111,
		common.BytesToAddress([]byte{index}),
		common.BytesToAddress([]byte{index + 10}),
		common.BytesToAddress([]byte{index + 20}),
		common.BytesToAddress([]byte{index + 30}),
		18,
		6,
	)
}

func successOutput(t *testing.T, method string, values ...any) chain.CallResult {
	t.Helper()
	parsed, err := adapter.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse adapter ABI: %v", err)
	}
	data, err := packMethodOutput(parsed, method, values...)
	if err != nil {
		t.Fatalf("pack %s output: %v", method, err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func successVaultOutput(t *testing.T, method string, values ...any) chain.CallResult {
	t.Helper()
	parsed, err := vaultv2.IVaultV2MetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse vault ABI: %v", err)
	}
	data, err := packMethodOutput(parsed, method, values...)
	if err != nil {
		t.Fatalf("pack %s output: %v", method, err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func successAssetOutput(t *testing.T, asset common.Address) chain.CallResult {
	t.Helper()
	parsed, err := erc4626.IERC4626MetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse ERC4626 ABI: %v", err)
	}
	data, err := packMethodOutput(parsed, "asset", asset)
	if err != nil {
		t.Fatalf("pack asset output: %v", err)
	}
	return chain.CallResult{Success: true, ReturnData: data}
}

func packMethodOutput(parsed *abi.ABI, method string, values ...any) ([]byte, error) {
	return parsed.Methods[method].Outputs.Pack(values...)
}
