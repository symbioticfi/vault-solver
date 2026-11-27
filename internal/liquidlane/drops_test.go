package liquidlane

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// revertedWith is a failed multicall sub-call carrying the payload of a no-argument custom error.
func revertedWith(t *testing.T, signature string) chain.CallResult {
	t.Helper()
	return chain.CallResult{ReturnData: crypto.Keccak256([]byte(signature))[:4]}
}

func revertedWithReason(t *testing.T, reason string) chain.CallResult {
	t.Helper()
	stringType, err := abi.NewType("string", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := abi.Arguments{{Type: stringType}}.Pack(reason)
	if err != nil {
		t.Fatal(err)
	}
	return chain.CallResult{ReturnData: append(crypto.Keccak256([]byte("Error(string)"))[:4], data...)}
}

// assertDrops checks one drop per route, in order, each naming the route and containing its reason.
func assertDrops(t *testing.T, drops []Drop, routes []Route, reasons ...string) {
	t.Helper()
	if len(drops) != len(routes) || len(reasons) != len(routes) {
		t.Fatalf("drops = %+v, want %d", drops, len(routes))
	}
	for i, route := range routes {
		drop, reason := drops[i], reasons[i]
		if drop.Adapter != route.Adapter || drop.TokenIn != route.TokenIn || !strings.Contains(drop.Reason, reason) {
			t.Fatalf("drop %d = %s, want route %s/%s containing %q", i, drop, route.Adapter.Hex(), route.TokenIn.Hex(), reason)
		}
	}
}

func TestReadFillQuotesDropNamesDecodedRevert(t *testing.T) {
	route := testReaderRoute(1)
	r := &Reader{chain: &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{{
		successOutput(t, "paused", false),
		successOutput(t, "getMaxAssets", big.NewInt(100)),
		revertedWithReason(t, "oracle stale"),
		successOutput(t, "minDiscount", big.NewInt(0)),
	}}}}
	var drops []Drop
	quotes, err := r.ReadFillQuotes(t.Context(), []Route{route}, route.TokenIn, big.NewInt(7), func(d Drop) { drops = append(drops, d) })
	if err != nil || len(quotes) != 0 {
		t.Fatalf("quotes = %+v, error = %v", quotes, err)
	}
	assertDrops(t, drops, []Route{route}, `getAmountOut reverted: "oracle stale"`)
}

func TestReadAuthDropsNameDecodedRevert(t *testing.T) {
	filler := common.HexToAddress("0x0000000000000000000000000000000000000f11")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000b11")
	marketMaker := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	unread, delegated := testReaderRoute(1), testReaderRoute(2)
	r := &Reader{chain: &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "marketMaker", marketMaker), revertedWith(t, "InvalidCaller()"),
			successOutput(t, "marketMaker", marketMaker), successOutput(t, "owner", owner),
		},
		{revertedWith(t, "EnforcedPause()")},
	}}}
	var drops []Drop
	got, err := r.FilterAuthorizedRoutes(t.Context(), []Route{unread, delegated}, filler, func(d Drop) { drops = append(drops, d) })
	if err != nil || len(got) != 0 {
		t.Fatalf("authorized = %+v, error = %v", got, err)
	}
	assertDrops(t, drops, []Route{unread, delegated},
		"authorization: owner reverted: InvalidCaller()", "authorization: isFiller reverted: EnforcedPause()")
}

func TestReadGasSnapshotDropsNameDecodedRevert(t *testing.T) {
	route := testReaderRoute(1)
	owner := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	r := &Reader{chain: &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
		{
			successOutput(t, "owner", owner), successOutput(t, "marketMaker", owner),
			revertedWith(t, "InvalidVault()"), successVaultOutput(t, "withdrawable", big.NewInt(1)),
		},
		{revertedWith(t, "InvalidTokenToRedeem()")},
	}}}
	var drops []Drop
	snapshot, err := r.ReadGasSnapshot(t.Context(), []Route{route}, func(d Drop) { drops = append(drops, d) })
	if err != nil || snapshot.Vaults[route.Vault] != nil || snapshot.Adapters[route.Adapter] == nil {
		t.Fatalf("snapshot = %+v, error = %v", snapshot, err)
	}
	if len(drops) != 2 || !strings.Contains(drops[0].Reason, "freeAssets reverted: InvalidVault()") ||
		drops[1].TokenIn != route.TokenIn || !strings.Contains(drops[1].Reason, "acquireBalance reverted: InvalidTokenToRedeem()") {
		t.Fatalf("drops = %+v", drops)
	}
}

func TestReadAdapterSnapshotErrorNamesDecodedRevert(t *testing.T) {
	route := testReaderRoute(1)
	r := &Reader{
		chain: &scriptedLiquidLaneBackend{latest: [][]chain.CallResult{
			{successOutput(t, "vault", route.Vault)},
			{successAssetOutput(t, route.TokenOut)},
			{successOutput(t, "getTokensToRedeemLength", big.NewInt(1))},
			{successOutput(t, "tokensToRedeem", route.TokenIn)},
			{revertedWith(t, "InvalidInitialization()")},
		}},
		log: logr.Discard(), chainID: 11155111, maxTokensPerAdapter: DefaultMaxTokensPerAdapter,
		dec: fixedDecimals{route.TokenIn: route.TokenInDecimals, route.TokenOut: route.TokenOutDecimals},
	}
	_, err := r.ReadAdapterSnapshot(t.Context(), route.Adapter, common.HexToAddress("0xf11"))
	if err == nil || !strings.Contains(err.Error(), "pause state unresolved: adapter "+route.Adapter.Hex()+": paused reverted: InvalidInitialization()") {
		t.Fatalf("error = %v", err)
	}
}

func TestSummarizeDropsBoundsTheReason(t *testing.T) {
	t.Parallel()
	if got := SummarizeDrops(nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
	drops := make([]Drop, 7)
	for i := range drops {
		drops[i] = Drop{Adapter: common.BigToAddress(big.NewInt(int64(i + 1))), Reason: "adapter is paused"}
	}
	got := SummarizeDrops(drops)
	if !strings.HasPrefix(got, "7 dropped reads: adapter 0x0000000000000000000000000000000000000001: adapter is paused; ") ||
		!strings.HasSuffix(got, "; 2 more") || strings.Count(got, "adapter is paused") != 5 {
		t.Fatalf("summary = %q", got)
	}
}
