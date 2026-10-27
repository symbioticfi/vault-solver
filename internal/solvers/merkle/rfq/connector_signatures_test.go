package rfq

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

func connectorSignatureTemplate(terms connectorTerms, recipient common.Address, amount *big.Int) (data, mask []byte) {
	padded := terms.Swap
	padded.SignerSignature = common.RightPadBytes(terms.Swap.SignerSignature, 4095)
	data = connectorBinding.PackSwap(padded, terms.Signature, recipient, amount)
	mask = make([]byte, len(data))
	new(big.Int).SetUint64(uint64(len(terms.Swap.SignerSignature))).FillBytes(data[388:420])
	mask[418] = 15
	mask[419] = 255
	for i := 420; i < 420+4095; i++ {
		mask[i] = 255
	}
	for i := 4548; i < 4548+65; i++ {
		mask[i] = 255
	}
	return data, mask
}

func TestConnectorSignaturesRequireBoundedLengthMasks(t *testing.T) {
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	terms.Swap.SignerSignature = make([]byte, 65)
	terms.Signature = make([]byte, 65)
	data := connectorBinding.PackSwap(terms.Swap, terms.Signature, common.HexToAddress("0x10"), big.NewInt(123))
	for _, lengthMask := range [][]byte{make([]byte, 32), bytes.Repeat([]byte{255}, 32)} {
		mask := make([]byte, len(data))
		copy(mask[388:420], lengthMask)
		leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
		if _, err := fitConnectorSignatures(leaf, terms.Swap, terms.Signature, common.HexToAddress("0x10"), big.NewInt(123)); err == nil || !strings.Contains(err.Error(), "connector signature length mask is invalid") {
			t.Fatalf("unbounded signature length accepted: %v", err)
		}
	}
}

func TestConnectorSignaturesFitCommittedBoundedBuffers(t *testing.T) {
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data, mask := connectorSignatureTemplate(terms, common.HexToAddress("0x10"), big.NewInt(123))
	leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
	before, err := recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	for _, length := range []int{0, 1, 64, 65, 130, 4095} {
		signed := terms.Swap
		signed.SignerSignature = bytes.Repeat([]byte{0xab}, length)
		protocol := bytes.Repeat([]byte{0xcd}, 65)
		fitted, err := fitConnectorSignatures(leaf, signed, protocol, common.HexToAddress("0x10"), big.NewInt(123))
		if err != nil {
			t.Fatalf("length %d: %v", length, err)
		}
		values, err := connectorBinding.GetABI().Methods["swap"].Inputs.Unpack(fitted[4:])
		if err != nil {
			t.Fatal(err)
		}
		decoded := *abi.ConvertType(values[0], new(connector.ILiquidLaneConnectorDiscountSwap)).(*connector.ILiquidLaneConnectorDiscountSwap)
		if !bytes.Equal(decoded.SignerSignature, signed.SignerSignature) || !bytes.Equal(values[1].([]byte), protocol) {
			t.Fatal("ABI decoder changed signature bytes")
		}
		for _, signature := range []struct {
			start    int
			expected []byte
		}{{388, signed.SignerSignature}, {4516, protocol}} {
			if new(big.Int).SetBytes(fitted[signature.start:signature.start+32]).Int64() != int64(len(signature.expected)) || !bytes.Equal(fitted[signature.start+32:signature.start+32+len(signature.expected)], signature.expected) {
				t.Fatal("signature bytes or length changed")
			}
		}
		after := leaf
		after.Data = fitted
		hash, err := recipes.HashLeaf(after)
		if err != nil || hash != before {
			t.Fatalf("fixed terms changed for %d bytes: %v", length, err)
		}
	}
	tooLong := terms.Swap
	tooLong.SignerSignature = make([]byte, 4096)
	if _, err := fitConnectorSignatures(leaf, tooLong, terms.Signature, common.HexToAddress("0x10"), big.NewInt(123)); err == nil {
		t.Fatal("oversized signature accepted")
	}
	for _, lengths := range [][2]int{{4095, 65}, {4096, 65}, {4095, 64}, {4095, 66}} {
		signed := terms.Swap
		signed.SignerSignature = make([]byte, lengths[0])
		_, err := fitConnectorSignatures(leaf, signed, make([]byte, lengths[1]), common.HexToAddress("0x10"), big.NewInt(123))
		if (err == nil) != (lengths[0] == 4095 && lengths[1] == 65) {
			t.Fatalf("incorrect capacity result for %v: %v", lengths, err)
		}
	}
	mutableProtocol := leaf
	mutableProtocol.DataMask = bytes.Clone(mask)
	mutableProtocol.DataMask[4518] = 15
	mutableProtocol.DataMask[4519] = 255
	if _, err := fitConnectorSignatures(mutableProtocol, terms.Swap, terms.Signature, common.HexToAddress("0x10"), big.NewInt(123)); err == nil {
		t.Fatal("mutable protocol signature length accepted")
	}
	for _, at := range []int{4, 36, 324} {
		malformed := leaf
		malformed.Data = bytes.Clone(data)
		malformed.Data[at] = 255
		if _, err := fitConnectorSignatures(malformed, terms.Swap, terms.Signature, common.HexToAddress("0x10"), big.NewInt(123)); err == nil {
			t.Fatalf("malformed offset %d accepted", at)
		}
	}
	// Exercise the complete connector route and generic SDK assembly with bounded
	// contract signatures; changing a fixed nonce still fails the committed mask.
	for i := 100; i < 132; i++ {
		leaf.DataMask[i] = 255
	}
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	account := common.HexToAddress("0x21")
	recipe := connectorDeltaRecipe(t, leaf.Hash, account, common.HexToAddress("0x10"), tOut, tIn)
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := recipes.Snapshot{Executor: common.HexToAddress("0x43"), RecipeHash: hash, Account: account}
	tree := &recipes.Tree{Root: leaf.Hash, Leaves: []recipes.Leaf{leaf}}
	for _, length := range []int{0, 130, 4095} {
		resolved := connectorDiscountFixture()
		resolved.SignerSignature = hexutil.Encode(make([]byte, length))
		parsed, err := parseConnectorDiscount(resolved, *connectorDiscountID(), vlt, tIn)
		if err != nil {
			t.Fatalf("parse length %d: %v", length, err)
		}
		assembled, err := assembleConnectorRoute(snapshot, recipe, tree, vlt, tIn, tOut, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), parsed.Swap, parsed.Signature)
		if err != nil {
			t.Fatalf("full route length %d: %v", length, err)
		}
		if len(assembled.Calldata) == 0 || len(assembled.Leaves) != 1 || assembled.Leaves[0] != leaf.Hash {
			t.Fatal("full route omitted calldata or its committed leaf")
		}
		// The SDK's immutable calldata check also applies after bounded re-layout.
		changed := parsed.Swap
		changed.Discount.Nonce = big.NewInt(99)
		if _, err := assembleConnectorRoute(snapshot, recipe, tree, vlt, tIn, tOut, common.HexToAddress("0x10"), big.NewInt(123), big.NewInt(456), changed, terms.Signature); err == nil {
			t.Fatal("full route changed fixed nonce")
		}
	}
}
