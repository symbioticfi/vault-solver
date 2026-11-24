package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
)

func TestConfiguredExecutorDerivesPaymentConnector(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(executors []common.Address, workflow *connector.Workflow)
		want   string
	}{
		{name: "configured executor"},
		{name: "wrong executor", mutate: func(executors []common.Address, _ *connector.Workflow) { executors[0] = common.HexToAddress("0xbad") }, want: ""},
		{name: "wrong caller", mutate: func(_ []common.Address, w *connector.Workflow) { w.Filler = common.HexToAddress("0xbad") }, want: "caller"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
			executors := []common.Address{fixture.reader.RecipeExecutor}
			if tc.mutate != nil {
				tc.mutate(executors, fixture.workflow)
			}
			s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(fixture.workflow, executors), Workflow: fixture.workflow}}
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
			if executor, account, ok := s.ResolveRoute(vlt, tIn, tOut); !ok || executor != fixture.reader.RecipeExecutor || account != fixture.reader.RecipeAccount {
				t.Fatal("wrong recipe connector")
			}
			reads := fixture.reader.IdentityReads
			if err := s.Initialize(t.Context()); err != nil || fixture.reader.IdentityReads != reads {
				t.Fatalf("initialize repeated discovery: %v", err)
			}
		})
	}
}

func TestCachedRouteResolutionWithoutRPC(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
	s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(fixture.workflow, []common.Address{fixture.reader.RecipeExecutor}), Workflow: fixture.workflow}}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	s.Workflow.Chain, s.Workflow.Source = nil, nil
	for _, tc := range []struct {
		name              string
		venue, rwa, asset common.Address
		want              bool
	}{
		{"derive identities", vlt, tIn, tOut, true},
		{"wrong RWA", vlt, tOut, tOut, false},
		{"wrong connector", tIn, tIn, tOut, false},
		{"wrong output", vlt, tIn, tIn, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor, account, ok := s.ResolveRoute(tc.venue, tc.rwa, tc.asset)
			if ok != tc.want || (ok && (executor != fixture.reader.RecipeExecutor || account != fixture.reader.RecipeAccount)) {
				t.Fatalf("route=(%s,%s,%t)", executor, account, ok)
			}
		})
	}
}

func TestConfiguredExecutorWithManagedPaymentApproval(t *testing.T) {
	for _, gate := range []int64{1, 0} {
		t.Run(big.NewInt(gate).String(), func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
			args := abi.Arguments{
				{Type: connectortest.DeltaType(t, "uint256")},
				{Type: connectortest.DeltaType(t, "bytes32[]")},
				{Type: connectortest.DeltaType(t, "uint256[][]")},
			}
			amount := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(36), 224), connectortest.DeltaOperand(recipes.Input, 1))
			data, err := args.Pack(big.NewInt(gate), [][32]byte{fixture.reader.ApprovalHash}, [][]*big.Int{{amount}})
			if err != nil {
				t.Fatal(err)
			}
			fixture.reader.Recipe.Steps[0] = append([]byte{recipes.Manage}, data...)
			fixture.reader.RecipeHash, err = recipes.HashRecipe(fixture.reader.Recipe)
			if err != nil {
				t.Fatal(err)
			}
			s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(fixture.workflow, []common.Address{fixture.reader.RecipeExecutor}), Workflow: fixture.workflow}}
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
			if executor, account, ok := s.ResolveRoute(vlt, tIn, tOut); !ok || executor != fixture.reader.RecipeExecutor || account != fixture.reader.RecipeAccount {
				t.Fatal("managed approval did not identify the payment connector")
			}
		})
	}
}

// A slow recipe read during deferred publication must not stall live quotes.
func TestInitializeDoesNotBlockRouteReads(t *testing.T) {
	fixture := connectorExecutionFixture(t, connectortest.DiscountFixture())
	reader := &blockingLogsReader{Chain: fixture.reader, entered: make(chan struct{}), release: make(chan struct{})}
	workflow := *fixture.workflow
	workflow.Chain = reader
	s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(&workflow, []common.Address{fixture.reader.RecipeExecutor}), Workflow: &workflow}}
	done := make(chan error, 1)
	go func() {
		done <- s.Initialize(t.Context())
	}()
	<-reader.entered
	declined := make(chan bool, 1)
	go func() {
		_, _, resolved := s.ResolveRoute(vlt, tIn, tOut)
		declined <- !resolved
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
	if _, _, ok := s.ResolveRoute(vlt, tIn, tOut); !ok {
		t.Fatal("route not published")
	}
}

type blockingLogsReader struct {
	*connectortest.Chain

	entered, release chan struct{}
}

func (r *blockingLogsReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	close(r.entered)
	<-r.release
	return r.Chain.FilterLogs(ctx, query)
}
