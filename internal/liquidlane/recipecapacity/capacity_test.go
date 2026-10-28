package recipecapacity

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	erc20binding "github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	executorbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

func TestReadCapacityAuthenticatesAndReplaysOnePrefix(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		cash, remaining, want int64
	}{
		{"shared source liquidity is sequential", 600, 1000, 600}, {"remaining reservation clips funding", 600, 400, 400}, {"zero funding", 0, 1000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, r := capacityFixture(t, true)
			r.cash = tc.cash
			r.remaining = tc.remaining
			got, err := Read(t.Context(), r, scope)
			if err != nil || got.MaxAssets.Int64() != tc.want {
				t.Fatalf("capacity=%+v err=%v", got, err)
			}
			if r.overrides != 1 || r.roles != 4 {
				t.Fatalf("override=%d roles=%d", r.overrides, r.roles)
			}
		})
	}
	scope, r := capacityFixture(t, false)
	got, err := Read(t.Context(), r, scope)
	if err != nil || got.MaxAssets.Int64() != 600 {
		t.Fatalf("pure cash capacity=%+v err=%v", got, err)
	}
}

func TestCapacityRejectsUnpinnedOrRelocatedSignedSwapTuple(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*recipes.Leaf)
		want   string
	}{
		{"relocated tuple with spoofed route", func(leaf *recipes.Leaf) {
			data := append(bytes.Clone(leaf.Data[:132]), leaf.Data[132:164]...)
			data = append(data, leaf.Data[132:]...)
			big.NewInt(160).FillBytes(data[4:36])
			new(big.Int).Add(new(big.Int).SetBytes(leaf.Data[36:68]), big.NewInt(32)).FillBytes(data[36:68])
			otherRWA := common.HexToAddress("0xff")
			copy(data[164:196], common.LeftPadBytes(otherRWA.Bytes(), 32))
			mask := append(bytes.Clone(leaf.DataMask[:132]), make([]byte, 32)...)
			mask = append(mask, leaf.DataMask[132:]...)
			leaf.Data, leaf.DataMask = data, mask
			decoded, err := connector.NewLiquidLaneConnector().GetABI().Methods["swap"].Inputs.Unpack(data[4:])
			if err != nil {
				t.Fatal(err)
			}
			actual := *abi.ConvertType(decoded[0], new(connector.ILiquidLaneConnectorDiscountSwap)).(*connector.ILiquidLaneConnectorDiscountSwap)
			if actual.Discount.TokenToRedeem != otherRWA {
				t.Fatal("relocated tuple fixture did not encode the other RWA")
			}
		}, "tuple offset"},
		{"mutable tuple header", func(leaf *recipes.Leaf) {
			for i := 4; i < 36; i++ {
				leaf.DataMask[i] = 255
			}
		}, "unconstrained"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, r := capacityFixture(t, true, tc.mutate)
			// Both attacks have valid hashes, proofs and original recipe commitments.
			_, _, err := witnessTree(scope, recipes.Snapshot{Root: r.root}, r.recipe)
			if err != nil {
				t.Fatalf("regression fixture has invalid witness: %v", err)
			}
			_, err = Read(t.Context(), r, scope)
			if err == nil || !strings.Contains(err.Error(), tc.want) || r.overrides != 0 {
				t.Fatalf("route header accepted: overrides=%d err=%v", r.overrides, err)
			}
		})
	}
}

func TestCapacityFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Scope, *capacityReader)
		want   string
	}{
		{"missing executor", func(s *Scope, _ *capacityReader) { s.Executor = common.Address{} }, "scope"},
		{"wrong caller", func(s *Scope, _ *capacityReader) { s.Caller = common.HexToAddress("0xff") }, "caller"},
		{"stale proof", func(_ *Scope, r *capacityReader) { r.root = common.HexToHash("0xff") }, "proof"},
		{"revoked role", func(_ *Scope, r *capacityReader) { r.denied = true }, "role"},
		{"source unavailable", func(_ *Scope, r *capacityReader) { r.overrideError = true }, "simulation"},
		{"recipe changed", func(_ *Scope, r *capacityReader) { r.hash = common.HexToHash("0xff") }, "hash"},
		{"missing witness", func(_ *Scope, r *capacityReader) { r.recipe.Queries = nil; r.rehash(t) }, "witness"},
		{"ambiguous witness", func(_ *Scope, r *capacityReader) {
			r.recipe.Queries = append(r.recipe.Queries, r.recipe.Queries[0])
			r.rehash(t)
		}, "ambiguous"},
		{"duplicate permission preimage", func(_ *Scope, r *capacityReader) {
			r.changeWitness(t, func(a []accountbinding.IMerkleAccountAction) []accountbinding.IMerkleAccountAction {
				return append(a, a[0])
			})
		}, "duplicate"},
		{"missing source permission", func(_ *Scope, r *capacityReader) {
			r.changeWitness(t, func(a []accountbinding.IMerkleAccountAction) []accountbinding.IMerkleAccountAction { return a[:2] })
		}, "absent"},
		{"missing approval permission", func(_ *Scope, r *capacityReader) {
			r.changeWitness(t, func(a []accountbinding.IMerkleAccountAction) []accountbinding.IMerkleAccountAction { return a[1:] })
		}, "missing"},
		{"self call under override", func(_ *Scope, r *capacityReader) {
			data, e := callArguments.Pack(r.scope.Executor, []byte{1, 2, 3, 4}, []*big.Int{})
			if e != nil {
				t.Fatal(e)
			}
			r.recipe.Steps = append([][]byte{append([]byte{recipes.Call}, data...)}, r.recipe.Steps...)
			r.rehash(t)
		}, "self-call"},
		{"self query under override", func(_ *Scope, r *capacityReader) {
			data, e := queryArguments.Pack(r.scope.Executor, executorbinding.NewRecipeExecutor().PackCaller(), []*big.Int{}, new(big.Int))
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := wordArguments.Pack(big.NewInt(int64(len(r.recipe.Queries))))
			r.recipe.Queries = append(r.recipe.Queries, data)
			r.recipe.Steps = append([][]byte{append([]byte{recipes.Read}, raw...)}, r.recipe.Steps...)
			r.rehash(t)
		}, "self-query"},

		{"marker executed", func(_ *Scope, r *capacityReader) {
			raw, _ := wordArguments.Pack(new(big.Int))
			r.recipe.Steps = append(r.recipe.Steps, append([]byte{recipes.Read}, raw...))
			r.rehash(t)
		}, "cannot be read"},
		{"wrong RWA", func(s *Scope, _ *capacityReader) { s.RWA = common.HexToAddress("0xff") }, "route"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := capacityFixture(t, true)
			tc.mutate(&s, r)
			_, err := Read(t.Context(), r, s)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want=%s", err, tc.want)
			}
		})
	}
}

type capacityReader struct {
	scope                 Scope
	recipe                recipes.Recipe
	root, hash            common.Hash
	header                *types.Header
	cash, remaining       int64
	roles, overrides      int
	denied, overrideError bool
	prefixActions         int
}

func (r *capacityReader) changeWitness(t *testing.T, change func([]accountbinding.IMerkleAccountAction) []accountbinding.IMerkleAccountAction) {
	t.Helper()
	values, err := queryArguments.Unpack(r.recipe.Queries[0])
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := accountbinding.NewMerkleAccount().GetABI().Methods["manage"].Inputs.Unpack(values[1].([]byte)[4:])
	if err != nil {
		t.Fatal(err)
	}
	actions := *abi.ConvertType(decoded[0], new([]accountbinding.IMerkleAccountAction)).(*[]accountbinding.IMerkleAccountAction)
	encoded := accountbinding.NewMerkleAccount().PackManage(change(actions))
	values[1] = append(bytes.Clone(witnessSelector), encoded[4:]...)
	r.recipe.Queries[0], err = queryArguments.Pack(values...)
	if err != nil {
		t.Fatal(err)
	}
	r.rehash(t)
}

func (r *capacityReader) rehash(t *testing.T) {
	t.Helper()
	var err error
	r.hash, err = recipes.HashRecipe(r.recipe)
	if err != nil {
		t.Fatal(err)
	}
}
func (r *capacityReader) HeaderByNumber(_ context.Context, b *big.Int) (*types.Header, error) {
	if b != nil && b.Int64() != 7 {
		return nil, errors.New("block mismatch")
	}
	return types.CopyHeader(r.header), nil
}
func (r *capacityReader) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	event := executorbinding.NewRecipeExecutor().GetABI().Events["SetRecipe"]
	if len(q.Addresses) != 1 || q.Addresses[0] != r.scope.Executor || q.FromBlock.Int64() != 7 || q.ToBlock.Int64() != 7 {
		return nil, errors.New("log scope mismatch")
	}
	data, err := event.Inputs.NonIndexed().Pack(r.recipe.Queries, r.recipe.Steps)
	return []types.Log{{Address: r.scope.Executor, BlockHash: r.header.Hash(), BlockNumber: 7, Topics: []common.Hash{event.ID, r.hash}, Data: data}}, err
}
func (r *capacityReader) CallContract(_ context.Context, msg ethereum.CallMsg, b *big.Int) ([]byte, error) {
	if b == nil || b.Int64() != 7 || msg.To == nil {
		return nil, errors.New("unpinned read")
	}
	var contract abi.ABI
	switch *msg.To {
	case r.scope.Executor:
		contract = executorbinding.NewRecipeExecutor().GetABI()
	case r.scope.Account:
		contract = accountbinding.NewMerkleAccount().GetABI()
	case r.scope.Connector:
		contract = connector.NewLiquidLaneConnector().GetABI()
	default:
		return nil, errors.New("unexpected read")
	}
	m, err := contract.MethodById(msg.Data)
	if err != nil {
		return nil, err
	}
	switch m.RawName {
	case "account":
		return m.Outputs.Pack(r.scope.Account)
	case "caller":
		return m.Outputs.Pack(r.scope.Caller)
	case "recipeHash":
		return m.Outputs.Pack(r.hash)
	case "recipeBlock":
		return m.Outputs.Pack(big.NewInt(7))
	case "root":
		return m.Outputs.Pack(r.root)
	case "hasRole":
		r.roles++
		args, e := m.Inputs.Unpack(msg.Data[4:])
		if e != nil || args[1].(common.Address) != r.scope.Executor {
			return nil, errors.New("role holder mismatch")
		}
		return m.Outputs.Pack(!r.denied)
	case "remainingAssets":
		return m.Outputs.Pack(big.NewInt(r.remaining))
	default:
		return nil, errors.New("unexpected getter")
	}
}
func (r *capacityReader) CallContractWithCode(_ context.Context, msg ethereum.CallMsg, b *big.Int, code []byte) ([]byte, error) {
	r.overrides++
	if r.overrideError {
		return nil, errors.New("source prepare failed")
	}
	if *msg.To != r.scope.Executor || msg.From != r.scope.Caller || b.Int64() != 7 || len(code) == 0 {
		return nil, errors.New("override identity mismatch")
	}
	args := abi.Arguments{{Type: typeOf("address")}, {Type: typeOf("bytes[]")}, {Type: typeOf("bytes[]")}, {Type: typeOf("uint256[]")}, {Type: typeOf("bool[]")}, {Type: typeOf("bytes[]")}, {Type: typeOf("bytes[]")}, {Type: typeOf("uint256[]")}}
	values, err := args.Unpack(msg.Data[4:])
	if err != nil {
		return nil, err
	}
	steps := values[2].([][]byte)
	if len(steps) != r.prefixActions+1 || steps[len(steps)-1][0] != recipes.Read {
		return nil, errors.New("prefix did not retain exact source sequence")
	}
	for _, step := range steps {
		if step[0] == recipes.Approve {
			return nil, errors.New("payment approval must not execute in funding probe")
		}
	}
	returned := make([]*big.Int, len(steps))
	for i := range returned {
		returned[i] = new(big.Int)
	}
	returned[len(returned)-1] = big.NewInt(r.cash)
	return (abi.Arguments{{Type: typeOf("uint256[]")}}).Pack(returned)
}

func capacityFixture(t *testing.T, stateful bool, mutateSwap ...func(*recipes.Leaf)) (Scope, *capacityReader) {
	t.Helper()
	s := Scope{ChainID: big.NewInt(1), Executor: common.HexToAddress("0x10"), Account: common.HexToAddress("0x11"), Caller: common.HexToAddress("0x12"), Connector: common.HexToAddress("0x13"), RWA: common.HexToAddress("0x14"), FundingAsset: common.HexToAddress("0x15"), Block: big.NewInt(7)}
	approval := erc20binding.NewERC20().PackApprove(s.Connector, new(big.Int))
	approvalMask := make([]byte, 68)
	for i := 36; i < 68; i++ {
		approvalMask[i] = 255
	}
	swap := connector.NewLiquidLaneConnector().PackSwap(connector.ILiquidLaneConnectorDiscountSwap{
		Discount:        connector.ILiquidLaneConnectorDiscount{TokenToRedeem: s.RWA, Discount: new(big.Int), Nonce: new(big.Int), Deadline: new(big.Int)},
		SignerSignature: []byte{}, ProtocolDeadline: new(big.Int),
	}, []byte{}, s.Caller, new(big.Int))
	swapMask := make([]byte, len(swap))
	for i := 100; i < 132; i++ {
		swapMask[i] = 255
	}
	leaves := []recipes.Leaf{{Target: s.FundingAsset, Value: new(big.Int), Data: approval, DataMask: approvalMask}, {Target: s.Connector, Value: new(big.Int), Data: swap, DataMask: swapMask}}
	for _, mutate := range mutateSwap {
		mutate(&leaves[1])
	}
	if stateful {
		for i := 0; i < 2; i++ {
			leaves = append(leaves, recipes.Leaf{Target: common.HexToAddress("0x16"), Value: new(big.Int), Data: []byte{1, 2, 3, byte(i)}, DataMask: make([]byte, 4)})
		}
	}
	for i := range leaves {
		var err error
		leaves[i].Hash, err = recipes.HashLeaf(leaves[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	hashes := make([]common.Hash, len(leaves))
	for i, l := range leaves {
		hashes[i] = l.Hash
	}
	pair := func(a, b common.Hash) common.Hash {
		if a.Cmp(b) > 0 {
			a, b = b, a
		}
		return crypto.Keccak256Hash(a[:], b[:])
	}
	root := pair(hashes[0], hashes[1])
	for i := 0; i < 2; i++ {
		leaves[i].Proof = []common.Hash{hashes[1-i]}
	}
	if stateful {
		other := pair(hashes[2], hashes[3])
		for i := 0; i < 2; i++ {
			leaves[i].Proof = append(leaves[i].Proof, other)
			leaves[i+2].Proof = []common.Hash{hashes[3-i], root}
		}
		root = pair(root, other)
	}
	actions := []accountbinding.IMerkleAccountAction{}
	for _, l := range leaves {
		proof := make([][32]byte, len(l.Proof))
		for i, h := range l.Proof {
			proof[i] = h
		}
		actions = append(actions, accountbinding.IMerkleAccountAction{Target: l.Target, Value: l.Value, Data: l.Data, DataMask: l.DataMask, Proof: proof})
	}
	data := accountbinding.NewMerkleAccount().PackManage(actions)
	marker := append(bytes.Clone(witnessSelector), data[4:]...)
	query, err := queryArguments.Pack(s.Connector, marker, []*big.Int{}, new(big.Int))
	if err != nil {
		t.Fatal(err)
	}
	manageArgs := abi.Arguments{{Type: typeOf("uint256")}, {Type: typeOf("bytes32[]")}, {Type: typeOf("uint256[][]")}}
	operand := func(source byte, n int64) *big.Int {
		return new(big.Int).Or(new(big.Int).Lsh(big.NewInt(int64(source)), 216), big.NewInt(n))
	}
	approveArgs := abi.Arguments{{Type: typeOf("uint256")}, {Type: typeOf("address")}, {Type: typeOf("address")}, {Type: typeOf("bytes32")}}
	approveStep, err := approveArgs.Pack(operand(recipes.Input, 1), s.FundingAsset, s.Connector, leaves[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	swapStep, err := manageArgs.Pack(operand(recipes.Input, 0), [][32]byte{leaves[1].Hash}, [][]*big.Int{{new(big.Int).Or(new(big.Int).Lsh(big.NewInt(100), 224), operand(recipes.Input, 0))}})
	if err != nil {
		t.Fatal(err)
	}
	recipe := recipes.Recipe{Queries: [][]byte{query}}
	if stateful {
		for _, l := range leaves[2:] {
			step, e := manageArgs.Pack(big.NewInt(1), [][32]byte{l.Hash}, [][]*big.Int{{}})
			if e != nil {
				t.Fatal(e)
			}
			recipe.Steps = append(recipe.Steps, append([]byte{recipes.Manage}, step...))
		}
	}
	recipe.Steps = append(recipe.Steps, append([]byte{recipes.Approve}, approveStep...), append([]byte{recipes.Manage}, swapStep...))
	r := &capacityReader{scope: s, recipe: recipe, root: root, header: &types.Header{Number: big.NewInt(7), Time: 7}, cash: 600, remaining: 1000}
	if stateful {
		r.prefixActions = 2
	}
	r.rehash(t)
	return s, r
}
