package bridgefacilitator

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/chain"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

// TestCollectRequests covers best-effort request enumeration: every valid address is retained for safe
// redemption, while any missing, failed, malformed, zero, or extra result makes the snapshot incomplete.
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
		name         string
		res          []chain.CallResult
		expected     int
		want         []common.Address
		wantComplete bool
	}{
		{"empty", nil, 0, nil, true},
		{"all active", []chain.CallResult{ok(a0), ok(a1), ok(a2)}, 3, []common.Address{a0, a1, a2}, true},
		{"failed middle slot retains valid suffix", []chain.CallResult{ok(a0), fail, ok(a2)}, 3, []common.Address{a0, a2}, false},
		{"failed first slot retains valid suffix", []chain.CallResult{fail, ok(a0)}, 2, []common.Address{a0}, false},
		{"undecodable slot retains valid suffix", []chain.CallResult{ok(a0), bad, ok(a1)}, 3, []common.Address{a0, a1}, false},
		{"zero address is malformed", []chain.CallResult{ok(a0), ok(common.Address{}), ok(a1)}, 3, []common.Address{a0, a1}, false},
		{"missing result", []chain.CallResult{ok(a0)}, 2, []common.Address{a0}, false},
		{"extra result is ignored", []chain.CallResult{ok(a0), ok(a1)}, 1, []common.Address{a0}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, complete := collectRequests(tc.res, tc.expected)
			if complete != tc.wantComplete {
				t.Fatalf("complete = %v, want %v", complete, tc.wantComplete)
			}
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

// abiEncodeAggregate3CallResults ABI-encodes a Multicall3.aggregate3 return value.
func abiEncodeAggregate3CallResults(t *testing.T, calls []chain.CallResult) []byte {
	t.Helper()
	// aggregate3 returns (Result[] returnData) where Result = (bool success, bytes returnData).
	resultTuple, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{
		{Name: "success", Type: "bool"},
		{Name: "returnData", Type: "bytes"},
	})
	testcheck.NoError(t, err, "abi.NewType tuple[]: %v")
	type result struct {
		Success    bool
		ReturnData []byte
	}
	results := make([]result, len(calls))
	for i, call := range calls {
		results[i] = result{Success: call.Success, ReturnData: call.ReturnData}
	}
	encoded, err := abi.Arguments{{Type: resultTuple}}.Pack(results)
	testcheck.NoError(t, err, "abi args.Pack: %v")
	return encoded
}

// abiEncodeAggregate3Results is the all-success convenience form used by most reader tests.
func abiEncodeAggregate3Results(t *testing.T, inners ...[]byte) []byte {
	t.Helper()
	results := make([]chain.CallResult, len(inners))
	for i, inner := range inners {
		results[i] = chain.CallResult{Success: true, ReturnData: inner}
	}
	return abiEncodeAggregate3CallResults(t, results)
}

// abiEncodeAddress ABI-encodes a single address as a 32-byte left-padded word (the raw returnData
// for a Solidity function returning address).
func abiEncodeAddress(t *testing.T, addr common.Address) []byte {
	t.Helper()
	addrType, err := abi.NewType("address", "", nil)
	testcheck.NoError(t, err, "abi.NewType address: %v")
	enc, err := abi.Arguments{{Type: addrType}}.Pack(addr)
	testcheck.NoError(t, err, "abi address Pack: %v")
	return enc
}

func abiEncodeUint256(t *testing.T, value int64) []byte {
	t.Helper()
	uintType, err := abi.NewType("uint256", "", nil)
	testcheck.NoError(t, err, "abi.NewType uint256: %v")
	enc, err := abi.Arguments{{Type: uintType}}.Pack(big.NewInt(value))
	testcheck.NoError(t, err, "abi uint256 Pack: %v")
	return enc
}

func abiEncodeBool(t *testing.T, value bool) []byte {
	t.Helper()
	boolType, err := abi.NewType("bool", "", nil)
	testcheck.NoError(t, err, "abi.NewType bool: %v")
	enc, err := abi.Arguments{{Type: boolType}}.Pack(value)
	testcheck.NoError(t, err, "abi bool Pack: %v")
	return enc
}

// abiEncodeBytes4 ABI-encodes a bytes4 return value (the raw returnData for a Solidity function
// returning bytes4, e.g. ERC-1271 isValidSignature).
func abiEncodeBytes4(t *testing.T, b [4]byte) []byte {
	t.Helper()
	ty, err := abi.NewType("bytes4", "", nil)
	testcheck.NoError(t, err, "abi.NewType bytes4: %v")
	enc, err := abi.Arguments{{Type: ty}}.Pack(b)
	testcheck.NoError(t, err, "abi bytes4 Pack: %v")
	return enc
}

func TestFactoryAdapters(t *testing.T) {
	t.Parallel()
	if maxFactoryEntities != 2_000 {
		t.Fatalf("maxFactoryEntities = %d, want 2000", maxFactoryEntities)
	}
	for _, count := range []int{0, 3, 2_000, 2_001} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			t.Parallel()
			rounds := [][]byte{abiEncodeAggregate3Results(t, abiEncodeUint256(t, int64(count)))}
			var want []common.Address
			if count > 0 && count <= 2_000 {
				want = make([]common.Address, count)
				encoded := make([][]byte, count)
				for i := range want {
					want[i] = common.BigToAddress(big.NewInt(int64(i + 1)))
					encoded[i] = abiEncodeAddress(t, want[i])
				}
				rounds = append(rounds, abiEncodeAggregate3Results(t, encoded...))
			}
			c, stop := newMulticallFakeClient(t, rounds...)
			defer stop()
			got, err := newReader(c, common.Address{}).factoryAdapters(t.Context(), common.HexToAddress("0xF0"))
			if count > 2_000 {
				const message = "adapter factory entity count 2001 exceeds safety limit 2000"
				if err == nil || err.Error() != message {
					t.Fatalf("factoryAdapters error = %v, want %q", err, message)
				}
				return
			}
			testcheck.NoError(t, err, "factoryAdapters: %v")
			if !slices.Equal(got, want) {
				t.Fatalf("factory adapters = %v, want %v", got, want)
			}
		})
	}
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

	got, err := newReader(c, common.Address{}).resolveAdapters(context.Background(), adapters, testProbe)
	testcheck.NoError(t, err, "resolveAdapters: %v")
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

		if _, err := newReader(c, common.Address{}).resolveAdapters(t.Context(), []common.Address{adapterAddr}, testProbe); err == nil {
			t.Fatal("expected an error for an incomplete adapter-field response")
		}
	})

	t.Run("assets", func(t *testing.T) {
		t.Parallel()
		fieldsRound := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer), abiEncodeBytes4(t, erc1271MagicValue))
		emptyAssetRound := abiEncodeAggregate3Results(t)
		c, stop := newMulticallFakeClient(t, fieldsRound, emptyAssetRound)
		defer stop()

		if _, err := newReader(c, common.Address{}).resolveAdapters(t.Context(), []common.Address{adapterAddr}, testProbe); err == nil {
			t.Fatal("expected an error for an incomplete asset response")
		}
	})
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

	got, err := newReader(c, common.Address{}).resolveAdapters(context.Background(), adapters, testProbe)
	testcheck.NoError(t, err, "resolveAdapters: %v")
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
