package rfq

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
)

func connectorDeltaRecipe(t *testing.T, hash common.Hash, account, recipient, asset, rwa common.Address) recipes.Recipe {
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
	result := func(index int64) *big.Int { return connectorDeltaOperand(recipes.Result, index) }
	input := func(index int64) *big.Int { return connectorDeltaOperand(recipes.Input, index) }
	return recipes.Recipe{
		Queries: [][]byte{connectorDeltaQuery(t, asset, account), connectorDeltaQuery(t, asset, recipient), connectorDeltaQuery(t, rwa, account)},
		Steps: [][]byte{
			read(0), read(1), read(2), append([]byte{recipes.Manage}, manage...),
			read(0), connectorDeltaBinary(t, recipes.Sub, result(0), result(4)), connectorDeltaBinary(t, recipes.Equal, result(5), input(1)),
			read(1), connectorDeltaBinary(t, recipes.Sub, result(7), result(1)), connectorDeltaBinary(t, recipes.Equal, result(8), input(1)),
			read(2), connectorDeltaBinary(t, recipes.Sub, result(10), result(2)), connectorDeltaBinary(t, recipes.Equal, result(11), input(0)),
		},
	}
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
