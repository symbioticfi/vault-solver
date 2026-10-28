package rfq

import (
	"context"
	"math/big"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	erc20binding "github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	executorbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestConnectorQuoteRecomputesCommittedRecipeCapacity(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		advertised, reserved, want int64
		fail                       bool
	}{
		{"funded by recipe", 1000, 0, 600, false},
		{"upstream cap", 400, 0, 400, false},
		{"shared account reservation", 1000, 100, 500, false},
		{"unavailable does not fall back", 1000, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connectorAddress, account := common.HexToAddress("0x42"), common.HexToAddress("0x21")
			tokenIn, tokenOut := common.HexToAddress("0x31"), common.HexToAddress("0x32")
			service := newConnectorCapacityRPC(t, connectorAddress, account, tokenIn, tokenOut)
			service.fail = tc.fail
			server := rpc.NewServer()
			if err := server.RegisterName("eth", service); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(server.Stop)
			httpServer := httptest.NewServer(server)
			t.Cleanup(httpServer.Close)
			client, err := chain.Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x66").Hex(), 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			r := newReader(client, logr.Discard(), common.Address{})
			r.rfqCaller = service.caller
			id := common.HexToHash("0x11")
			inventory := []solverInventory{{Route: liquidlane.NewRoute(1, connectorAddress, account, tokenIn, tokenOut, 6, 6), Kind: "connector", RecipeExecutor: service.executor, DiscountID: &id, Discount: big.NewInt(1), DiscountPrecision: connectorDiscountPrecision, MaxAssets: big.NewInt(tc.advertised)}}
			pending := func(rows []solverInventory) liquidlane.CapacityReservations {
				if len(rows) != 1 || rows[0].Vault != account || rows[0].BlockNumber != 7 {
					t.Fatal("capacity lost pinned account identity")
				}
				return liquidlane.CapacityReservations{liquidlane.NewCapacityID(1, account, tokenOut): big.NewInt(tc.reserved)}
			}
			candidates, err := r.readConnectorCandidates(t.Context(), inventory, tokenIn, tokenOut, big.NewInt(100), pending)
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "recipe") {
					t.Fatalf("expected capacity failure: %v", err)
				}
			} else if err != nil || len(candidates) != 1 || candidates[0].MaxAmountOut.Int64() != tc.want {
				t.Fatalf("candidates=%+v, want %d: %v", candidates, tc.want, err)
			}
			service.mu.Lock()
			defer service.mu.Unlock()
			if service.capacityReads != 1 || service.balanceReads != 0 {
				t.Fatalf("capacity reads=%d cash reconstruction=%d", service.capacityReads, service.balanceReads)
			}
		})
	}
}

type connectorCapacityRPC struct {
	mu                          sync.Mutex
	connector, account, output  common.Address
	capacityReads, balanceReads int
	executor, caller            common.Address
	recipe                      recipes.Recipe
	hash, root                  common.Hash
	fail                        bool
}

func (*connectorCapacityRPC) ChainId(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}
func (*connectorCapacityRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}, nil
}
func (s *connectorCapacityRPC) Call(_ context.Context, args map[string]any, block string, override *map[string]any) (hexutil.Bytes, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.call(args, block, override)
}

func (s *connectorCapacityRPC) call(args map[string]any, block string, override *map[string]any) (hexutil.Bytes, error) {
	raw, _ := args["input"].(string)
	if raw == "" {
		raw, _ = args["data"].(string)
	}
	data, err := hexutil.Decode(raw)
	if err != nil {
		return nil, err
	}
	to := common.HexToAddress(args["to"].(string))

	if to == s.executor || to == s.account {
		if block != "0x7" {
			return nil, errors.New("unpinned recipe read")
		}
		if override != nil {
			s.capacityReads++
			if s.fail {
				return nil, errors.New("recipe source unavailable")
			}
			typ, _ := abi.NewType("uint256[]", "", nil)
			return (abi.Arguments{{Type: typ}}).Pack([]*big.Int{big.NewInt(600)})
		}
		a := executorbinding.NewRecipeExecutor().GetABI()
		if to == s.account {
			a = accountbinding.NewMerkleAccount().GetABI()
		}
		m, e := a.MethodById(data)
		if e != nil {
			return nil, e
		}
		switch m.RawName {
		case "account":
			return m.Outputs.Pack(s.account)
		case "caller":
			return m.Outputs.Pack(s.caller)
		case "recipeHash":
			return m.Outputs.Pack(s.hash)
		case "recipeBlock":
			return m.Outputs.Pack(big.NewInt(7))
		case "root":
			return m.Outputs.Pack(s.root)
		case "hasRole":
			return m.Outputs.Pack(true)
		}
		return nil, errors.New("unexpected recipe read")
	}
	if len(data) >= 4 && hexutil.Encode(data[:4]) == "0x313ce567" {
		return common.LeftPadBytes([]byte{6}, 32), nil
	}
	if to != s.connector {
		b := multicall3.NewMulticall3()
		contractABI := b.GetABI()
		method, err := contractABI.MethodById(data)
		if err != nil {
			return nil, err
		}
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		calls := *abi.ConvertType(values[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
		results := make([]multicall3.Multicall3Result, len(calls))
		for i := range results {
			result, err := s.call(map[string]any{"to": calls[i].Target.Hex(), "input": hexutil.Encode(calls[i].CallData)}, block, nil)
			if err != nil {
				return nil, err
			}
			results[i] = multicall3.Multicall3Result{Success: true, ReturnData: result}
		}
		return method.Outputs.Pack(results)
	}
	if block != "0x7" {
		return nil, errors.New("connector read escaped pinned block")
	}
	contractABI := connector.NewLiquidLaneConnector().GetABI()
	method, err := contractABI.MethodById(data)
	if err != nil {
		return nil, err
	}
	switch method.RawName {
	case "eip712Domain":
		return method.Outputs.Pack([1]byte{15}, "LiquidLaneConnector", "1", big.NewInt(1), s.connector, [32]byte{}, []*big.Int{})
	case "account":
		return method.Outputs.Pack(s.account)
	case "outputAsset":
		return method.Outputs.Pack(s.output)
	case "remainingAssets":
		return method.Outputs.Pack(big.NewInt(1000))
	case "minDiscount":
		return method.Outputs.Pack(big.NewInt(0))
	case "getAmountOut":
		values, err := method.Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		return method.Outputs.Pack(values[1])
	default:
		s.balanceReads++
		return nil, errors.New("unexpected cash or budget reconstruction")
	}
}

func (s *connectorCapacityRPC) GetLogs(context.Context, map[string]any) ([]types.Log, error) {
	event := executorbinding.NewRecipeExecutor().GetABI().Events["SetRecipe"]
	data, err := event.Inputs.NonIndexed().Pack(s.recipe.Queries, s.recipe.Steps)
	header := &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}
	return []types.Log{{Address: s.executor, Topics: []common.Hash{event.ID, s.hash}, Data: data, BlockNumber: 7, BlockHash: header.Hash()}}, err
}
func newConnectorCapacityRPC(t *testing.T, connectorAddress, account, tokenIn, output common.Address) *connectorCapacityRPC {
	t.Helper()
	typ := func(name string) abi.Type {
		v, e := abi.NewType(name, "", nil)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	executorAddress, caller := common.HexToAddress("0x43"), common.HexToAddress("0x44")
	approvalData := erc20binding.NewERC20().PackApprove(connectorAddress, new(big.Int))
	approvalMask := make([]byte, 68)
	for i := 36; i < 68; i++ {
		approvalMask[i] = 255
	}
	swapData := make([]byte, 164)
	copy(swapData, connector.NewLiquidLaneConnector().GetABI().Methods["swap"].ID)
	big.NewInt(128).FillBytes(swapData[4:36])
	copy(swapData[132:164], common.LeftPadBytes(tokenIn.Bytes(), 32))
	swapMask := make([]byte, 164)
	for i := 100; i < 132; i++ {
		swapMask[i] = 255
	}
	approval := recipes.Leaf{Target: output, Value: new(big.Int), Data: approvalData, DataMask: approvalMask}
	swap := recipes.Leaf{Target: connectorAddress, Value: new(big.Int), Data: swapData, DataMask: swapMask}
	var err error
	approval.Hash, err = recipes.HashLeaf(approval)
	if err != nil {
		t.Fatal(err)
	}
	swap.Hash, err = recipes.HashLeaf(swap)
	if err != nil {
		t.Fatal(err)
	}
	a, b := approval.Hash, swap.Hash
	if a.Cmp(b) > 0 {
		a, b = b, a
	}
	root := crypto.Keccak256Hash(a[:], b[:])
	actions := []accountbinding.IMerkleAccountAction{{Target: output, Value: new(big.Int), Data: approvalData, DataMask: approvalMask, Proof: [][32]byte{swap.Hash}}, {Target: connectorAddress, Value: new(big.Int), Data: swapData, DataMask: swapMask, Proof: [][32]byte{approval.Hash}}}
	manageData := accountbinding.NewMerkleAccount().PackManage(actions)
	marker := append(crypto.Keccak256([]byte("recipeFundingWitness((address,uint96,bytes,bytes,bytes32[])[])"))[:4], manageData[4:]...)
	query, err := (abi.Arguments{{Type: typ("address")}, {Type: typ("bytes")}, {Type: typ("uint256[]")}, {Type: typ("uint256")}}).Pack(connectorAddress, marker, []*big.Int{}, new(big.Int))
	if err != nil {
		t.Fatal(err)
	}
	operand := func(index int64) *big.Int {
		return new(big.Int).Or(new(big.Int).Lsh(big.NewInt(1), 216), big.NewInt(index))
	}
	approve, err := (abi.Arguments{{Type: typ("uint256")}, {Type: typ("address")}, {Type: typ("address")}, {Type: typ("bytes32")}}).Pack(operand(1), output, connectorAddress, approval.Hash)
	if err != nil {
		t.Fatal(err)
	}
	swapStep, err := (abi.Arguments{{Type: typ("uint256")}, {Type: typ("bytes32[]")}, {Type: typ("uint256[][]")}}).Pack(operand(0), [][32]byte{swap.Hash}, [][]*big.Int{{new(big.Int).Or(new(big.Int).Lsh(big.NewInt(100), 224), operand(0))}})
	if err != nil {
		t.Fatal(err)
	}
	recipe := recipes.Recipe{Queries: [][]byte{query}, Steps: [][]byte{append([]byte{recipes.Approve}, approve...), append([]byte{recipes.Manage}, swapStep...)}}
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return &connectorCapacityRPC{connector: connectorAddress, account: account, output: output, executor: executorAddress, caller: caller, recipe: recipe, hash: hash, root: root}
}
