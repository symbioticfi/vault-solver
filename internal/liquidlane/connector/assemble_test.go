package connector

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

var recipeBinding = recipeexecutor.NewRecipeExecutor()

var vlt, tIn, tOut = connectortest.Venue, connectortest.TokenIn, connectortest.TokenOut

func TestAssembleConnectorRouteBindsRWAInRecipe(t *testing.T) {
	terms, err := ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		value   *big.Int
		offset  int64
		wantErr bool
	}{
		{name: "literal token", value: new(big.Int).SetBytes(tIn[:]), offset: 132},
		{name: "different token", value: big.NewInt(999), offset: 132, wantErr: true},
		{name: "oversized token", value: new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 160), new(big.Int).SetBytes(tIn[:])), offset: 132, wantErr: true},
		{name: "runtime token", value: connectortest.DeltaOperand(recipes.Input, 0), offset: 132, wantErr: true},
		{name: "partial token binding", value: new(big.Int).SetBytes(tIn[:]), offset: 120, wantErr: true},
		{name: "unbound token", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, mask := connectortest.SignatureTemplate(terms.Swap, terms.Signature, common.HexToAddress("0x10"), new(big.Int))
			for i := 100; i < 164; i++ {
				mask[i] = 255
			}
			clear(data[132:164])
			leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
			leaf.Hash, err = recipes.HashLeaf(leaf)
			if err != nil {
				t.Fatal(err)
			}
			tree := &recipes.Tree{Root: leaf.Hash, Leaves: []recipes.Leaf{leaf}}
			recipe := connectortest.DeltaRecipe(t, leaf.Hash, common.HexToAddress("0x21"), common.HexToAddress("0x10"), tOut, tIn)
			args := abi.Arguments{{Type: connectortest.DeltaType(t, "uint256")}, {Type: connectortest.DeltaType(t, "bytes32[]")}, {Type: connectortest.DeltaType(t, "uint256[][]")}}
			decoded, err := args.Unpack(recipe.Steps[3][1:])
			if err != nil {
				t.Fatal(err)
			}
			bindings := decoded[2].([][]*big.Int)
			if tc.value != nil {
				bindings[0] = append(bindings[0], new(big.Int).Or(new(big.Int).Lsh(big.NewInt(tc.offset), 224), tc.value))
			}
			packed, err := args.Pack(decoded[0], decoded[1], bindings)
			if err != nil {
				t.Fatal(err)
			}
			recipe.Steps[3] = append([]byte{recipes.Manage}, packed...)
			before, err := recipes.HashRecipe(recipe)
			if err != nil {
				t.Fatal(err)
			}
			_, err = assembleRoute(recipe, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error = %t", err, tc.wantErr)
			}
			after, err := recipes.HashRecipe(recipe)
			if err != nil || before != after {
				t.Fatal("committed recipe changed")
			}
		})
	}
}

func TestAssembleConnectorRouteKeepsCommittedProgram(t *testing.T) {
	terms, err := ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data, mask := connectortest.SignatureTemplate(terms.Swap, terms.Signature, common.HexToAddress("0x10"), big.NewInt(0))
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
	recipe := connectortest.DeltaRecipe(t, leaf.Hash, account, common.HexToAddress("0x10"), tOut, tIn)
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := assembleRoute(recipe, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
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
	_, err = assembleRoute(recipe, tree, vlt, common.HexToAddress("0x99"), common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil {
		t.Fatal("mismatched RWA permission accepted")
	}
	multiple := recipes.CloneRecipe(recipe)
	multiple.Steps = append(multiple.Steps, append([]byte(nil), recipe.Steps[3]...))
	_, err = assembleRoute(multiple, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err != nil {
		t.Fatalf("curator-authorized repeated swap rejected: %v", err)
	}
	for _, gate := range []*big.Int{connectortest.DeltaOperand(recipes.Literal, 1), connectortest.DeltaOperand(recipes.Result, 0)} {
		alternative := recipes.CloneRecipe(recipe)
		args := abi.Arguments{{Type: connectortest.DeltaType(t, "uint256")}, {Type: connectortest.DeltaType(t, "bytes32[]")}, {Type: connectortest.DeltaType(t, "uint256[][]")}}
		managedArgs, err := args.Unpack(alternative.Steps[3][1:])
		if err != nil {
			t.Fatal(err)
		}
		managedStep, err := args.Pack(gate, managedArgs[1], managedArgs[2])
		if err != nil {
			t.Fatal(err)
		}
		alternative.Steps[3] = append([]byte{recipes.Manage}, managedStep...)
		if _, err := assembleRoute(alternative, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature); err != nil {
			t.Fatalf("curator gate rejected: %v", err)
		}
	}
	// A signed field remains fixed when the curator has not granted it a mutable mask.
	terms.Swap.Discount.Nonce = big.NewInt(18)
	for i := 260; i < 292; i++ {
		tree.Leaves[0].DataMask[i] = 0
	}
	_, err = assembleRoute(recipe, tree, vlt, tIn, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil {
		t.Fatal("pinned signed data changed")
	}
}

func TestConnectorDiscountPreservesUpstreamTerms(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*discounts.Resolved)
		want   string
	}{
		{"unchanged", func(*discounts.Resolved) {}, ""},
		{"missing precision", func(r *discounts.Resolved) { r.DiscountPrecision = "" }, "precision"},
		{"native precision", func(r *discounts.Resolved) { r.DiscountPrecision = "1000000" }, "precision"},
		{"unknown precision", func(r *discounts.Resolved) { r.DiscountPrecision = "10000" }, "precision"},
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
			r := connectortest.DiscountFixture()
			tc.mutate(r)
			terms, err := ParseDiscount(r, *connectortest.DiscountID(), vlt, tIn)
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

func TestAssembleConnectorRouteRequiresPinnedTupleOffset(t *testing.T) {
	terms, err := ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), vlt, tIn)
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
			data, mask := connectortest.SignatureTemplate(terms.Swap, terms.Signature, caller, new(big.Int))
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
			recipe := connectortest.DeltaRecipe(t, leaf.Hash, common.HexToAddress("0x21"), caller, tOut, tIn)
			_, err = assembleRoute(recipe, tree, vlt, tIn, caller, big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("route header accepted: %v", err)
			}
		})
	}
}
