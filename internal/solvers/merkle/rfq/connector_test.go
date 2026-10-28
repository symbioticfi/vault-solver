package rfq

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

var recipeBinding = recipeexecutor.NewRecipeExecutor()

func TestAssembleConnectorRouteKeepsCommittedProgram(t *testing.T) {
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data, mask := connectorSignatureTemplate(terms, common.HexToAddress("0x10"), big.NewInt(0))
	for i := 100; i < 132; i++ {
		mask[i] = 255
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
	assembled, err := assembleConnectorRoute(recipe, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err != nil {
		t.Fatal(err)
	}
	values, err := recipeBinding.GetABI().Methods["execute"].Inputs.Unpack(assembled.Calldata[4:])
	if err != nil {
		t.Fatal(err)
	}
	inputs := values[2].([]*big.Int)
	if len(inputs) != 2 || inputs[0].Int64() != 123 || inputs[1].Int64() != 456 || !bytes.Equal(values[1].([][]byte)[0], recipe.Steps[0]) {
		t.Fatal("awarded inputs or committed program changed")
	}
	after, _ := recipes.HashRecipe(recipe)
	if after != hash {
		t.Fatal("recipe instructions changed")
	}
	_, err = assembleConnectorRoute(recipe, tree, vlt, common.HexToAddress("0x99"), common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil {
		t.Fatal("mismatched RWA permission accepted")
	}
	multiple := recipes.CloneRecipe(recipe)
	multiple.Steps = append(multiple.Steps, append([]byte(nil), recipe.Steps[3]...))
	_, err = assembleConnectorRoute(multiple, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err != nil {
		t.Fatalf("curator-authorized repeated swap rejected: %v", err)
	}
	for _, gate := range []*big.Int{connectorDeltaOperand(recipes.Literal, 1), connectorDeltaOperand(recipes.Result, 0)} {
		alternative := recipes.CloneRecipe(recipe)
		args := abi.Arguments{{Type: connectorDeltaType(t, "uint256")}, {Type: connectorDeltaType(t, "bytes32[]")}, {Type: connectorDeltaType(t, "uint256[][]")}}
		managedArgs, err := args.Unpack(alternative.Steps[3][1:])
		if err != nil {
			t.Fatal(err)
		}
		managedStep, err := args.Pack(gate, managedArgs[1], managedArgs[2])
		if err != nil {
			t.Fatal(err)
		}
		alternative.Steps[3] = append([]byte{recipes.Manage}, managedStep...)
		if _, err := assembleConnectorRoute(alternative, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature); err != nil {
			t.Fatalf("curator gate rejected: %v", err)
		}
	}
	// A signed field remains fixed when the curator has not granted it a mutable mask.
	terms.Swap.Discount.Nonce = big.NewInt(18)
	for i := 260; i < 292; i++ {
		tree.Leaves[0].DataMask[i] = 0
	}
	_, err = assembleConnectorRoute(recipe, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil {
		t.Fatal("pinned signed data changed")
	}
}

type connectorReadFake struct {
	remaining *big.Int
	account   common.Address
	executor  common.Address
}

func (f *connectorReadFake) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(7)}, nil
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
		}
	}
	return nil, nil
}

func connectorDiscountFixture() *discounts.Resolved {
	return &discounts.Resolved{
		DiscountID:      common.HexToHash("0x42").Hex(),
		Discount:        discounts.Terms{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Discount: "2000000", Signer: common.HexToAddress("0x44").Hex(), Protocol: common.HexToAddress("0x45").Hex(), Nonce: "17", Deadline: 4102444800},
		SignerSignature: "0x" + strings.Repeat("ab", 65), ProtocolSignature: "0x" + strings.Repeat("cd", 65), ProtocolDeadline: 4102444801,
	}
}

func TestConnectorDiscountPreservesUpstreamTerms(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*discounts.Resolved)
		want   string
	}{
		{"unchanged", func(*discounts.Resolved) {}, ""},
		{"uint256 overflow", func(r *discounts.Resolved) { r.Discount.Nonce = new(big.Int).Lsh(big.NewInt(1), 256).String() }, "nonce"},
		{"uint48 overflow", func(r *discounts.Resolved) { r.ProtocolDeadline = 1 << 48 }, "deadline"},
		{"malformed signer signature", func(r *discounts.Resolved) { r.SignerSignature = "0xgg" }, "signerSignature"},
		{"odd signer signature", func(r *discounts.Resolved) { r.SignerSignature = "0xabc" }, "signerSignature"},
		{"malformed protocol signature", func(r *discounts.Resolved) { r.ProtocolSignature = "0xgg" }, "protocolSignature"},
		{"odd protocol signature", func(r *discounts.Resolved) { r.ProtocolSignature = "0xabc" }, "protocolSignature"},
		{"empty signatures", func(r *discounts.Resolved) { r.SignerSignature, r.ProtocolSignature = "0x", "0x" }, ""},
		{"short signer signature", func(r *discounts.Resolved) { r.SignerSignature = "0xaabb" }, ""},
		{"long signer signature", func(r *discounts.Resolved) { r.SignerSignature = "0x" + strings.Repeat("ab", 130) }, ""},
		{"short protocol signature", func(r *discounts.Resolved) { r.ProtocolSignature = "0xaabb" }, ""},
		{"long protocol signature", func(r *discounts.Resolved) { r.ProtocolSignature = "0x" + strings.Repeat("ab", 130) }, ""},
		{"wrong route", func(r *discounts.Resolved) { r.Discount.Adapter = common.HexToAddress("0x55").Hex() }, "route"},
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
			if terms.Swap.Discount.Discount.String() != "2000000" || terms.Swap.Discount.Nonce.String() != "17" || terms.Swap.Discount.Deadline.Int64() != 4102444800 || terms.Swap.ProtocolDeadline.Int64() != 4102444801 || hexutil.Encode(terms.Swap.SignerSignature) != r.SignerSignature || hexutil.Encode(terms.Signature) != r.ProtocolSignature || terms.ValidUntil.Unix() != 4102444800 {
				t.Fatalf("signed data changed: %+v", terms)
			}
		})
	}
}

func connectorDiscountID() *common.Hash { v := common.HexToHash("0x42"); return &v }

func connectorSignatureTemplate(terms connectorTerms, recipient common.Address, amount *big.Int) (data, mask []byte) {
	data = connectorBinding.PackSwap(terms.Swap, terms.Signature, recipient, amount)
	mask = make([]byte, len(data))
	for _, field := range []struct{ start, length int }{
		{420, len(terms.Swap.SignerSignature)},
		{452 + (len(terms.Swap.SignerSignature)+31)/32*32, len(terms.Signature)},
	} {
		for i := field.start; i < field.start+field.length; i++ {
			mask[i] = 255
		}
	}
	return data, mask
}

func TestAssembleConnectorRouteRequiresPinnedTupleOffset(t *testing.T) {
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		offset  int64
		mutable bool
		want    string
	}{
		{"relocated tuple", 160, false, "tuple offset"},
		{"mutable tuple header", 128, true, "unconstrained"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := common.HexToAddress("0x10")
			data, mask := connectorSignatureTemplate(terms, caller, new(big.Int))
			big.NewInt(tc.offset).FillBytes(data[4:36])
			if tc.mutable {
				for i := 4; i < 36; i++ {
					mask[i] = 255
				}
			}
			leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
			leaf.Hash, err = recipes.HashLeaf(leaf)
			if err != nil {
				t.Fatal(err)
			}
			tree := &recipes.Tree{Root: leaf.Hash, Leaves: []recipes.Leaf{leaf}}
			recipe := connectorDeltaRecipe(t, leaf.Hash, common.HexToAddress("0x21"), caller, tOut, tIn)
			_, err = assembleConnectorRoute(recipe, tree, vlt, tIn, caller, big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("route header accepted: %v", err)
			}
		})
	}
}
