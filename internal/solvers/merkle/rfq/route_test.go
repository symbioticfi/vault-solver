package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func TestConfiguredExecutorDerivesPaymentConnector(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Solver)
		want   string
	}{
		{name: "configured executor"},
		{name: "wrong executor", mutate: func(s *Solver) { s.recipeExecutors[0] = common.HexToAddress("0xbad") }, want: ""},
		{name: "wrong caller", mutate: func(s *Solver) { s.workflow.caller = common.HexToAddress("0xbad") }, want: "caller"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: fixture.workflow}
			if tc.mutate != nil {
				tc.mutate(s)
			}
			err := s.Initialize(t.Context())
			if tc.mutate != nil {
				if err == nil || (tc.want != "" && !strings.Contains(err.Error(), tc.want)) {
					t.Fatalf("expected %s, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !s.AcceptsConnector(vlt) || !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut) {
				t.Fatal("wrong recipe connector")
			}
			reads := fixture.reader.identityReads
			if err := s.Initialize(t.Context()); err != nil || fixture.reader.identityReads != reads {
				t.Fatalf("initialize repeated discovery: %v", err)
			}
		})
	}
}

func TestCachedRouteResolutionWithoutRPC(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectorDiscountFixture())
	s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: fixture.workflow}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	// All chain and permission-source access is unavailable after initialization.
	s.workflow.chain, s.workflow.source = nil, nil
	for _, tc := range []struct {
		name                    string
		hint, venue, rwa, asset common.Address
		want                    bool
	}{
		{"derive both", common.Address{}, vlt, tIn, tOut, true},
		{"matching capacity binding", fixture.reader.recipeExecutor, vlt, tIn, tOut, true},
		{"wrong RWA", common.Address{}, vlt, tOut, tOut, false},
		{"wrong connector", common.Address{}, tIn, tIn, tOut, false},
		{"wrong output", common.Address{}, vlt, tIn, tIn, false},
		{"unconfigured hint", common.HexToAddress("0xbad"), vlt, tIn, tOut, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor, account, ok := s.ResolveRoute(tc.hint, tc.venue, tc.rwa, tc.asset)
			if ok != tc.want || (ok && (executor != fixture.reader.recipeExecutor || account != fixture.reader.recipeAccount)) {
				t.Fatalf("route=(%s,%s,%t), want resolved=%t", executor, account, ok, tc.want)
			}
		})
	}
}

func TestCachedRouteResolutionRequiresUnambiguousCapacity(t *testing.T) {
	first, second := common.HexToAddress("0x43"), common.HexToAddress("0x44")
	for name, sameRWA := range map[string]bool{"different RWA": false, "same RWA": true} {
		t.Run(name, func(t *testing.T) {
			otherToken := tOut
			if sameRWA {
				otherToken = tIn
			}
			s := &Solver{routes: map[common.Address]recipeRoute{
				first:  {connector: vlt, account: vlt, asset: tOut, tokens: map[common.Address]struct{}{tIn: {}}},
				second: {connector: vlt, account: vlt, asset: tOut, tokens: map[common.Address]struct{}{otherToken: {}}},
			}}
			for _, hint := range []common.Address{{}, first, second} {
				executor, account, ok := s.ResolveRoute(hint, vlt, tIn, tOut)
				want := hint == first || (hint == (common.Address{}) && !sameRWA) || (hint == second && sameRWA)
				if ok != want || (ok && (account != vlt || (hint != (common.Address{}) && executor != hint))) {
					t.Fatalf("sameRWA=%t hint=%s route=(%s,%s,%t)", sameRWA, hint, executor, account, ok)
				}
			}
		})
	}
}

func TestConfiguredExecutorWithManagedPaymentApproval(t *testing.T) {
	for _, gate := range []int64{1, 0} {
		t.Run(big.NewInt(gate).String(), func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			args := abi.Arguments{
				{Type: connectorDeltaType(t, "uint256")},
				{Type: connectorDeltaType(t, "bytes32[]")},
				{Type: connectorDeltaType(t, "uint256[][]")},
			}
			amount := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(36), 224), connectorDeltaOperand(recipes.Input, 1))
			data, err := args.Pack(big.NewInt(gate), [][32]byte{fixture.reader.approvalHash}, [][]*big.Int{{amount}})
			if err != nil {
				t.Fatal(err)
			}
			fixture.reader.recipe.Steps[0] = append([]byte{recipes.Manage}, data...)
			fixture.reader.recipeHash, err = recipes.HashRecipe(fixture.reader.recipe)
			if err != nil {
				t.Fatal(err)
			}
			s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: fixture.workflow}
			err = s.Initialize(t.Context())
			if gate == 0 {
				if err == nil || !strings.Contains(err.Error(), "unconditional") {
					t.Fatalf("conditional payment accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !s.AcceptsConnector(vlt) || !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut) {
				t.Fatal("managed approval did not identify the payment connector")
			}
		})
	}
}

// A slow recipe read during deferred publication must not stall live quotes.
func TestInitializeDoesNotBlockRouteReads(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectorDiscountFixture())
	reader := &blockingLogsReader{connectorExecutionReader: fixture.reader, entered: make(chan struct{}), release: make(chan struct{})}
	workflow := *fixture.workflow
	workflow.chain = reader
	s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: &workflow}
	done := make(chan error, 1)
	go func() {
		done <- s.Initialize(t.Context())
	}()
	<-reader.entered
	declined := make(chan bool, 1)
	go func() {
		_, _, resolved := s.ResolveRoute(common.Address{}, vlt, tIn, tOut)
		declined <- !resolved && !s.AcceptsConnector(vlt) && !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut)
	}()
	select {
	case ok := <-declined:
		if !ok {
			t.Fatal("route accepted before publication")
		}
	case <-time.After(time.Second):
		t.Fatal("route read blocked on recipe discovery")
	}
	close(reader.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !s.AcceptsConnector(vlt) {
		t.Fatal("route not published")
	}
}

type blockingLogsReader struct {
	*connectorExecutionReader

	entered, release chan struct{}
}

func (r *blockingLogsReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	close(r.entered)
	<-r.release
	return r.connectorExecutionReader.FilterLogs(ctx, query)
}
