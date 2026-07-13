package bridgefacilitator

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// TestCollectRequests covers the enumeration-prefix logic: the adapter's requests[] is dense (kept so
// by finalizeRequest's swap-pop), and indices past the end revert, so collectRequests must take the
// leading run of decodable successes and stop at the first gap.
func TestCollectRequests(t *testing.T) {
	t.Parallel()

	a0 := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	a1 := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	a2 := common.HexToAddress("0x00000000000000000000000000000000000000A2")
	ok := func(addr common.Address) chain.CallResult {
		return chain.CallResult{Success: true, ReturnData: abiEncodeAddress(t, addr)}
	}
	fail := chain.CallResult{Success: false}
	bad := chain.CallResult{Success: true, ReturnData: []byte{0x01}} // undecodable as an address

	tests := []struct {
		name string
		res  []chain.CallResult
		want []common.Address
	}{
		{"empty", nil, nil},
		{"all active", []chain.CallResult{ok(a0), ok(a1), ok(a2)}, []common.Address{a0, a1, a2}},
		{"prefix then end-of-array gap", []chain.CallResult{ok(a0), ok(a1), fail, ok(a2)}, []common.Address{a0, a1}},
		{"first slot reverts", []chain.CallResult{fail, ok(a0)}, nil},
		{"undecodable slot ends the set", []chain.CallResult{ok(a0), bad, ok(a1)}, []common.Address{a0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := collectRequests(tc.res)
			if len(got) != len(tc.want) {
				t.Fatalf("collectRequests = %v (len %d), want %v (len %d)", got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %s, want %s", i, got[i].Hex(), tc.want[i].Hex())
				}
			}
		})
	}
}

func TestDecodeAddr_RejectsZeroAddress(t *testing.T) {
	t.Parallel()

	_, err := decodeAddr(
		chain.CallResult{Success: true, ReturnData: abiEncodeAddress(t, common.Address{})},
		bfAdapter.UnpackVault,
		"adapter.vault()",
	)
	if err == nil {
		t.Fatal("expected a zero address to fail validation")
	}
}

// TestPpmToBps covers the ceil(ppm/100) conversion of minYieldPerRequest (ppm) to the bps the pre-screen
// compares against the auction maxRate — rounded up so the bot never bids below the on-chain floor.
func TestPpmToBps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ppm, want int64
	}{
		{0, 0}, {1, 1}, {99, 1}, {100, 1}, {150, 2}, {10_000, 100}, {1_000_000, 10_000},
	}
	for _, tc := range tests {
		if got := ppmToBps(big.NewInt(tc.ppm)).Int64(); got != tc.want {
			t.Errorf("ppmToBps(%d) = %d, want %d", tc.ppm, got, tc.want)
		}
	}
}

// newMulticallFakeClient returns a chain.Client backed by a minimal JSON-RPC httptest server.
// The server responds to eth_chainId and eth_call; ethCallReplies are the hex-encoded bytes returned
// by successive eth_call requests (i.e. each ABI-encoded Multicall3.aggregate3 Result[] array). With
// one reply it serves that every call; with several it serves them in order (e.g. round 1 then round
// 2 of resolveAdapters), sticking on the last once exhausted.
func newMulticallFakeClient(t *testing.T, ethCallReplies ...[]byte) (*chain.Client, func()) {
	t.Helper()
	multicallAddr := common.HexToAddress("0x0000000000000000000000000000000000000001")
	var ethCallN atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "eth_chainId":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"result":"0x1"}`, marshalID(req.ID))
		case "eth_call":
			i := int(ethCallN.Add(1)) - 1
			if i >= len(ethCallReplies) {
				i = len(ethCallReplies) - 1
			}
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"result":"0x%x"}`, marshalID(req.ID), ethCallReplies[i])
		default:
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"error":{"code":-32601,"message":"method not found"}}`, marshalID(req.ID))
		}
	}))

	c, err := chain.Dial(t.Context(), []string{srv.URL}, "", multicallAddr.Hex(), logr.Discard())
	if err != nil {
		srv.Close()
		t.Fatalf("chain.Dial: %v", err)
	}
	return c, srv.Close
}

// marshalID renders a JSON-RPC request id (number or string) back to JSON so we can embed it in the
// response without re-encoding quotes. json.Marshal on an any holding a json.Number or string is
// always safe; if it somehow fails we fall back to a literal null which keeps the server response
// well-formed for the client.
func marshalID(id any) string {
	b, err := json.Marshal(id)
	if err != nil {
		return "null"
	}
	return string(b)
}

// abiEncodeAggregate3Results ABI-encodes a Multicall3.aggregate3 return value: one Result per inner
// payload, each Success=true with ReturnData=inner. This is the hex payload eth_call returns for a
// successful aggregate3 with len(inners) sub-call results.
func abiEncodeAggregate3Results(t *testing.T, inners ...[]byte) []byte {
	t.Helper()
	// aggregate3 returns (Result[] returnData) where Result = (bool success, bytes returnData).
	resultTuple, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{
		{Name: "success", Type: "bool"},
		{Name: "returnData", Type: "bytes"},
	})
	if err != nil {
		t.Fatalf("abi.NewType tuple[]: %v", err)
	}
	type result struct {
		Success    bool
		ReturnData []byte
	}
	results := make([]result, len(inners))
	for i, inner := range inners {
		results[i] = result{Success: true, ReturnData: inner}
	}
	encoded, err := abi.Arguments{{Type: resultTuple}}.Pack(results)
	if err != nil {
		t.Fatalf("abi args.Pack: %v", err)
	}
	return encoded
}

// abiEncodeAddress ABI-encodes a single address as a 32-byte left-padded word (the raw returnData
// for a Solidity function returning address).
func abiEncodeAddress(t *testing.T, addr common.Address) []byte {
	t.Helper()
	addrType, err := abi.NewType("address", "", nil)
	if err != nil {
		t.Fatalf("abi.NewType address: %v", err)
	}
	enc, err := abi.Arguments{{Type: addrType}}.Pack(addr)
	if err != nil {
		t.Fatalf("abi address Pack: %v", err)
	}
	return enc
}

func abiEncodeUint256(t *testing.T, value int64) []byte {
	t.Helper()
	uintType, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatalf("abi.NewType uint256: %v", err)
	}
	enc, err := abi.Arguments{{Type: uintType}}.Pack(big.NewInt(value))
	if err != nil {
		t.Fatalf("abi uint256 Pack: %v", err)
	}
	return enc
}

func TestFactoryAdapters_EmptyRegistry(t *testing.T) {
	t.Parallel()

	round := abiEncodeAggregate3Results(t, abiEncodeUint256(t, 0))
	c, stop := newMulticallFakeClient(t, round)
	defer stop()

	got, err := newReader(c).factoryAdapters(t.Context(), common.HexToAddress("0x00000000000000000000000000000000000000F0"))
	if err != nil {
		t.Fatalf("factoryAdapters: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("factory adapters = %v, want empty", got)
	}
}

func TestFactoryAdapters_EnumeratesEntitiesInRegistryOrder(t *testing.T) {
	t.Parallel()

	want := []common.Address{
		common.HexToAddress("0x00000000000000000000000000000000000000A0"),
		common.HexToAddress("0x00000000000000000000000000000000000000A1"),
		common.HexToAddress("0x00000000000000000000000000000000000000A2"),
	}
	countRound := abiEncodeAggregate3Results(t, abiEncodeUint256(t, int64(len(want))))
	entitiesRound := abiEncodeAggregate3Results(t,
		abiEncodeAddress(t, want[0]), abiEncodeAddress(t, want[1]), abiEncodeAddress(t, want[2]),
	)
	c, stop := newMulticallFakeClient(t, countRound, entitiesRound)
	defer stop()

	got, err := newReader(c).factoryAdapters(t.Context(), common.HexToAddress("0x00000000000000000000000000000000000000F0"))
	if err != nil {
		t.Fatalf("factoryAdapters: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("factory adapters = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("factory adapter %d = %s, want %s", i, got[i].Hex(), want[i].Hex())
		}
	}
}

func TestFactoryAdapters_RejectsEntityCountAboveLimit(t *testing.T) {
	t.Parallel()

	countRound := abiEncodeAggregate3Results(t, abiEncodeUint256(t, int64(maxFactoryEntities+1)))
	c, stop := newMulticallFakeClient(t, countRound)
	defer stop()

	_, err := newReader(c).factoryAdapters(t.Context(), common.HexToAddress("0x00000000000000000000000000000000000000F0"))
	if err == nil {
		t.Fatal("expected an oversized factory registry to be rejected before allocation")
	}
}

// TestResolveAdapters verifies the two-Multicall batch resolves each adapter's vault, signer, and
// collateral and maps them back by index: round 1 returns [vault0, signer0, vault1, signer1] and
// round 2 returns [asset0, asset1], so a layout off-by-one would cross adapters' fields.
func TestResolveAdapters(t *testing.T) {
	t.Parallel()

	adapters := []common.Address{
		common.HexToAddress("0x00000000000000000000000000000000000000A0"),
		common.HexToAddress("0x00000000000000000000000000000000000000A1"),
	}
	vault0 := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer0 := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	asset0 := common.HexToAddress("0x00000000000000000000000000000000000000D0")
	vault1 := common.HexToAddress("0x00000000000000000000000000000000000000B1")
	signer1 := common.HexToAddress("0x00000000000000000000000000000000000000C1")
	asset1 := common.HexToAddress("0x00000000000000000000000000000000000000D1")

	round1 := abiEncodeAggregate3Results(t,
		abiEncodeAddress(t, vault0), abiEncodeAddress(t, signer0),
		abiEncodeAddress(t, vault1), abiEncodeAddress(t, signer1),
	)
	round2 := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset0), abiEncodeAddress(t, asset1))

	c, stop := newMulticallFakeClient(t, round1, round2)
	defer stop()

	got, err := newReader(c).resolveAdapters(context.Background(), adapters)
	if err != nil {
		t.Fatalf("resolveAdapters: %v", err)
	}
	want := []resolvedAdapter{
		{vault: vault0, signer: signer0, collateral: asset0},
		{vault: vault1, signer: signer1, collateral: asset1},
	}
	for i, w := range want {
		if got[i].err != nil {
			t.Fatalf("adapter %d: unexpected err %v", i, got[i].err)
		}
		if got[i].vault != w.vault || got[i].signer != w.signer || got[i].collateral != w.collateral {
			t.Errorf("adapter %d = {vault:%s signer:%s collateral:%s}, want {vault:%s signer:%s collateral:%s}",
				i, got[i].vault.Hex(), got[i].signer.Hex(), got[i].collateral.Hex(),
				w.vault.Hex(), w.signer.Hex(), w.collateral.Hex())
		}
	}
}

func TestResolveAdapters_RejectsUnexpectedMulticallResultCounts(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")

	t.Run("adapter fields", func(t *testing.T) {
		t.Parallel()
		shortRound := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault))
		c, stop := newMulticallFakeClient(t, shortRound)
		defer stop()

		if _, err := newReader(c).resolveAdapters(t.Context(), []common.Address{adapterAddr}); err == nil {
			t.Fatal("expected an error for an incomplete adapter-field response")
		}
	})

	t.Run("assets", func(t *testing.T) {
		t.Parallel()
		fieldsRound := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer))
		emptyAssetRound := abiEncodeAggregate3Results(t)
		c, stop := newMulticallFakeClient(t, fieldsRound, emptyAssetRound)
		defer stop()

		if _, err := newReader(c).resolveAdapters(t.Context(), []common.Address{adapterAddr}); err == nil {
			t.Fatal("expected an error for an incomplete asset response")
		}
	})
}
