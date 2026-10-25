package threef

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	wire "github.com/symbioticfi/vault-solver/api/threef"
)

func fixture(t *testing.T) (provider.Snapshot, recipes.Recipe, *recipes.Tree) {
	t.Helper()
	return loadFixture(t, "testdata/3f-executing.json")
}

func loadFixture(t *testing.T, path string) (provider.Snapshot, recipes.Recipe, *recipes.Tree) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Account common.Address `json:"account"`
		Recipe  hexutil.Bytes  `json:"recipe"`
		Data    hexutil.Bytes  `json:"mintLeafData"`
		Mask    hexutil.Bytes  `json:"mintLeafMask"`
		Hash    common.Hash    `json:"mintLeafHash"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(referenceJSON, &doc); err != nil {
		t.Fatal(err)
	}
	recipe, err := recipes.DecodeRecipe(doc.Recipe)
	if err != nil {
		t.Fatal(err)
	}
	leaf := recipes.Leaf{Target: doc.Account, Data: doc.Data, DataMask: doc.Mask, Hash: doc.Hash, Value: new(big.Int)}
	return provider.Snapshot{ChainID: big.NewInt(1), Account: doc.Account, Executor: common.HexToAddress("0x111"), BlockNumber: big.NewInt(10), Timestamp: 1000}, recipe, &recipes.Tree{Leaves: []recipes.Leaf{leaf}}
}

func TestLatestAdaptersRecipe(t *testing.T) {
	snapshot, recipe, tree := loadFixture(t, "testdata/3f-executing-latest.json")
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(markets) != 1 || markets[0].PrincipalInput != 1 || markets[0].YieldInput != 2 || markets[0].RequestInput != 0 {
		t.Fatalf("latest adapters recipe has unexpected markets: %+v", markets)
	}
}

func TestCanonicalGuardAndEverySecurityBoundary(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	if market.PrincipalInput != 1 || market.YieldInput != 2 || market.RequestInput != 0 {
		t.Fatalf("wrong derived input identities: %+v", market)
	}
	for name, mutate := range map[string]func(*provider.Snapshot, *recipes.Recipe, *recipes.Tree){
		"custody": func(s *provider.Snapshot, _ *recipes.Recipe, _ *recipes.Tree) {
			s.Account = common.HexToAddress("0x987")
		},
		"extra mask":       func(_ *provider.Snapshot, _ *recipes.Recipe, tr *recipes.Tree) { tr.Leaves[0].DataMask[0] = 255 },
		"guard alteration": func(_ *provider.Snapshot, _ *recipes.Recipe, tr *recipes.Tree) { tr.Leaves[0].Data[4+32*4] ^= 1 },
		"native value":     func(_ *provider.Snapshot, _ *recipes.Recipe, tr *recipes.Tree) { tr.Leaves[0].Value.SetInt64(1) },
		"missing request binding": func(_ *provider.Snapshot, r *recipes.Recipe, _ *recipes.Tree) {
			r.Steps[len(r.Steps)-1] = []byte{recipes.Manage}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, r, tr := fixture(t)
			mutate(&s, &r, tr)
			if _, err := inspect(s, r, tr); err == nil {
				t.Fatal("unsafe program accepted")
			}
		})
	}
	// Context substitution changes only the account, funding pair and policy pins.
	other := mintMarket{Asset: common.HexToAddress("0x1234"), UnderlyingAsset: common.HexToAddress("0x5678"), Policy: common.HexToAddress("0x9abc")}
	data, mask, err := referenceGuard(common.HexToAddress("0xcccc"), other)
	if err != nil {
		t.Fatal(err)
	}
	leaf := recipes.Leaf{Data: data, DataMask: mask, Value: new(big.Int)}
	if _, err := inspectGuard(common.HexToAddress("0xcccc"), leaf); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectGuard(snapshot.Account, leaf); err == nil {
		t.Fatal("guard from another account accepted")
	}
}

type fakeAPI struct {
	offers      []wire.OfferDto
	identityErr error
	calls       int
	onOffers    func()
}

func (f *fakeAPI) Identity(context.Context, common.Address) error { f.calls++; return f.identityErr }
func (f *fakeAPI) Offers(context.Context) ([]wire.OfferDto, error) {
	f.calls++
	if f.onOffers != nil {
		f.onOffers()
	}
	return f.offers, nil
}

type readFunc func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)

func (f readFunc) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	return f(ctx, msg, block)
}

func marketReads(t *testing.T, snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree, next readFunc) readFunc {
	t.Helper()
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	return func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		if block.Cmp(snapshot.BlockNumber) != 0 {
			t.Fatal("market reads lost block pin")
		}
		if *msg.To == market.Asset {
			return tokenABI.GetABI().Methods["balanceOf"].Outputs.Pack(big.NewInt(123))
		}
		if *msg.To == snapshot.Account {
			return capabilitySchema.Methods["execute"].Outputs.Pack([]*big.Int{big.NewInt(45), new(big.Int), big.NewInt(44), big.NewInt(1), new(big.Int)})
		}
		if *msg.To == market.Policy {
			method, e := policySchema.MethodById(msg.Data)
			if e != nil {
				return nil, e
			}
			switch method.Name {
			case "account":
				return method.Outputs.Pack(snapshot.Account)
			case "remainingAssets":
				return method.Outputs.Pack(big.NewInt(45))
			case "minYieldPerRequest":
				return method.Outputs.Pack(big.NewInt(2_000_000))
			}
		}
		return next(ctx, msg, block)
	}
}

func TestAggregateAuthorizationLeaseAndPreparationAreReadOnly(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	now := time.Unix(1000, 0).UTC()
	deadline := now.Add(2 * time.Minute).Format(time.RFC3339)
	request := common.HexToAddress("0x123456")
	offer := wire.OfferDto{Maker: snapshot.Account.Hex(), Status: "AWAITING_MINT", RequestId: request.Hex(), ExecutionMode: "PUSH", MintDeadline: *wire.NewNullableString(&deadline), PendingAmount: "3", PendingExpectedReturn: "1"}
	api := &fakeAPI{offers: []wire.OfferDto{offer, offer}}
	calls := 0
	chain := marketReads(t, snapshot, recipe, tree, func(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		calls++
		if msg.To == nil || *msg.To != request || block.Cmp(snapshot.BlockNumber) != 0 || !bytes.Equal(msg.Data, requestABI.PackMintAuthorization(snapshot.Account)) {
			t.Fatal("mint read lost request/account/block pin")
		}
		return requestABI.GetABI().Methods["mintAuthorization"].Outputs.Pack(big.NewInt(12), big.NewInt(5))
	})
	runtime := &mintProvider{API: api, Chain: chain, Now: func() time.Time { return now }}
	out, err := runtime.PrepareRecipe(t.Context(), snapshot, recipe, tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || out.Inputs[1] != "12" || out.Inputs[2] != "5" {
		t.Fatalf("must use aggregate chain authorization instead of one offered fill: %+v", out)
	}
	if err := runtime.ValidateLease(t.Context(), snapshot, out); err != nil {
		t.Fatal(err)
	}
	api.onOffers = func() { now = now.Add(100 * time.Second) }
	if err := runtime.ValidateLease(t.Context(), snapshot, out); !errors.Is(err, provider.ErrNotReady) {
		t.Fatalf("slow API consumed admission margin: %v", err)
	}
	api.onOffers = nil
	now = now.Add(-100 * time.Second)
	now = now.Add(100 * time.Second)
	if err := runtime.ValidateLease(t.Context(), snapshot, out); !errors.Is(err, provider.ErrNotReady) {
		t.Fatalf("expired admission: %v", err)
	}
	now = now.Add(-100 * time.Second)
	api.offers = nil
	if err := runtime.ValidateLease(t.Context(), snapshot, out); !errors.Is(err, provider.ErrNotReady) {
		t.Fatalf("revoked API lease: %v", err)
	}
	if _, err := runtime.PrepareRecipe(t.Context(), snapshot, recipe, tree, nil); !errors.Is(err, provider.ErrNotReady) {
		t.Fatalf("no accepted offers must idle: %v", err)
	}
	// An unauthenticated recipe fails before any HTTP or onchain authorization read.
	api.calls = 0
	tree.Leaves = nil
	if _, err := runtime.PrepareRecipe(t.Context(), snapshot, recipe, tree, nil); err == nil || api.calls != 0 {
		t.Fatal("external reads preceded authenticated recipe")
	}
}

func TestMintRejectsMalformedLeaseAndAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, deadline   string
		principal, yield *big.Int
		rpcFail          bool
	}{
		{"missing deadline", "", big.NewInt(1), big.NewInt(1), false},
		{"bad deadline", "not-a-date", big.NewInt(1), big.NewInt(1), false},
		{"expired", "1970-01-01T00:16:50Z", big.NewInt(1), big.NewInt(1), false},
		{"zero authorization", "1970-01-01T00:20:00Z", new(big.Int), new(big.Int), false},
		{"RPC failed", "1970-01-01T00:20:00Z", big.NewInt(1), big.NewInt(1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, recipe, tree := fixture(t)
			deadline := tc.deadline
			api := &fakeAPI{offers: []wire.OfferDto{{Maker: snapshot.Account.Hex(), Status: "AWAITING_MINT", ExecutionMode: "PUSH", RequestId: common.HexToAddress("0x123").Hex(), MintDeadline: *wire.NewNullableString(&deadline)}}}
			chain := marketReads(t, snapshot, recipe, tree, func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
				if tc.rpcFail {
					return nil, errors.New("failed")
				}
				return requestABI.GetABI().Methods["mintAuthorization"].Outputs.Pack(tc.principal, tc.yield)
			})
			p := mintProvider{API: api, Chain: chain, Now: func() time.Time { return time.Unix(1000, 0) }}
			if _, err := p.PrepareRecipe(t.Context(), snapshot, recipe, tree, nil); err == nil {
				t.Fatal("invalid mint became executable")
			}
		})
	}
}

type policyRevertError struct{}

func (policyRevertError) Error() string  { return "execution reverted" }
func (policyRevertError) ErrorData() any { return "0x1234" }

func TestMintSkipsIneligibleEarlierRequests(t *testing.T) {
	for _, tc := range []struct {
		name             string
		principal, yield int64
		wrongPair        bool
	}{
		{"another funding pair", 12, 5, true},
		{"exceeds policy budget", 46, 5, false},
		{"insufficient yield", 12, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, recipe, tree := fixture(t)
			now := time.Unix(1000, 0).UTC()
			first, second := common.HexToAddress("0x123456"), common.HexToAddress("0x789abc")
			api := &fakeAPI{}
			for i, address := range []common.Address{first, second} {
				deadline := now.Add(time.Duration(i+2) * time.Minute).Format(time.RFC3339)
				api.offers = append(api.offers, wire.OfferDto{Maker: snapshot.Account.Hex(), Status: "AWAITING_MINT", ExecutionMode: "PUSH", RequestId: address.Hex(), MintDeadline: *wire.NewNullableString(&deadline)})
			}
			chain := marketReads(t, snapshot, recipe, tree, func(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
				if *msg.To == first {
					return requestABI.GetABI().Methods["mintAuthorization"].Outputs.Pack(big.NewInt(tc.principal), big.NewInt(tc.yield))
				}
				if *msg.To != second {
					t.Fatal("unexpected mint request")
				}
				return requestABI.GetABI().Methods["mintAuthorization"].Outputs.Pack(big.NewInt(3), big.NewInt(1))
			})
			withPolicy := readFunc(func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
				if *msg.To == snapshot.Account && tc.wrongPair && bytes.Contains(msg.Data, common.LeftPadBytes(first.Bytes(), 32)) {
					return nil, policyRevertError{}
				}
				return chain(ctx, msg, block)
			})
			p := mintProvider{API: api, Chain: withPolicy, Now: func() time.Time { return now }}
			out, err := p.PrepareRecipe(t.Context(), snapshot, recipe, tree, nil)
			if err != nil {
				t.Fatal(err)
			}
			if out.Inputs[0] != new(big.Int).SetBytes(second.Bytes()).String() || out.Inputs[1] != "3" {
				t.Fatalf("earlier ineligible request blocked executable mint: %+v", out)
			}
		})
	}
}

func TestConfirmedMintRequiresCanonicalReceiptAndPolicyDebit(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	record := mint{Request: common.HexToAddress("0x123"), Policy: market.Policy, Asset: market.Asset, UnderlyingAsset: market.UnderlyingAsset, Principal: "12", Deadline: time.Unix(2000, 0)}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	out := provider.Output{Data: data}
	p := mintProvider{}
	if _, err := p.Advance(t.Context(), snapshot, out, nil); err == nil {
		t.Fatal("unconfirmed mint accepted")
	}
	snapshot.Execution = &provider.Execution{TxHash: common.HexToHash("0xabc"), BlockHash: common.HexToHash("0xdef"), BlockNumber: big.NewInt(12)}
	if _, err := p.Advance(t.Context(), snapshot, out, nil); err == nil {
		t.Fatal("missing debit accepted")
	}
	event := policySchema.Events["Consume"]
	packed, err := event.Inputs.NonIndexed().Pack(big.NewInt(12))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Execution.Logs = []types.Log{{Address: market.Policy, Topics: []common.Hash{event.ID, common.BytesToHash(market.Asset.Bytes()), common.BytesToHash(market.UnderlyingAsset.Bytes())}, Data: packed, TxHash: snapshot.Execution.TxHash, BlockHash: snapshot.Execution.BlockHash, BlockNumber: 12}}
	result, err := p.Advance(t.Context(), snapshot, out, nil)
	if err != nil || !result.Done {
		t.Fatalf("canonical confirmed debit rejected: %+v %v", result, err)
	}
	snapshot.Execution.Logs = append(snapshot.Execution.Logs, snapshot.Execution.Logs[0])
	snapshot.Execution.Logs[1].Index = 1
	if _, err := p.Advance(t.Context(), snapshot, out, nil); err == nil {
		t.Fatal("ambiguous duplicate debit accepted")
	}
}

func TestEligibilityUsesNativePolicyAndExactBudgetDelta(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	for _, tc := range []struct {
		name  string
		value int64
		err   error
		want  string
	}{
		{name: "eligible", value: 1}, {name: "wrong pair", value: 0, want: "invalid eligibility"}, {name: "transport", err: errors.New("secret URL"), want: "eligibility read failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := readFunc(func(_ context.Context, msg ethereum.CallMsg, pin *big.Int) ([]byte, error) {
				if pin.Cmp(snapshot.BlockNumber) != 0 || msg.From != snapshot.Account || msg.To == nil || *msg.To != snapshot.Account {
					t.Fatal("lost native account execution context")
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return capabilitySchema.Methods["execute"].Outputs.Pack([]*big.Int{big.NewInt(10), new(big.Int), big.NewInt(9), big.NewInt(tc.value), new(big.Int)})
			})
			err := eligible(t.Context(), chain, snapshot, market, common.HexToAddress("0x777"), big.NewInt(2_000_000))
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("unexpected eligibility: %v", err)
			}
		})
	}
}

func TestObserveDerivesLiveCashBudgetAndCommittedCap(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	malformed := false
	chain := readFunc(func(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		if block.Cmp(snapshot.BlockNumber) != 0 {
			t.Fatal("market observation lost block pin")
		}
		if malformed {
			return []byte{1}, nil
		}
		if *msg.To == market.Asset {
			return tokenABI.GetABI().Methods["balanceOf"].Outputs.Pack(big.NewInt(123))
		}
		method, err := policySchema.MethodById(msg.Data)
		if err != nil {
			return nil, err
		}
		switch method.Name {
		case "account":
			return method.Outputs.Pack(snapshot.Account)
		case "remainingAssets":
			return method.Outputs.Pack(big.NewInt(45))
		case "minYieldPerRequest":
			return method.Outputs.Pack(big.NewInt(2_000_000))
		default:
			t.Fatal("unexpected getter")
			return nil, nil
		}
	})
	observed, err := observe(t.Context(), chain, snapshot, recipe, market, nil)
	if err != nil {
		t.Fatal(err)
	}
	if observed.AvailableFunding.Int64() != 123 || observed.Budget.Int64() != 45 || observed.MinimumYield.Int64() != 2_000_000 || observed.MaxPrincipal.Int64() != 100_000_000 {
		t.Fatalf("wrong cash/budget/ratio/cap: %+v", observed)
	}
	malformed = true
	if _, err := observe(t.Context(), chain, snapshot, recipe, market, nil); err == nil {
		t.Fatal("malformed required getter accepted")
	}
}
