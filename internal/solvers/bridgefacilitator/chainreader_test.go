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

// TestDeriveLiquidity covers the pure reduction of the core-mirror on-chain reads
// (delegator.limitOf, adapter.totalAssets, adapter.outstandingPrincipal, vault.withdrawable) to the
// sizer's inputs:
//
//	fundable    = max(min(limit - held, withdrawable), 0)
//	outstanding = max(outstandingPrincipal, 0)
func TestDeriveLiquidity(t *testing.T) {
	t.Parallel()

	bn := big.NewInt
	huge := bn(1_000_000) // vault liquidity not the binding constraint

	tests := []struct {
		name                                            string
		limit, held, outstandingPrincipal, withdrawable *big.Int
		wantFundable, wantOutstanding                   *big.Int
	}{
		{
			name:                 "room available (cap binds)",
			limit:                bn(1000),
			held:                 bn(400),
			outstandingPrincipal: bn(350),
			withdrawable:         huge,
			wantFundable:         bn(600),
			wantOutstanding:      bn(350),
		},
		{
			name:                 "vault liquidity binds below cap headroom",
			limit:                bn(1000),
			held:                 bn(400),
			outstandingPrincipal: bn(350),
			withdrawable:         bn(250),
			wantFundable:         bn(250),
			wantOutstanding:      bn(350),
		},
		{
			name:                 "dry vault clamps fundable to zero despite cap room",
			limit:                bn(1000),
			held:                 bn(400),
			outstandingPrincipal: bn(350),
			withdrawable:         bn(0),
			wantFundable:         bn(0),
			wantOutstanding:      bn(350),
		},
		{
			name:                 "cap fully consumed",
			limit:                bn(1000),
			held:                 bn(1000),
			outstandingPrincipal: bn(900),
			withdrawable:         huge,
			wantFundable:         bn(0),
			wantOutstanding:      bn(900),
		},
		{
			name:                 "held over cap clamps fundable to zero",
			limit:                bn(500),
			held:                 bn(800),
			outstandingPrincipal: bn(700),
			withdrawable:         huge,
			wantFundable:         bn(0),
			wantOutstanding:      bn(700),
		},
		{
			name:                 "negative outstanding clamps to zero",
			limit:                bn(1000),
			held:                 bn(100),
			outstandingPrincipal: bn(-5),
			withdrawable:         huge,
			wantFundable:         bn(900),
			wantOutstanding:      bn(0),
		},
		{
			name:                 "all zero",
			limit:                bn(0),
			held:                 bn(0),
			outstandingPrincipal: bn(0),
			withdrawable:         bn(0),
			wantFundable:         bn(0),
			wantOutstanding:      bn(0),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotFundable, gotOutstanding := deriveLiquidity(tc.limit, tc.held, tc.outstandingPrincipal, tc.withdrawable)
			if gotFundable.Cmp(tc.wantFundable) != 0 {
				t.Errorf("fundable = %s, want %s", gotFundable, tc.wantFundable)
			}
			if gotOutstanding.Cmp(tc.wantOutstanding) != 0 {
				t.Errorf("outstanding = %s, want %s", gotOutstanding, tc.wantOutstanding)
			}
		})
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

	c, err := chain.Dial(t.Context(), []string{srv.URL}, multicallAddr.Hex(), logr.Discard())
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

// TestDeriveLiquidityDoesNotMutateInputs guards against the clamp accidentally aliasing/mutating the
// caller's *big.Int values (deriveLiquidity must allocate its own results).
func TestDeriveLiquidityDoesNotMutateInputs(t *testing.T) {
	t.Parallel()

	limit := big.NewInt(500)
	held := big.NewInt(800) // held > limit, so fundable clamps to 0
	outstandingPrincipal := big.NewInt(-1)
	withdrawable := big.NewInt(1000)

	_, _ = deriveLiquidity(limit, held, outstandingPrincipal, withdrawable)

	if limit.Cmp(big.NewInt(500)) != 0 || held.Cmp(big.NewInt(800)) != 0 ||
		outstandingPrincipal.Cmp(big.NewInt(-1)) != 0 || withdrawable.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("deriveLiquidity mutated its inputs: limit=%s held=%s outstanding=%s withdrawable=%s",
			limit, held, outstandingPrincipal, withdrawable)
	}
}
