package rfq

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

func TestFrontendRecipeCatalog(t *testing.T) {
	path := os.Getenv("LL_RECIPE_MATRIX_PATH")
	if path == "" {
		t.Skip("set LL_RECIPE_MATRIX_PATH to the curator-frontend catalog export")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile(filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ChainID   int             `json:"chainId"`
		Route     string          `json:"route"`
		Rwa       string          `json:"rwa"`
		Connector string          `json:"connector"`
		Payment   string          `json:"payment"`
		Account   string          `json:"account"`
		Redeem    string          `json:"redeem"`
		Set       json.RawMessage `json:"set"`
		Recipe    hexutil.Bytes   `json:"recipe"`
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		t.Run(row.Route+"/"+row.Redeem, func(t *testing.T) {
			tree, err := recipes.DecodeTree(row.Set)
			if err != nil {
				t.Fatal(err)
			}
			recipe, err := recipes.DecodeRecipe(row.Recipe)
			if err != nil {
				t.Fatal(err)
			}
			venue, asset, err := paymentConnector(recipe, tree)
			if err != nil {
				t.Fatal(err)
			}
			if venue != common.HexToAddress(row.Connector) || asset != common.HexToAddress(row.Payment) {
				t.Fatal("wrong payment route")
			}
			actions, err := recipes.ManagedActions(recipe, tree)
			if err != nil {
				t.Fatal(err)
			}
			var signed connector.ILiquidLaneConnectorDiscountSwap
			var signature []byte
			for _, action := range actions {
				if action.Leaf.Target != common.HexToAddress(row.Connector) {
					continue
				}
				values, err := connectorBinding.GetABI().Methods["swap"].Inputs.Unpack(action.Leaf.Data[4:])
				if err != nil {
					t.Fatal(err)
				}
				signed = *abi.ConvertType(values[0], new(connector.ILiquidLaneConnectorDiscountSwap)).(*connector.ILiquidLaneConnectorDiscountSwap)
				signature = values[1].([]byte)
			}
			signed.Discount.TokenToRedeem = common.HexToAddress(row.Rwa)
			before, _ := recipes.HashRecipe(recipe)
			result, err := assembleConnectorRoute(recipe, tree, common.HexToAddress(row.Connector), common.HexToAddress(row.Rwa), common.HexToAddress("0x10"), big.NewInt(100), big.NewInt(90), signed, signature)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := recipes.HashRecipe(recipe)
			if before != after {
				t.Fatal("recipe changed")
			}
			if !bytes.Equal(result.Calldata[:4], recipeBinding.GetABI().Methods["execute"].ID) {
				t.Fatal("wrong selector")
			}
		})
	}
}
