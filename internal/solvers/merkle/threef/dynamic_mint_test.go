package threef

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	executor "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/threef"
)

func dynamicMintFixture(t *testing.T, computed bool) (provider.Snapshot, recipes.Recipe, *recipes.Tree) {
	t.Helper()
	raw, err := os.ReadFile("testdata/3f-dynamic-mint.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Target common.Address `json:"target"`
		Data   hexutil.Bytes  `json:"data"`
		Mask   hexutil.Bytes  `json:"dataMask"`
		Hash   common.Hash    `json:"hash"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	leaf := recipes.Leaf{Target: doc.Target, Data: doc.Data, DataMask: doc.Mask, Hash: doc.Hash, Value: new(big.Int)}
	base := new(big.Int).SetBytes(leaf.Data[68:100]).Uint64() + 36
	bindings := make([]*big.Int, 3)
	for i := range bindings {
		// Inner principal/yield/request map to independent outer inputs 7/8/9.
		bindings[i] = new(big.Int).Lsh(new(big.Int).SetUint64(base+32*uint64(i)), 224)
		source, value := recipes.Input, uint64(7+i)
		if computed && i < 2 {
			source, value = recipes.Result, uint64(i)
		}
		bindings[i].Or(bindings[i], encodedValue(source, value))
	}
	args := abi.Arguments{{Type: uintType}, {Type: instructionType("bytes32[]")}, {Type: instructionType("uint256[][]")}}
	data, err := args.Pack(encodedValue(recipes.Literal, 1), []common.Hash{leaf.Hash}, [][]*big.Int{bindings})
	if err != nil {
		t.Fatal(err)
	}
	r := recipes.Recipe{Steps: [][]byte{append([]byte{recipes.Manage}, data...)}}
	if computed {
		for i := range uint64(2) {
			query, err := queryArgs.Pack(common.HexToAddress("0x91"), requestABI.PackMintAuthorization(doc.Target), []*big.Int{}, new(big.Int).SetUint64(32*i))
			if err != nil {
				t.Fatal(err)
			}
			r.Queries = append(r.Queries, query)
		}
		r.Steps = append([][]byte{abiRead(0), abiRead(1)}, r.Steps...)
	}
	return provider.Snapshot{ChainID: big.NewInt(11155111), Account: doc.Target, BlockNumber: big.NewInt(10), Timestamp: 1000}, r, &recipes.Tree{Root: leaf.Hash, Leaves: []recipes.Leaf{leaf}}
}

func TestDynamicMintReadsCommittedInputBindings(t *testing.T) {
	pin, recipe, tree := dynamicMintFixture(t, false)
	markets, err := inspect(pin, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(markets) != 1 || uint(markets[0].Principal.Value.Uint64()) != 7 || uint(markets[0].Yield.Value.Uint64()) != 8 || markets[0].RequestInput != 9 || markets[0].UnderlyingAsset != (common.Address{}) {
		t.Fatalf("unexpected dynamic mint: %+v", markets)
	}
	if markets[0].Policy != common.HexToAddress("0x92a2c58ea13f218bc180262518bf55a0d7220b0c") || markets[0].Asset != common.HexToAddress("0x809e6c18bc13cc7c9f7b8000d74243baacce84d7") {
		t.Fatalf("wrong policy or funding token: %+v", markets[0])
	}
}

func TestDynamicMintResolvesAndAttestsPendingAuctionPair(t *testing.T) {
	for _, computed := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime amounts", true: "onchain amounts"}[computed], func(t *testing.T) {
			pin, recipe, tree := dynamicMintFixture(t, computed)
			markets, err := inspect(pin, recipe, tree)
			if err != nil {
				t.Fatal(err)
			}
			market := markets[0]
			now := time.Unix(1000, 0).UTC()
			deadline := now.Add(2 * time.Minute).Format(time.RFC3339)
			request, manager, collateral, underlying := common.HexToAddress("0x91"), common.HexToAddress("0x92"), common.HexToAddress("0x93"), common.HexToAddress("0x94")
			auction := testAuction(testMarket(), 1, request)
			auction.Status = "SOLVED"
			auction.DepositAsset.Set(&threef.AuctionDepositAssetDto{Address: market.Asset.Hex()})
			auction.Asset.Set(&threef.ResolvedAssetDto{PositionManager: manager.Hex()})
			api := &fakeAPI{auctions: []threef.AuctionDto{auction}, offers: []threef.OfferDto{{Maker: pin.Account.Hex(), Status: "AWAITING_MINT", RequestId: request.Hex(), ExecutionMode: "PUSH", MintDeadline: *threef.NewNullableString(&deadline)}}}
			attested := false
			chain := readFunc(func(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
				if block.Cmp(pin.BlockNumber) != 0 {
					t.Fatal("lost snapshot pin")
				}
				switch *msg.To {
				case manager:
					return positionManagerABI.GetABI().Methods["assets"].Outputs.Pack(collateral, common.Address{})
				case collateral:
					return underlyingAssetABI.GetABI().Methods["underlying"].Outputs.Pack(underlying)
				case market.Asset:
					return tokenABI.GetABI().Methods["balanceOf"].Outputs.Pack(big.NewInt(100))
				case market.Policy:
					if string(msg.Data[:4]) == string(policySchema.Methods["remainingAssets"].ID) {
						return policySchema.Methods["remainingAssets"].Outputs.Pack(big.NewInt(50))
					}
					return policySchema.Methods["minYieldPerRequest"].Outputs.Pack(big.NewInt(2_000_000))
				case request:
					return requestABI.GetABI().Methods["mintAuthorization"].Outputs.Pack(big.NewInt(12), big.NewInt(5))
				case pin.Account:
					attested = true
					return capabilitySchema.Methods["execute"].Outputs.Pack([]*big.Int{big.NewInt(50), new(big.Int), big.NewInt(49), big.NewInt(1), new(big.Int)})
				default:
					t.Fatal("unexpected call", msg.To)
					return nil, nil
				}
			})
			p := mintProvider{Policy: market.Policy, API: api, Chain: chain, Now: func() time.Time { return now }}
			out, err := p.PrepareRecipe(t.Context(), pin, recipe, tree, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 1 || out[0].Inputs[9] != "145" || !attested {
				t.Fatalf("wrong authorized inputs: %+v, attested %v", out, attested)
			}
			record, err := decodeMint(out[0])
			if err != nil || record.UnderlyingAsset != underlying {
				t.Fatalf("wrong policy pair: %+v, %v", record, err)
			}
			if !computed {
				if out[0].Inputs[7] != "12" || out[0].Inputs[8] != "5" {
					t.Fatalf("wrong amount inputs: %+v", out[0])
				}
				return
			}
			if len(out[0].Inputs) != 1 || out[0].Results[0] != "12" || out[0].Results[1] != "5" {
				t.Fatalf("expected request input and native amount witnesses: %+v", out[0])
			}
			calldata, _, err := recipes.Assemble(recipe, tree, out[0].Inputs, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, principal := range []int64{12, 11} {
				result, err := executor.NewRecipeExecutor().GetABI().Methods["execute"].Outputs.Pack([]*big.Int{big.NewInt(principal), big.NewInt(5), new(big.Int)})
				if err != nil {
					t.Fatal(err)
				}
				err = recipes.VerifyResults(calldata, result, out[0].Results)
				if (err != nil) != (principal != 12) {
					t.Fatalf("amount witness mismatch: %v", err)
				}
			}
		})
	}
}
