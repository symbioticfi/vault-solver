package threef

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
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
		market, err := inspectGuard(s.Account, tr.Leaves[0])
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

func TestGroupedGuardRejectsAmbiguousPairsInputsAndGate(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		mutate     func(*recipes.Recipe, *recipes.Tree)
	}{
		{"duplicate pair", "duplicate guarded funding pair", func(_ *recipes.Recipe, tr *recipes.Tree) { tr.Leaves[1].Data = tr.Leaves[0].Data }},
		{"aliased inputs", "inputs overlap", func(r *recipes.Recipe, _ *recipes.Tree) {
			args := abi.Arguments{{Type: uintType}, {Type: instructionType("bytes32[]")}, {Type: instructionType("uint256[][]")}}
			step := r.Steps[len(r.Steps)-1]
			decoded, err := args.Unpack(step[1:])
			if err != nil {
				t.Fatal(err)
			}
			shift := big.NewInt(3)
			gate := new(big.Int).Sub(decoded[0].(*big.Int), shift)
			bindings := decoded[2].([][]*big.Int)
			for i, b := range bindings[0] {
				bindings[0][i] = new(big.Int).Sub(b, shift)
			}
			packed, err := args.Pack(gate, decoded[1], bindings)
			if err != nil {
				t.Fatal(err)
			}
			r.Steps[len(r.Steps)-1] = append([]byte{recipes.Manage}, packed...)
		}},
		{"ungated group", "input identities or gate", func(r *recipes.Recipe, _ *recipes.Tree) {
			step := r.Steps[len(r.Steps)-1]
			copy(step[1:33], common.LeftPadBytes(big.NewInt(1).Bytes(), 32))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, tr := groupedFixture(t)
			tc.mutate(&r, tr)
			if _, err := inspect(s, r, tr); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
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
	if _, err := inspect(s, r, tr); err == nil || !strings.Contains(err.Error(), "32-group bound") {
		t.Fatal(fmt.Sprint("group overflow: ", err))
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

func TestGroupedProviderRejectsForeignSecondPolicyAccount(t *testing.T) {
	s, r, tr := groupedFixture(t)
	second, err := inspectGuard(s.Account, tr.Leaves[1])
	if err != nil {
		t.Fatal(err)
	}
	second.Policy = common.HexToAddress("0x9898")
	tr.Leaves[1].Data, tr.Leaves[1].DataMask, err = referenceGuard(s.Account, second)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Unix(1000, 0).Add(time.Hour).Format(time.RFC3339)
	api := &fakeAPI{offers: []wire.OfferDto{{Maker: s.Account.Hex(), Status: "AWAITING_MINT", ExecutionMode: "PUSH", RequestId: common.HexToAddress("0x901").Hex(), MintDeadline: *wire.NewNullableString(&deadline)}}}
	chain := marketReads(t, s, r, tr, func(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
		if *msg.To == second.Policy {
			return policySchema.Methods["account"].Outputs.Pack(common.HexToAddress("0xbad"))
		}
		t.Fatalf("unexpected chain read %s", msg.To)
		return nil, nil
	})
	p := mintProvider{API: api, Chain: chain, Now: func() time.Time { return time.Unix(1000, 0) }}
	if _, err := p.PrepareRecipe(t.Context(), s, r, tr, nil); err == nil || !strings.Contains(err.Error(), "policy belongs to another account") {
		t.Fatalf("foreign second policy: %v", err)
	}
}

func TestGroupedRecipeRejectsUngatedCalls(t *testing.T) {
	call, err := callArgs.Pack(common.HexToAddress("0x456"), []byte{}, []*big.Int{})
	if err != nil {
		t.Fatal(err)
	}
	args := abi.Arguments{{Type: uintType}, {Type: addressType}, {Type: addressType}, {Type: instructionType("bytes32")}}
	approve, err := args.Pack(new(big.Int), common.HexToAddress("0x456"), common.HexToAddress("0x789"), common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		opcode byte
		data   []byte
		want   string
	}{{recipes.Call, call, "ungated call"}, {recipes.Approve, approve, "ungated approval"}} {
		t.Run(tc.want, func(t *testing.T) {
			s, r, tr := groupedFixture(t)
			r.Steps = append(r.Steps, append([]byte{tc.opcode}, tc.data...))
			if _, err := inspect(s, r, tr); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("inactive side effect accepted: %v", err)
			}
		})
	}
}
