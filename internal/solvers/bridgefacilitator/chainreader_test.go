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

// abiEncodeBytes4 ABI-encodes a bytes4 return value (the raw returnData for a Solidity function
// returning bytes4, e.g. ERC-1271 isValidSignature).
func abiEncodeBytes4(t *testing.T, b [4]byte) []byte {
	t.Helper()
	ty, err := abi.NewType("bytes4", "", nil)
	if err != nil {
		t.Fatalf("abi.NewType bytes4: %v", err)
	}
	enc, err := abi.Arguments{{Type: ty}}.Pack(b)
	if err != nil {
		t.Fatalf("abi bytes4 Pack: %v", err)
	}
	return enc
}

// testProbe is any non-empty (hash, sig) pair; the fake client returns canned replies regardless of
// calldata, so its contents don't matter — only the isValidSignature return slots do.
var testProbe = signerProbe{hash: [32]byte{0x01}, sig: []byte{0x02}}

// TestResolveAdapters verifies the two-Multicall batch resolves each adapter's vault, signer, and
// collateral and marks it authorized when isValidSignature returns the ERC-1271 magic value. Round 1
// returns [vault0, signer0, magic0, vault1, signer1, magic1] and round 2 returns [asset0, asset1], so a
// layout off-by-one would cross adapters' fields.
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
		abiEncodeAddress(t, vault0), abiEncodeAddress(t, signer0), abiEncodeBytes4(t, erc1271MagicValue),
		abiEncodeAddress(t, vault1), abiEncodeAddress(t, signer1), abiEncodeBytes4(t, erc1271MagicValue),
	)
	round2 := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset0), abiEncodeAddress(t, asset1))

	c, stop := newMulticallFakeClient(t, round1, round2)
	defer stop()

	got, err := newReader(c).resolveAdapters(context.Background(), adapters, testProbe)
	if err != nil {
		t.Fatalf("resolveAdapters: %v", err)
	}
	want := []resolvedAdapter{
		{vault: vault0, signer: signer0, collateral: asset0, authorized: true},
		{vault: vault1, signer: signer1, collateral: asset1, authorized: true},
	}
	for i, w := range want {
		if got[i].err != nil {
			t.Fatalf("adapter %d: unexpected err %v", i, got[i].err)
		}
		if got[i].vault != w.vault || got[i].signer != w.signer ||
			got[i].collateral != w.collateral || got[i].authorized != w.authorized {
			t.Errorf("adapter %d = {vault:%s signer:%s collateral:%s authorized:%v}, want {vault:%s signer:%s collateral:%s authorized:%v}",
				i, got[i].vault.Hex(), got[i].signer.Hex(), got[i].collateral.Hex(), got[i].authorized,
				w.vault.Hex(), w.signer.Hex(), w.collateral.Hex(), w.authorized)
		}
	}
}

// TestResolveAdaptersDropsUnauthorized verifies an adapter whose isValidSignature returns a non-magic
// value is marked unauthorized and has no collateral read (round 2 only queries the authorized vault).
func TestResolveAdaptersDropsUnauthorized(t *testing.T) {
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

	round1 := abiEncodeAggregate3Results(t,
		abiEncodeAddress(t, vault0), abiEncodeAddress(t, signer0), abiEncodeBytes4(t, erc1271MagicValue),
		abiEncodeAddress(t, vault1), abiEncodeAddress(t, signer1), abiEncodeBytes4(t, [4]byte{0xff, 0xff, 0xff, 0xff}),
	)
	// Only the authorized adapter's vault gets an asset() call in round 2.
	round2 := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset0))

	c, stop := newMulticallFakeClient(t, round1, round2)
	defer stop()

	got, err := newReader(c).resolveAdapters(context.Background(), adapters, testProbe)
	if err != nil {
		t.Fatalf("resolveAdapters: %v", err)
	}
	if got[0].err != nil || !got[0].authorized || got[0].collateral != asset0 {
		t.Errorf("adapter 0 = {authorized:%v collateral:%s err:%v}, want authorized with collateral %s",
			got[0].authorized, got[0].collateral.Hex(), got[0].err, asset0.Hex())
	}
	if got[1].authorized || got[1].collateral != (common.Address{}) {
		t.Errorf("adapter 1 = {authorized:%v collateral:%s}, want unauthorized with no collateral",
			got[1].authorized, got[1].collateral.Hex())
	}
	if got[1].signer != signer1 {
		t.Errorf("adapter 1 signer = %s, want %s (kept for diagnostics)", got[1].signer.Hex(), signer1.Hex())
	}
}
