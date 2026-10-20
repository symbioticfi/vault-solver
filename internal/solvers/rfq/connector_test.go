package rfq

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

func TestConnectorQuoteDoesNotUseAdapterEconomics(t *testing.T) {
	cfg := &Config{SolverMode: solverModeInternal, RecipeExecutors: map[common.Address]common.Address{vlt: common.HexToAddress("0x43")}}
	quotes, exec := buildServices(cfg, 1, newStore(time.Now), nil, nil, func() bool { return true }, newDefaultTestStrategy(), logr.Discard())
	q := validQuoteBody()
	q.Adapters = []quoteAdapter{{Adapter: vlt.Hex(), Asset: tOut.Hex(), AssetDecimals: 6, MaxAssets: "900", Discount: "2000000"}}
	decision, err := quotes.quote(t.Context(), &q)
	if err != nil || decision.response != nil {
		t.Fatalf("connector advertised a fabricated quote: %+v, %v", decision, err)
	}
	if len(exec.vaults) != 0 {
		t.Fatal("connector entered vault inventory discovery")
	}
}

func TestAssembleConnectorRouteKeepsCommittedProgram(t *testing.T) {
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data := connectorBinding.PackSwap0(terms.Swap, terms.Signature, common.HexToAddress("0x10"), big.NewInt(0))
	mask := bytes.Repeat([]byte{255}, len(data))
	for i := 0; i < 4; i++ {
		mask[i] = 0
	}
	// The RWA word is part of the permission's fixed preimage.
	for i := 132; i < 164; i++ {
		mask[i] = 0
	}
	leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	tree := &recipes.Tree{Root: leaf.Hash, Leaves: []recipes.Leaf{leaf}}
	account := common.HexToAddress("0x21")
	recipe := connectorDeltaRecipe(t, leaf.Hash, account, common.HexToAddress("0x10"), tOut, tIn)
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := recipes.Snapshot{Executor: common.HexToAddress("0x43"), RecipeHash: hash, Account: account}
	assembled, err := assembleConnectorRoute(snapshot, recipe, tree, vlt, tIn, tOut, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err != nil {
		t.Fatal(err)
	}
	route := assembled.Route
	if route.AmountIn.Int64() != 123 || len(route.Inputs) != 2 || route.Inputs[0].Int64() != 123 || route.Inputs[1].Int64() != 456 || !bytes.Equal(route.Steps[0], recipe.Steps[0]) || len(assembled.Calldata) == 0 {
		t.Fatalf("incorrect route: %+v", route)
	}
	after, _ := recipes.HashRecipe(recipe)
	if after != hash {
		t.Fatal("recipe instructions changed")
	}
	_, err = assembleConnectorRoute(snapshot, recipe, tree, vlt, common.HexToAddress("0x99"), tOut, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil {
		t.Fatal("mismatched RWA permission accepted")
	}
	// A signed field remains fixed when the curator has not granted it a mutable mask.
	terms.Swap.Discount.Nonce = big.NewInt(18)
	for i := 260; i < 292; i++ {
		tree.Leaves[0].DataMask[i] = 0
	}
	_, err = assembleConnectorRoute(snapshot, recipe, tree, vlt, tIn, tOut, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil {
		t.Fatal("pinned signed data changed")
	}
}

type connectorReadFake struct {
	remaining *big.Int
	account   common.Address
}

func (f *connectorReadFake) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(7)}, nil
}
func (f *connectorReadFake) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}
func (f *connectorReadFake) CallContract(_ context.Context, m ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	a := connectorBinding.GetABI()
	for _, method := range a.Methods {
		if !bytes.Equal(m.Data[:4], method.ID) {
			continue
		}
		switch method.RawName {
		case "account":
			return method.Outputs.Pack(f.account)
		case "outputAsset":
			return method.Outputs.Pack(tOut)
		case "remainingAssets":
			return method.Outputs.Pack(f.remaining)
		case "version":
			return method.Outputs.Pack(uint64(2))
		}
	}
	return nil, nil
}

func TestConnectorPlanUsesWholeAwardAndReservations(t *testing.T) {
	other := common.HexToAddress("0x20")
	w := &connectorWorkflow{chain: &connectorReadFake{remaining: big.NewInt(900), account: common.HexToAddress("0x21")}, chainID: big.NewInt(1), executors: map[common.Address]common.Address{vlt: common.HexToAddress("0x43"), other: common.HexToAddress("0x44")}}
	listing := &discounts.List{Discounts: []discounts.ListItem{
		{Adapter: other.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: common.HexToHash("0x02").Hex(), MaxAssets: "900", Deadline: 4102444800},
		{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex(), DiscountID: common.HexToHash("0x01").Hex(), MaxAssets: "900", Deadline: 4102444800},
	}}
	for _, tc := range []struct {
		name     string
		reserved int64
		want     bool
	}{{"exact award", 100, true}, {"capacity reserved", 101, false}} {
		t.Run(tc.name, func(t *testing.T) {
			plan, _, err := w.plan(t.Context(), listing, "quote", tIn, tOut, big.NewInt(123), big.NewInt(800), 7, func(inv []solverInventory) liquidlane.CapacityReservations {
				r := liquidlane.CapacityReservations{}
				for _, item := range inv {
					r[item.CapacityID] = big.NewInt(tc.reserved)
				}
				return r
			})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.want {
				if plan != nil {
					t.Fatal("reserved capacity reused")
				}
				return
			}
			if plan == nil || len(plan.Legs) != 1 || plan.Legs[0].Adapter != vlt || plan.Legs[0].AmountIn.Int64() != 123 || plan.Legs[0].AmountOut.Int64() != 800 || plan.AmountIn.Int64() != 123 || plan.QuotedAmountOut.Int64() != 800 {
				t.Fatalf("award changed: %+v", plan)
			}
		})
	}
}

func connectorDiscountFixture() *resolveDiscountResponse {
	return &discounts.Resolved{
		DiscountID:      common.HexToHash("0x42").Hex(),
		Discount:        discounts.Terms{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Discount: "2000000", Signer: common.HexToAddress("0x44").Hex(), Protocol: common.HexToAddress("0x45").Hex(), Nonce: "17", Deadline: 4102444800},
		SignerSignature: "0xaabb", ProtocolSignature: "0xccdd", ProtocolDeadline: 4102444801,
	}
}

func TestConnectorDiscountPreservesUpstreamTerms(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*resolveDiscountResponse)
		want   string
	}{
		{"unchanged", func(*resolveDiscountResponse) {}, ""},
		{"uint256 overflow", func(r *resolveDiscountResponse) { r.Discount.Nonce = new(big.Int).Lsh(big.NewInt(1), 256).String() }, "nonce"},
		{"uint48 overflow", func(r *resolveDiscountResponse) { r.ProtocolDeadline = 1 << 48 }, "deadline"},
		{"malformed signature", func(r *resolveDiscountResponse) { r.SignerSignature = "0xgg" }, "signerSignature"},
		{"wrong route", func(r *resolveDiscountResponse) { r.Discount.Adapter = common.HexToAddress("0x55").Hex() }, "route"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := connectorDiscountFixture()
			tc.mutate(r)
			terms, err := parseConnectorDiscount(r, *connectorDiscountID(), vlt, tIn)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error=%v, want %s", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if terms.Swap.Discount.Discount.String() != "2000000" || terms.Swap.Discount.Nonce.String() != "17" || terms.Swap.Discount.Deadline.Int64() != 4102444800 || terms.Swap.ProtocolDeadline.Int64() != 4102444801 || string(terms.Swap.SignerSignature) != "\xaa\xbb" || string(terms.Signature) != "\xcc\xdd" || terms.ValidUntil.Unix() != 4102444800 {
				t.Fatalf("signed data changed: %+v", terms)
			}
		})
	}
}

func connectorDiscountID() *common.Hash { v := common.HexToHash("0x42"); return &v }
