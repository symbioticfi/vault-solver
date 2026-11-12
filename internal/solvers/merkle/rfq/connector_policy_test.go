package rfq

import (
	"math/big"
	"testing"

	"encoding/json"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	erc20binding "github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
)

func connectorDeltaRecipe(t *testing.T, hash common.Hash, account, recipient, asset, rwa common.Address, approval ...common.Hash) recipes.Recipe {
	t.Helper()
	uintType := connectorDeltaType(t, "uint256")
	manage, err := (abi.Arguments{{Type: uintType}, {Type: connectorDeltaType(t, "bytes32[]")}, {Type: connectorDeltaType(t, "uint256[][]")}}).Pack(
		connectorDeltaOperand(recipes.Input, 0), [][32]byte{hash}, [][]*big.Int{{new(big.Int).Or(new(big.Int).Lsh(big.NewInt(100), 224), connectorDeltaOperand(recipes.Input, 0))}},
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
	result := func(index int64) *big.Int { return connectorDeltaOperand(recipes.Result, index+shift) }
	input := func(index int64) *big.Int { return connectorDeltaOperand(recipes.Input, index) }
	recipe := recipes.Recipe{
		Queries: [][]byte{connectorDeltaQuery(t, asset, account), connectorDeltaQuery(t, asset, recipient), connectorDeltaQuery(t, rwa, account)},
		Steps: [][]byte{
			read(0), read(1), read(2), append([]byte{recipes.Manage}, manage...),
			read(0), connectorDeltaBinary(t, recipes.Sub, result(0), result(4)), connectorDeltaBinary(t, recipes.Equal, result(5), input(1)),
			read(1), connectorDeltaBinary(t, recipes.Sub, result(7), result(1)), connectorDeltaBinary(t, recipes.Equal, result(8), input(1)),
			read(2), connectorDeltaBinary(t, recipes.Sub, result(10), result(2)), connectorDeltaBinary(t, recipes.Equal, result(11), input(0)),
		},
	}
	if len(approval) > 0 {
		args := abi.Arguments{{Type: uintType}, {Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes32")}}
		raw, err := args.Pack(input(1), asset, vlt, approval[0])
		if err != nil {
			t.Fatal(err)
		}
		recipe.Steps = append([][]byte{append([]byte{recipes.Approve}, raw...)}, recipe.Steps...)
	}
	return recipe
}

func connectorDeltaType(t *testing.T, name string) abi.Type {
	t.Helper()
	typ, err := abi.NewType(name, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func connectorDeltaOperand(source uint8, value int64) *big.Int {
	return new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(uint64(source)), 216), big.NewInt(value))
}

func connectorDeltaBinary(t *testing.T, operation byte, left, right *big.Int) []byte {
	t.Helper()
	uintType := connectorDeltaType(t, "uint256")
	data, err := (abi.Arguments{{Type: uintType}, {Type: uintType}}).Pack(left, right)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte{operation}, data...)
}

func connectorDeltaQuery(t *testing.T, token, owner common.Address) []byte {
	t.Helper()
	args := abi.Arguments{{Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes")}, {Type: connectorDeltaType(t, "uint256[]")}, {Type: connectorDeltaType(t, "uint256")}}
	data, err := args.Pack(token, erc4626.NewIERC4626().PackBalanceOf(owner), []*big.Int{}, big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func connectorCommittedFunding(t *testing.T, leaf recipes.Leaf, account, recipient, asset, rwa common.Address) (recipes.Recipe, *recipes.Tree, []byte) {
	t.Helper()
	data := erc20binding.NewERC20().PackApprove(vlt, new(big.Int))
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
	recipe := connectorDeltaRecipe(t, leaf.Hash, account, recipient, asset, rwa, approve.Hash)
	entries := []map[string]any{}
	for _, l := range tree.Leaves {
		text := []string{}
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
