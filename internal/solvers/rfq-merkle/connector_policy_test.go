package rfqmerkle

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
)

func TestConnectorDeltasRequireExactAwardedBalanceChanges(t *testing.T) {
	account := common.HexToAddress("0x21")
	recipient := common.HexToAddress("0x10")
	for _, tc := range []struct {
		name   string
		mutate func(*recipes.Recipe)
		valid  bool
	}{
		{"canonical balance deltas", func(*recipes.Recipe) {}, true},
		{"equal operands reversed", func(r *recipes.Recipe) {
			r.Steps[6] = connectorDeltaBinary(t, recipes.Equal, connectorDeltaOperand(recipes.Input, 1), connectorDeltaOperand(recipes.Result, 5))
		}, true},
		{"wrong payment recipient", func(r *recipes.Recipe) { r.Queries[1] = connectorDeltaQuery(t, tOut, common.HexToAddress("0x88")) }, false},
		{"wrong payment asset", func(r *recipes.Recipe) { r.Queries[1] = connectorDeltaQuery(t, tIn, recipient) }, false},
		{"wrong account", func(r *recipes.Recipe) { r.Queries[0] = connectorDeltaQuery(t, tOut, common.HexToAddress("0x88")) }, false},
		{"cash direction reversed", func(r *recipes.Recipe) {
			r.Steps[5] = connectorDeltaBinary(t, recipes.Sub, connectorDeltaOperand(recipes.Result, 4), connectorDeltaOperand(recipes.Result, 0))
		}, false},
		{"self comparison", func(r *recipes.Recipe) {
			r.Steps[6] = connectorDeltaBinary(t, recipes.Equal, connectorDeltaOperand(recipes.Input, 1), connectorDeltaOperand(recipes.Input, 1))
		}, false},
		{"wrong payment input", func(r *recipes.Recipe) {
			r.Steps[9] = connectorDeltaBinary(t, recipes.Equal, connectorDeltaOperand(recipes.Result, 8), connectorDeltaOperand(recipes.Input, 0))
		}, false},
		{"missing RWA delta", func(r *recipes.Recipe) { r.Steps = r.Steps[:12] }, false},
		{"pre reads cross action", func(r *recipes.Recipe) { connectorDeltaInsertAction(t, r, 3) }, false},
		{"post checks cross action", func(r *recipes.Recipe) { connectorDeltaInsertAction(t, r, 7) }, false},
		{"truncated comparison", func(r *recipes.Recipe) { r.Steps[6] = []byte{recipes.Equal} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipe := connectorDeltaRecipe(t, common.HexToHash("0x42"), account, recipient, tOut, tIn)
			tc.mutate(&recipe)
			swapStep := uint(3)
			if tc.name == "pre reads cross action" {
				swapStep++
			}
			err := validateConnectorDeltas(recipe, swapStep, account, recipient, tOut, tIn)
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

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

func connectorDeltaInsertAction(t *testing.T, recipe *recipes.Recipe, at int) {
	t.Helper()
	for _, step := range recipe.Steps {
		if len(step) < 65 || step[0] < recipes.Add || step[0] > recipes.LessOrEqual {
			continue
		}
		for _, word := range [][]byte{step[1:33], step[33:65]} {
			if word[4] == recipes.Result {
				index := new(big.Int).SetBytes(word[5:])
				if index.Cmp(big.NewInt(int64(at))) >= 0 {
					index.Add(index, big.NewInt(1)).FillBytes(word[5:])
				}
			}
		}
	}
	args := abi.Arguments{{Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes")}, {Type: connectorDeltaType(t, "uint256[]")}}
	call, err := args.Pack(common.HexToAddress("0x77"), []byte{}, []*big.Int{})
	if err != nil {
		t.Fatal(err)
	}
	steps := append([][]byte{}, recipe.Steps[:at]...)
	steps = append(steps, append([]byte{recipes.Call}, call...))
	recipe.Steps = append(steps, recipe.Steps[at:]...)
}

func TestConnectorDeltasQueryBindingsAndOffsetsAreNotStaticBalances(t *testing.T) {
	account, recipient := common.HexToAddress("0x21"), common.HexToAddress("0x10")
	for _, offset := range []int64{1, 32} {
		recipe := connectorDeltaRecipe(t, common.HexToHash("0x42"), account, recipient, tOut, tIn)
		args := abi.Arguments{{Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes")}, {Type: connectorDeltaType(t, "uint256[]")}, {Type: connectorDeltaType(t, "uint256")}}
		var err error
		recipe.Queries[1], err = args.Pack(tOut, erc4626.NewIERC4626().PackBalanceOf(recipient), []*big.Int{}, big.NewInt(offset))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateConnectorDeltas(recipe, 3, account, recipient, tOut, tIn); err == nil {
			t.Fatalf("nonzero result offset %d accepted", offset)
		}
	}
	bound := connectorDeltaRecipe(t, common.HexToHash("0x42"), account, recipient, tOut, tIn)
	args := abi.Arguments{{Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes")}, {Type: connectorDeltaType(t, "uint256[]")}, {Type: connectorDeltaType(t, "uint256")}}
	binding := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(4), 224), connectorDeltaOperand(recipes.Input, 1))
	var err error
	bound.Queries[1], err = args.Pack(tOut, erc4626.NewIERC4626().PackBalanceOf(recipient), []*big.Int{binding}, big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConnectorDeltas(bound, 3, account, recipient, tOut, tIn); err == nil {
		t.Fatal("bound balance owner accepted as a static recipient")
	}
	malformed := connectorDeltaRecipe(t, common.HexToHash("0x42"), account, recipient, tOut, tIn)
	malformed.Queries[1] = bytes.Clone(malformed.Queries[1][:10])
	if err := validateConnectorDeltas(malformed, 3, account, recipient, tOut, tIn); err == nil {
		t.Fatal("malformed balance query accepted")
	}
}
