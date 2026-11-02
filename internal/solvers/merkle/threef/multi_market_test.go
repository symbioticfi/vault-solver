package threef

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	wire "github.com/symbioticfi/vault-solver/api/threef"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func groupedFixture(t *testing.T) (provider.Snapshot, recipes.Recipe, *recipes.Tree) {
	t.Helper()
	return groupsFixture(t, 2)
}
func groupsFixture(t *testing.T, count int) (provider.Snapshot, recipes.Recipe, *recipes.Tree) {
	t.Helper()
	s, r, tr := fixture(t)
	template := r.Steps[len(r.Steps)-1]
	args := abi.Arguments{{Type: uintType}, {Type: instructionType("bytes32[]")}, {Type: instructionType("uint256[][]")}}
	for index := 1; index < count; index++ {
		market, err := inspectGuard(tr.Leaves[0])
		if err != nil {
			t.Fatal(err)
		}
		market.UnderlyingAsset = common.BigToAddress(big.NewInt(int64(10000 + index)))
		data, mask, err := referenceGuard(s.Account, market)
		if err != nil {
			t.Fatal(err)
		}
		leaf := tr.Leaves[0]
		leaf.Data = data
		leaf.DataMask = mask
		leaf.Hash = common.BigToHash(big.NewInt(int64(100000 + index)))
		tr.Leaves = append(tr.Leaves, leaf)
		decoded, err := args.Unpack(template[1:])
		if err != nil {
			t.Fatal(err)
		}
		shift := big.NewInt(int64(3 * index))
		gate := new(big.Int).Add(decoded[0].(*big.Int), shift)
		bindings := decoded[2].([][]*big.Int)
		for i, b := range bindings[0] {
			bindings[0][i] = new(big.Int).Add(b, shift)
		}
		packed, err := args.Pack(gate, []common.Hash{leaf.Hash}, bindings)
		if err != nil {
			t.Fatal(err)
		}
		r.Steps = append(r.Steps, append([]byte{recipes.Manage}, packed...))
	}
	return s, r, tr
}
func TestOneExecutorAcceptsDistinctGuardedMarkets(t *testing.T) {
	s, r, tr := groupedFixture(t)
	if _, err := inspect(s, r, tr); err != nil {
		t.Fatalf("single executor rejected two pinned policy pairs: %v", err)
	}
}
func TestSingleExecutorConfigurationHasNoArrayAlias(t *testing.T) {
	raw := `mode: simulate
stateDir: state/threef-merkle
executor: "0x0000000000000000000000000000000000000010"
curator:
  url: http://127.0.0.1:1/api/v1
url: https://bf.3f.xyz
apiKeyEnv: THREEF_CURATOR_API_KEY
broker: "0x0000000000000000000000000000000000000040"
`
	if _, err := parseConfig(configNode(t, raw)); err != nil {
		t.Fatal(err)
	}
}

func TestMaximumGroupsFitCompletePermissionResponseAndInspection(t *testing.T) {
	s, r, tr := groupsFixture(t, 32)
	got, err := inspect(s, r, tr)
	if err != nil || len(got) != 32 {
		t.Fatalf("max groups: %d %v", len(got), err)
	}
	inspection, err := recipes.Inspect(r)
	if err != nil || len(inspection.Inputs) != 96 {
		t.Fatalf("max inputs: %v %v", inspection, err)
	}
	// Include the full hex data/mask, eight-hash proofs, four policy/claim grants
	// per group, all bundle leaves and both copies of the root/account scope.
	calls := []any{}
	hashes := []common.Hash{}
	for i, leaf := range tr.Leaves {
		proof := make([]common.Hash, 8)
		calls = append(calls, map[string]any{"target": leaf.Target, "value": "0", "data": hexutil.Encode(leaf.Data), "dataMask": hexutil.Encode(leaf.DataMask), "hash": leaf.Hash, "proof": proof})
		hashes = append(hashes, leaf.Hash)
		for j := 0; j < 4; j++ {
			h := common.BigToHash(big.NewInt(int64(1000 + 4*i + j)))
			hashes = append(hashes, h)
			calls = append(calls, map[string]any{"target": s.Account, "value": "0", "data": hexutil.Encode(make([]byte, 132)), "dataMask": hexutil.Encode(make([]byte, 132)), "hash": h, "proof": proof})
		}
	}
	metadata := map[string]any{"chainId": 1, "adapter": s.Account, "account": s.Account, "root": tr.Root, "calls": calls}
	response, err := json.Marshal(map[string]any{"bundle": map[string]any{"chainId": 1, "adapter": s.Account, "account": s.Account, "root": tr.Root, "leaves": hashes, "metadata": metadata}})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(response)) >= permissions.HostMaxResponseBytes {
		t.Fatalf("full permission response %d exceeds %d", len(response), permissions.HostMaxResponseBytes)
	}
	t.Logf("32 guards, 128 policy/claim grants, 160 eight-hash proofs and complete envelope: %d/%d bytes", len(response), permissions.HostMaxResponseBytes)
	s, r, tr = groupsFixture(t, 33)
	if _, err := inspect(s, r, tr); err != nil {
		t.Fatalf("host added a group limit: %v", err)
	}
}

func TestGlobalLeaseSelectionZerosEveryInactiveMarket(t *testing.T) {
	s, r, tr := groupedFixture(t)
	markets, err := inspect(s, r, tr)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	late := now.Add(3 * time.Minute).Format(time.RFC3339)
	early := now.Add(2 * time.Minute).Format(time.RFC3339)
	first, second := common.HexToAddress("0x901"), common.HexToAddress("0x902")
	api := &fakeAPI{offers: []wire.OfferDto{{Maker: s.Account.Hex(), Status: "AWAITING_MINT", ExecutionMode: "PUSH", RequestId: first.Hex(), MintDeadline: *wire.NewNullableString(&late)}, {Maker: s.Account.Hex(), Status: "AWAITING_MINT", ExecutionMode: "PUSH", RequestId: second.Hex(), MintDeadline: *wire.NewNullableString(&early)}}}
	chain := marketReads(t, s, r, tr, func(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
		return requestABI.GetABI().Methods["mintAuthorization"].Outputs.Pack(big.NewInt(12), big.NewInt(5))
	})
	p := mintProvider{API: api, Chain: chain, Now: func() time.Time { return now }}
	out, err := p.PrepareRecipe(t.Context(), s, r, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Inputs[markets[0].RequestInput] != new(big.Int).SetBytes(second.Bytes()).String() || out.Inputs[markets[1].RequestInput] != "0" || out.Inputs[markets[1].PrincipalInput] != "0" || out.Inputs[markets[1].YieldInput] != "0" {
		t.Fatalf("global earliest lease or inactive input isolation failed: %+v", out.Inputs)
	}
	// Equal deadlines use canonical request address, independent of API ordering.
	api.offers[0].MintDeadline = *wire.NewNullableString(&early)
	out, err = p.PrepareRecipe(t.Context(), s, r, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Inputs[markets[0].RequestInput] != new(big.Int).SetBytes(first.Bytes()).String() {
		t.Fatal("equal leases lost address tie break")
	}
}

// fundingManage adds a fully pinned, permissioned ERC20 funding action. Its
// gate remains the only variable here; generic SDK decoding must accept it.
func fundingManage(t *testing.T, r *recipes.Recipe, tree *recipes.Tree, gate *big.Int, before bool) {
	t.Helper()
	leaf := recipes.Leaf{Target: common.HexToAddress("0xf00d"), Value: new(big.Int), Data: tokenABI.PackApprove(common.HexToAddress("0xbeef"), big.NewInt(1)), Hash: common.HexToHash("0xf001")}
	leaf.DataMask = make([]byte, len(leaf.Data))
	tree.Leaves = append(tree.Leaves, leaf)
	args := abi.Arguments{{Type: uintType}, {Type: instructionType("bytes32[]")}, {Type: instructionType("uint256[][]")}}
	packed, err := args.Pack(gate, []common.Hash{leaf.Hash}, [][]*big.Int{{}})
	if err != nil {
		t.Fatal(err)
	}
	step := append([]byte{recipes.Manage}, packed...)
	r.Steps = append(r.Steps, step)
	if before {
		// Swap the trailing guard rather than shifting numerical Result identities.
		last := len(r.Steps) - 1
		r.Steps[last], r.Steps[last-1] = r.Steps[last-1], r.Steps[last]
	}
	if _, err := recipes.Inspect(*r); err != nil {
		t.Fatalf("funding fixture must decode: %v", err)
	}
}

func TestCommittedFundingUsesRecipeGates(t *testing.T) {
	for name, gate := range map[string]*big.Int{
		"literal":       encodedValue(recipes.Literal, 1),
		"read result":   encodedValue(recipes.Result, 3),
		"request input": encodedValue(recipes.Input, 0),
	} {
		t.Run(name, func(t *testing.T) {
			snapshot, recipe, tree := groupedFixture(t)
			fundingManage(t, &recipe, tree, gate, false)
			if _, err := inspect(snapshot, recipe, tree); err != nil {
				t.Fatalf("committed funding gate rejected: %v", err)
			}
		})
	}
}

func TestMarketsCanShareRequestInput(t *testing.T) {
	snapshot, recipe, tree := groupedFixture(t)
	args := abi.Arguments{{Type: uintType}, {Type: instructionType("bytes32[]")}, {Type: instructionType("uint256[][]")}}
	last := len(recipe.Steps) - 1
	values, err := args.Unpack(recipe.Steps[last][1:])
	if err != nil {
		t.Fatal(err)
	}
	bindings := values[2].([][]*big.Int)
	for _, word := range bindings[0] {
		// The second group's request was Input 3; share Input 0 instead.
		var data [32]byte
		word.FillBytes(data[:])
		if new(big.Int).SetBytes(data[5:]).Cmp(big.NewInt(3)) == 0 {
			word.Sub(word, big.NewInt(3))
		}
	}
	packed, err := args.Pack(values[0], values[1], bindings)
	if err != nil {
		t.Fatal(err)
	}
	recipe.Steps[last] = append([]byte{recipes.Manage}, packed...)
	requirements, err := (&mintProvider{}).RecipeRequirements(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uint]bool{}
	for _, input := range requirements.Inputs {
		if seen[input] {
			t.Fatalf("duplicate provider ownership for input %d", input)
		}
		seen[input] = true
	}
	if len(seen) != 5 {
		t.Fatalf("shared request requirements = %v", requirements.Inputs)
	}
}
