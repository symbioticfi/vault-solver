// Package recipecapacity derives connector funding from the committed recipe.
package recipecapacity

import (
	"bytes"
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	erc20binding "github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

// Scope names one recipe and the connector account/output read at Block.
type Scope struct {
	ChainID                                                 *big.Int
	Executor, Account, Caller, Connector, RWA, FundingAsset common.Address
	Block                                                   *big.Int
}

type Result struct {
	Snapshot  recipes.Snapshot
	Recipe    recipes.Recipe
	MaxAssets *big.Int
}

var queryArguments = abi.Arguments{{Type: typeOf("address")}, {Type: typeOf("bytes")}, {Type: typeOf("uint256[]")}, {Type: typeOf("uint256")}}
var wordArguments = abi.Arguments{{Type: typeOf("uint256")}}
var callArguments = queryArguments[:3]
var witnessSelector = crypto.Keccak256([]byte("recipeFundingWitness((address,uint96,bytes,bytes,bytes32[])[])"))[:4]

func typeOf(name string) abi.Type {
	t, err := abi.NewType(name, "", nil)
	if err != nil {
		panic(err)
	}
	return t
}

// Read authenticates the original program before replaying its sequential funding
// prefix with the SDK's rollback-only code override at the same block.
func Read(ctx context.Context, reader recipes.Reader, scope Scope) (Result, error) {
	if scope.ChainID == nil || scope.ChainID.Sign() <= 0 || scope.Block == nil || !scope.Block.IsUint64() || scope.Executor == (common.Address{}) || scope.Account == (common.Address{}) || scope.Caller == (common.Address{}) || scope.Connector == (common.Address{}) || scope.RWA == (common.Address{}) || scope.FundingAsset == (common.Address{}) {
		return Result{}, errors.New("recipe capacity requires complete pinned scope")
	}
	snapshot, recipe, err := recipes.DiscoverForAccount(ctx, reader, scope.Executor, scope.Account, scope.Block)
	if err != nil {
		return Result{}, errors.Errorf("discover capacity recipe: %w", err)
	}
	if snapshot.Caller != scope.Caller {
		return Result{}, errors.New("capacity RecipeExecutor caller mismatch")
	}
	program, err := fundingPrefix(scope, snapshot, recipe)
	if err != nil {
		return Result{}, err
	}
	if err = recipes.VerifyRoles(ctx, reader, snapshot, program.leaves); err != nil {
		return Result{}, errors.Errorf("capacity recipe roles: %w", err)
	}
	inputs := map[uint]string{0: "1", 1: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)).String()}
	inspection, err := recipes.Inspect(program.recipe)
	if err != nil {
		return Result{}, err
	}
	// Pure-cash prefixes can omit Input0 entirely.
	allowed := make(map[uint]string, len(inspection.Inputs))
	for _, index := range inspection.Inputs {
		if v, ok := inputs[index]; ok {
			allowed[index] = v
		}
	}
	expressions := []recipes.Value{}
	for index, step := range program.recipe.Steps {
		if step[0] <= recipes.MulDiv {
			expressions = append(expressions, recipes.Value{Source: recipes.Result, Value: big.NewInt(int64(index))})
		}
	}
	values, err := recipes.ResolveValuesWithRuntime(ctx, reader, snapshot, program.recipe, expressions, recipes.PrefixRuntime{Tree: program.tree, Inputs: allowed})
	if err != nil {
		return Result{}, errors.Errorf("capacity funding prefix: %w", err)
	}
	maximum, ok := values[program.last]
	if !ok || maximum == nil || maximum.Sign() < 0 {
		return Result{}, errors.New("capacity funding prefix missing final balance")
	}
	binding := connector.NewLiquidLaneConnector()
	raw, err := reader.CallContract(ctx, ethereum.CallMsg{To: &scope.Connector, Data: binding.PackRemainingAssets(scope.RWA)}, scope.Block)
	if err != nil {
		return Result{}, errors.Errorf("connector remainingAssets: %w", err)
	}
	remaining, err := binding.UnpackRemainingAssets(raw)
	if err != nil {
		return Result{}, errors.Errorf("connector remainingAssets: %w", err)
	}
	if remaining.Cmp(maximum) < 0 {
		maximum = remaining
	}
	header, err := reader.HeaderByNumber(ctx, scope.Block)
	if err != nil || header == nil || header.Number == nil || header.Number.Cmp(scope.Block) != 0 || header.Hash() != snapshot.BlockHash {
		return Result{}, errors.New("capacity recipe block reorganized")
	}
	return Result{Snapshot: snapshot, Recipe: recipe, MaxAssets: new(big.Int).Set(maximum)}, nil
}

type fundingProgram struct {
	recipe recipes.Recipe
	tree   *recipes.Tree
	leaves []common.Hash
	last   uint
}

func fundingPrefix(scope Scope, snapshot recipes.Snapshot, recipe recipes.Recipe) (fundingProgram, error) {
	fail := func(err error) (fundingProgram, error) { return fundingProgram{}, err }
	hash, err := recipes.HashRecipe(recipe)
	if err != nil || hash != snapshot.RecipeHash {
		return fail(errors.New("capacity recipe hash mismatch"))
	}
	if len(recipe.Steps) > 4096 || len(recipe.Queries) > 4096 {
		return fail(errors.New("capacity recipe exceeds resource bounds"))
	}
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return fail(err)
	}
	tree, marker, err := witnessTree(scope, snapshot, recipe)
	if err != nil {
		return fail(err)
	}
	var approve *recipes.RuntimeSlot
	for _, slot := range inspection.Slots {
		if slot.Operation == recipes.Approve {
			copySlot := slot
			approve = &copySlot
			break
		}
	}
	if approve == nil || !isInput(approve.Amount, 1) || approve.Token != scope.FundingAsset || approve.Spender != scope.Connector {
		return fail(errors.New("capacity recipe payment approval mismatch"))
	}
	var swap *recipes.RuntimeSlot
	for _, slot := range inspection.Slots {
		if slot.StepIndex > approve.StepIndex {
			copySlot := slot
			swap = &copySlot
			break
		}
	}
	if swap == nil || swap.Operation != recipes.Manage || len(swap.Leaves) != 1 || !isInput(swap.Gate, 0) {
		return fail(errors.New("capacity recipe signed swap mismatch"))
	}
	partial := recipes.CloneRecipe(recipe)
	partial.Steps = partial.Steps[:swap.StepIndex+1]
	actions, err := recipes.ManagedActions(partial, tree)
	if err != nil {
		return fail(err)
	}
	selector := connector.NewLiquidLaneConnector().GetABI().Methods["swap"].ID
	found := false
	for _, action := range actions {
		if action.Leaf.Target == scope.Executor {
			return fail(errors.New("capacity prefix cannot call executor with overridden code"))
		}
		if action.Step != swap.StepIndex {
			continue
		}
		if action.Leaf.Target != scope.Connector || len(action.Leaf.Data) < 164 || !bytes.Equal(action.Leaf.Data[:4], selector) {
			return fail(errors.New("capacity signed swap route mismatch"))
		}

		if !bytes.Equal(action.Leaf.Data[4:36], common.LeftPadBytes([]byte{128}, 32)) {
			return fail(errors.New("capacity signed swap tuple offset must be 128"))
		}
		if err = action.Pinned(4, 32); err != nil {
			return fail(err)
		}
		if !bytes.Equal(action.Leaf.Data[132:164], common.LeftPadBytes(scope.RWA.Bytes(), 32)) {
			return fail(errors.New("capacity signed swap route mismatch"))
		}
		if err = action.Pinned(0, 4); err != nil {
			return fail(err)
		}
		if err = action.Pinned(132, 32); err != nil {
			return fail(err)
		}
		amount, e := action.Field(100)
		if e != nil || !isInput(amount, 0) {
			return fail(errors.New("capacity signed swap amount must bind Input0"))
		}
		found = true
	}
	if !found {
		return fail(errors.New("capacity signed swap witness missing"))
	}
	var leaves []common.Hash
	for _, slot := range inspection.Slots {
		if slot.StepIndex <= swap.StepIndex {
			leaves = append(leaves, slot.Leaves...)
		}
	}
	byHash := make(map[common.Hash]bool, len(tree.Leaves))
	for _, leaf := range tree.Leaves {
		byHash[leaf.Hash] = true
	}
	for _, leaf := range leaves {
		if !byHash[leaf] {
			return fail(errors.New("capacity permission witness missing"))
		}
	}
	prefix := recipes.CloneRecipe(recipe)
	prefix.Steps = prefix.Steps[:approve.StepIndex]
	for i := approve.StepIndex + 1; i < swap.StepIndex; i++ {
		if recipe.Steps[i][0] > recipes.LessOrEqual {
			return fail(errors.New("capacity payment boundary contains intervening effect"))
		}
	}
	for _, leaf := range tree.Leaves {
		if leaf.Hash != approve.Leaves[0] {
			continue
		}
		if leaf.Target != scope.FundingAsset || len(leaf.Data) != 68 || len(leaf.DataMask) != 68 {
			return fail(errors.New("capacity payment approval witness mismatch"))
		}
		formed := leaf
		formed.Data = erc20binding.NewERC20().PackApprove(scope.Connector, new(big.Int).SetBytes(leaf.Data[36:]))
		approvalHash, e := recipes.HashLeaf(formed)
		if e != nil || approvalHash != leaf.Hash {
			return fail(errors.New("capacity payment approval witness mismatch"))
		}
	}
	for _, step := range recipe.Steps {
		if step[0] != recipes.Read {
			continue
		}
		index, e := wordArguments.Unpack(step[1:])
		if e != nil || index[0].(*big.Int).Uint64() == uint64(marker) {
			return fail(errors.New("capacity witness query cannot be read"))
		}
	}
	for _, step := range prefix.Steps {
		if step[0] != recipes.Call {
			continue
		}
		args, e := callArguments.Unpack(step[1:])
		if e != nil || args[0].(common.Address) == scope.Executor {
			return fail(errors.New("capacity prefix cannot self-call executor"))
		}
	}
	data, err := queryArguments.Pack(scope.FundingAsset, erc4626.NewIERC4626().PackBalanceOf(scope.Account), []*big.Int{}, new(big.Int))
	if err != nil {
		return fail(err)
	}
	read, err := wordArguments.Pack(new(big.Int).SetUint64(uint64(len(prefix.Queries))))
	if err != nil {
		return fail(err)
	}
	prefix.Queries = append(prefix.Queries, data)
	last := uint(len(prefix.Steps))
	prefix.Steps = append(prefix.Steps, append([]byte{recipes.Read}, read...))
	leaves = leaves[:0]
	for _, leaf := range tree.Leaves {
		leaves = append(leaves, leaf.Hash)
	}
	return fundingProgram{recipe: prefix, tree: tree, leaves: leaves, last: last}, nil
}

func isInput(v recipes.Value, index uint64) bool {
	return v.Source == recipes.Input && v.Value != nil && v.Value.IsUint64() && v.Value.Uint64() == index
}

func witnessTree(scope Scope, snapshot recipes.Snapshot, recipe recipes.Recipe) (*recipes.Tree, int, error) {
	tree := &recipes.Tree{Root: snapshot.Root, ChainID: new(big.Int).Set(scope.ChainID), Account: &scope.Account}
	marker := -1
	seen := map[common.Hash]bool{}
	for i, query := range recipe.Queries {
		args, err := queryArguments.Unpack(query)
		if err != nil {
			return nil, 0, errors.Errorf("capacity query %d: %w", i, err)
		}
		data := args[1].([]byte)
		if len(data) < 4 || !bytes.Equal(data[:4], witnessSelector) {
			continue
		}
		if marker != -1 || args[0].(common.Address) != scope.Connector || len(args[2].([]*big.Int)) != 0 || args[3].(*big.Int).Sign() != 0 || len(data) > 2<<20 {
			return nil, 0, errors.New("capacity funding witness is ambiguous or malformed")
		}
		marker = i
		values, err := accountbinding.NewMerkleAccount().GetABI().Methods["manage"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, 0, errors.Errorf("capacity funding witness: %w", err)
		}
		actions := *abi.ConvertType(values[0], new([]accountbinding.IMerkleAccountAction)).(*[]accountbinding.IMerkleAccountAction)
		if len(actions) == 0 || len(actions) > 4096 {
			return nil, 0, errors.New("capacity funding witness size invalid")
		}
		for _, action := range actions {
			leaf := recipes.Leaf{Target: action.Target, Value: action.Value, Data: action.Data, DataMask: action.DataMask}
			for _, proof := range action.Proof {
				leaf.Proof = append(leaf.Proof, common.Hash(proof))
			}
			hash, err := recipes.HashLeaf(leaf)
			if err != nil || seen[hash] || !recipes.Proof(tree.Root, hash, leaf.Proof) || action.Value == nil || action.Value.Sign() != 0 {
				return nil, 0, errors.New("capacity witness has duplicate, invalid or stale proof")
			}
			leaf.Hash = hash
			seen[hash] = true
			tree.Leaves = append(tree.Leaves, leaf)
		}
	}
	if marker < 0 {
		return nil, 0, errors.New("capacity funding witness missing")
	}
	return tree, marker, nil
}
