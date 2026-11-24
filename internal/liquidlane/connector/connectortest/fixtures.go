// Package connectortest builds recipe, permission and signed-discount fixtures shared by the
// connector package tests and the settlement solvers that wrap it.
package connectortest

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	erc20binding "github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	binding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// Fixture identities: the connector, the RWA it buys and the asset it pays.
var (
	Venue    = common.HexToAddress("0x42")
	TokenIn  = common.HexToAddress("0x31")
	TokenOut = common.HexToAddress("0x32")
)

var connectorBinding = binding.NewLiquidLaneConnector()

// DiscountID is the identifier of DiscountFixture.
func DiscountID() *common.Hash { v := common.HexToHash("0x42"); return &v }

// DiscountFixture returns resolved connector terms for the fixture route.
func DiscountFixture() *discounts.Resolved {
	return &discounts.Resolved{DiscountPrecision: "100000000",
		DiscountID:      common.HexToHash("0x42").Hex(),
		Discount:        discounts.Terms{Adapter: Venue.Hex(), TokenToRedeem: TokenIn.Hex(), Discount: "2000000", Signer: common.HexToAddress("0x44").Hex(), Protocol: common.HexToAddress("0x45").Hex(), Nonce: "17", Deadline: 4102444800},
		SignerSignature: "0x" + strings.Repeat("ab", 65), ProtocolSignature: "0x" + strings.Repeat("cd", 65), ProtocolDeadline: 4102444801,
	}
}

// SignatureTemplate packs a connector swap whose permission mask frees only both signatures.
func SignatureTemplate(swap binding.ILiquidLaneConnectorDiscountSwap, signature []byte, recipient common.Address, amount *big.Int) (data, mask []byte) {
	data = connectorBinding.PackSwap(swap, signature, recipient, amount)
	mask = make([]byte, len(data))
	for _, field := range []struct{ start, length int }{
		{420, len(swap.SignerSignature)},
		{452 + (len(swap.SignerSignature)+31)/32*32, len(signature)},
	} {
		for i := field.start; i < field.start+field.length; i++ {
			mask[i] = 255
		}
	}
	return data, mask
}

// DeltaRecipe commits one connector swap guarded by exact account, recipient and RWA balance deltas.
// An optional approval leaf adds the payment approval from input 1.
func DeltaRecipe(t *testing.T, hash common.Hash, account, recipient, asset, rwa common.Address, approval ...recipes.Leaf) recipes.Recipe {
	t.Helper()
	uintType := DeltaType(t, "uint256")
	manage, err := (abi.Arguments{{Type: uintType}, {Type: DeltaType(t, "bytes32[]")}, {Type: DeltaType(t, "uint256[][]")}}).Pack(
		DeltaOperand(recipes.Input, 0), [][32]byte{hash}, [][]*big.Int{{new(big.Int).Or(new(big.Int).Lsh(big.NewInt(100), 224), DeltaOperand(recipes.Input, 0))}},
	)
	if err != nil {
		t.Fatal(err)
	}
	read := func(query int64) []byte {
		data, err := (abi.Arguments{{Type: uintType}}).Pack(big.NewInt(query))
		if err != nil {
			t.Fatal(err)
		}
		return append([]byte{recipes.Read}, data...)
	}
	shift := int64(len(approval))
	result := func(index int64) *big.Int { return DeltaOperand(recipes.Result, index+shift) }
	input := func(index int64) *big.Int { return DeltaOperand(recipes.Input, index) }
	recipe := recipes.Recipe{
		Queries: [][]byte{deltaQuery(t, asset, account), deltaQuery(t, asset, recipient), deltaQuery(t, rwa, account)},
		Steps: [][]byte{
			read(0), read(1), read(2), append([]byte{recipes.Manage}, manage...),
			read(0), DeltaBinary(t, recipes.Sub, result(0), result(4)), DeltaBinary(t, recipes.Equal, result(5), input(1)),
			read(1), DeltaBinary(t, recipes.Sub, result(7), result(1)), DeltaBinary(t, recipes.Equal, result(8), input(1)),
			read(2), DeltaBinary(t, recipes.Sub, result(10), result(2)), DeltaBinary(t, recipes.Equal, result(11), input(0)),
		},
	}
	if len(approval) > 0 {
		args := abi.Arguments{{Type: uintType}, {Type: DeltaType(t, "address")}, {Type: DeltaType(t, "address")}, {Type: DeltaType(t, "bytes32")}}
		// The approval leaf names its spender: the connector the payment is approved for.
		raw, err := args.Pack(input(1), asset, common.BytesToAddress(approval[0].Data[4:36]), approval[0].Hash)
		if err != nil {
			t.Fatal(err)
		}
		recipe.Steps = append([][]byte{append([]byte{recipes.Approve}, raw...)}, recipe.Steps...)
	}
	return recipe
}

// DeltaType returns an elementary ABI type for recipe instruction encoding.
func DeltaType(t *testing.T, name string) abi.Type {
	t.Helper()
	typ, err := abi.NewType(name, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

// DeltaOperand packs a recipe operand from its source and value.
func DeltaOperand(source uint8, value int64) *big.Int {
	return new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(uint64(source)), 216), big.NewInt(value))
}

// DeltaBinary packs a binary recipe instruction over two operands.
func DeltaBinary(t *testing.T, operation byte, left, right *big.Int) []byte {
	t.Helper()
	uintType := DeltaType(t, "uint256")
	data, err := (abi.Arguments{{Type: uintType}, {Type: uintType}}).Pack(left, right)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte{operation}, data...)
}

func deltaQuery(t *testing.T, token, owner common.Address) []byte {
	t.Helper()
	args := abi.Arguments{{Type: DeltaType(t, "address")}, {Type: DeltaType(t, "bytes")}, {Type: DeltaType(t, "uint256[]")}, {Type: DeltaType(t, "uint256")}}
	data, err := args.Pack(token, erc4626.NewIERC4626().PackBalanceOf(owner), []*big.Int{}, big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// CommittedFunding pairs the swap leaf with a payment approval leaf and returns the committed
// recipe, its permission tree and the curator response serving that tree.
func CommittedFunding(t *testing.T, leaf recipes.Leaf, account, recipient, asset, rwa common.Address) (recipes.Recipe, *recipes.Tree, []byte) {
	t.Helper()
	data := erc20binding.NewERC20().PackApprove(leaf.Target, new(big.Int))
	mask := make([]byte, len(data))
	for i := 36; i < 68; i++ {
		mask[i] = 255
	}
	approve := recipes.Leaf{Target: asset, Value: new(big.Int), Data: data, DataMask: mask}
	var err error
	approve.Hash, err = recipes.HashLeaf(approve)
	if err != nil {
		t.Fatal(err)
	}
	a, b := leaf.Hash, approve.Hash
	if a.Cmp(b) > 0 {
		a, b = b, a
	}
	root := crypto.Keccak256Hash(a[:], b[:])
	leaf.Proof = []common.Hash{approve.Hash}
	approve.Proof = []common.Hash{leaf.Hash}
	tree := &recipes.Tree{Root: root, ChainID: big.NewInt(1), Account: &account, Leaves: []recipes.Leaf{approve, leaf}}
	recipe := DeltaRecipe(t, leaf.Hash, account, recipient, asset, rwa, approve)
	entries := make([]map[string]any, 0, len(tree.Leaves))
	for _, l := range tree.Leaves {
		text := make([]string, 0, len(l.Proof))
		for _, h := range l.Proof {
			text = append(text, h.Hex())
		}
		entries = append(entries, map[string]any{"hash": l.Hash.Hex(), "target": l.Target.Hex(), "value": "0", "data": hexutil.Encode(l.Data), "dataMask": hexutil.Encode(l.DataMask), "proof": text})
	}
	raw, err := json.Marshal(map[string]any{"root": root.Hex(), "chainId": "1", "account": account.Hex(), "leaves": entries})
	if err != nil {
		t.Fatal(err)
	}
	return recipe, tree, raw
}
