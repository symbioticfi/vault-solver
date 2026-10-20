package rfq

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"gopkg.in/yaml.v3"
)

func TestGroupedExecutorConfiguration(t *testing.T) {
	const base = `backendUrl: https://rfq.example
stateDir: /persistent/rfq-merkle
executor: "0x0000000000000000000000000000000000000010"
curator: {url: https://curator.example/api/v1}
recipeExecutors:
  "0x0000000000000000000000000000000000000042":
    swapExecutor: "0x0000000000000000000000000000000000000043"
    redemptionExecutors:
      "0x00000000000000000000000000000000000000ab": "0x0000000000000000000000000000000000000044"
`
	for _, tc := range []struct{ name, text, want string }{
		{"shared plus issuer", base, ""},
		{"flat map rejected", strings.ReplaceAll(base, "    swapExecutor: \"0x0000000000000000000000000000000000000043\"\n    redemptionExecutors:\n      \"0x00000000000000000000000000000000000000ab\": \"0x0000000000000000000000000000000000000044\"", "    old: value"), "field old"},
		{"duplicate asset", base + "      \"0x00000000000000000000000000000000000000AB\": \"0x0000000000000000000000000000000000000045\"\n", "duplicate redemption asset"},
		{"shared issuer executor", strings.ReplaceAll(base, "0000000044", "0000000043"), "distinct executors"},
		{"empty route", strings.ReplaceAll(base, "    swapExecutor: \"0x0000000000000000000000000000000000000043\"\n    redemptionExecutors:\n      \"0x00000000000000000000000000000000000000ab\": \"0x0000000000000000000000000000000000000044\"", "    redemptionExecutors: {}"), "requires a swap or redemption executor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var n yaml.Node
			if err := yaml.Unmarshal([]byte(tc.text), &n); err != nil {
				t.Fatal(err)
			}
			cfg, err := parseConfig(n)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted %s: %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			routes := cfg.RecipeExecutors[common.HexToAddress("0x42")]
			if got, _ := routes.forToken(common.HexToAddress("0xab")); got != common.HexToAddress("0x44") {
				t.Fatal("issuer route did not override shared custody route")
			}
			if got, _ := routes.forToken(common.HexToAddress("0xac")); got != common.HexToAddress("0x43") {
				t.Fatal("swap-only route did not use shared executor")
			}
		})
	}
}

func TestSharedConnectorSelectsOnlyPinnedAssetPair(t *testing.T) {
	account, recipient := common.HexToAddress("0x21"), common.HexToAddress("0x10")
	terms, err := parseConnectorDiscount(connectorDiscountFixture(), *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	tokens := []common.Address{tIn, common.HexToAddress("0x987")}
	tree := &recipes.Tree{}
	var recipe recipes.Recipe
	for route, token := range tokens {
		signed := terms.Swap
		signed.Discount.TokenToRedeem = token
		data := connectorBinding.PackSwap(signed, terms.Signature, recipient, big.NewInt(0))
		mask := bytes.Repeat([]byte{255}, len(data))
		clear(mask[:4])
		clear(mask[132:164])
		leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
		leaf.Hash, err = recipes.HashLeaf(leaf)
		if err != nil {
			t.Fatal(err)
		}
		tree.Leaves = append(tree.Leaves, leaf)
		part := connectorDeltaRecipe(t, leaf.Hash, account, recipient, tOut, token)
		if route > 0 {
			offset := int64(len(recipe.Steps))
			queryOffset := int64(len(recipe.Queries))
			for i, step := range part.Steps {
				switch step[0] {
				case recipes.Read:
					step[32] += byte(queryOffset)
				case recipes.Manage:
					uintType := connectorDeltaType(t, "uint256")
					payload, e := (abi.Arguments{{Type: uintType}, {Type: connectorDeltaType(t, "bytes32[]")}, {Type: connectorDeltaType(t, "uint256[][]")}}).Pack(connectorDeltaOperand(recipes.Input, 2), [][32]byte{leaf.Hash}, [][]*big.Int{{new(big.Int).Or(new(big.Int).Lsh(big.NewInt(100), 224), connectorDeltaOperand(recipes.Input, 2))}})
					if e != nil {
						t.Fatal(e)
					}
					part.Steps[i] = append([]byte{recipes.Manage}, payload...)
				default:
					for _, at := range []int{1, 33} {
						word := step[at : at+32]
						switch word[4] {
						case recipes.Result:
							word[31] += byte(offset)
						case recipes.Input:
							word[31] += 2
						}
					}
				}
			}
		}
		recipe.Queries = append(recipe.Queries, part.Queries...)
		recipe.Steps = append(recipe.Steps, part.Steps...)
	}
	first, second := tree.Leaves[0].Hash, tree.Leaves[1].Hash
	if bytes.Compare(first[:], second[:]) > 0 {
		first, second = second, first
	}
	tree.Root = crypto.Keccak256Hash(first[:], second[:])
	tree.Leaves[0].Proof = []common.Hash{tree.Leaves[1].Hash}
	tree.Leaves[1].Proof = []common.Hash{tree.Leaves[0].Hash}
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := recipes.Snapshot{Executor: common.HexToAddress("0x43"), RecipeHash: hash, Account: account}
	for index, token := range tokens {
		signed := terms.Swap
		signed.Discount.TokenToRedeem = token
		assembled, err := assembleConnectorRoute(snapshot, recipe, tree, vlt, token, tOut, recipient, big.NewInt(123), big.NewInt(456), signed, terms.Signature)
		if err != nil {
			t.Fatal(err)
		}
		for i, value := range assembled.Route.Inputs {
			want := int64(0)
			if i == index*2 {
				want = 123
			}
			if i == index*2+1 {
				want = 456
			}
			if value.Int64() != want {
				t.Fatalf("route%d input%d=%s want%d", index, i, value, want)
			}
		}
	}
	_, err = assembleConnectorRoute(snapshot, recipe, tree, vlt, common.HexToAddress("0x999"), tOut, recipient, big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil || !strings.Contains(err.Error(), "exactly one selected") {
		t.Fatalf("unknown asset accepted: %v", err)
	}
	bad := recipes.CloneRecipe(recipe)
	clear(bad.Steps[16][1:33])
	bad.Steps[16][32] = 1
	_, err = assembleConnectorRoute(snapshot, bad, tree, vlt, tokens[0], tOut, recipient, big.NewInt(123), big.NewInt(456), terms.Swap, terms.Signature)
	if err == nil || !strings.Contains(err.Error(), "gate must equal") {
		t.Fatalf("foreign gate accepted: %v", err)
	}
}
