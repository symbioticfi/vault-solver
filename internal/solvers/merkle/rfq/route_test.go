package rfq

import (
	"context"
	"math/big"
	"slices"
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
		{name: "wrong caller", mutate: func(s *Solver) { s.workflow.caller = common.HexToAddress("0xbad") }, want: "caller mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: fixture.workflow}
			s.workflow.connector = common.Address{}
			if tc.mutate != nil {
				tc.mutate(s)
			}
			venue, err := s.Initialize(t.Context())
			if tc.mutate != nil {
				if err == nil || (tc.want != "" && !strings.Contains(err.Error(), tc.want)) {
					t.Fatalf("expected %s, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(venue) != 1 || venue[0] != vlt || !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut) {
				t.Fatalf("wrong recipe connector %s", venue)
			}
			again, err := s.Initialize(t.Context())
			if err != nil || !slices.Equal(again, venue) {
				t.Fatalf("initialize not stable: %s %v", again, err)
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
			venues, err := s.Initialize(t.Context())
			if gate == 0 {
				if err == nil || !strings.Contains(err.Error(), "unconditional") {
					t.Fatalf("conditional payment accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(venues, []common.Address{vlt}) || !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut) {
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
	workflow.connector = common.Address{}
	s := &Solver{recipeExecutors: []common.Address{fixture.reader.recipeExecutor}, workflow: &workflow}
	done := make(chan error, 1)
	go func() {
		_, err := s.Initialize(t.Context())
		done <- err
	}()
	<-reader.entered
	declined := make(chan bool, 1)
	go func() {
		declined <- !s.AcceptsConnector(vlt) && !s.AcceptsRoute(fixture.reader.recipeExecutor, vlt, fixture.reader.recipeAccount, tOut)
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
