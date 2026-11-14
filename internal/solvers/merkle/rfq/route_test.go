package rfq

import (
	"context"
	"math/big"
	"strconv"
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
			if executor, account, ok := s.ResolveRoute(vlt, tIn, tOut); !ok || executor != fixture.reader.recipeExecutor || account != fixture.reader.recipeAccount {
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
	s.workflow.chain, s.workflow.source = nil, nil
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
			if ok != tc.want || (ok && (executor != fixture.reader.recipeExecutor || account != fixture.reader.recipeAccount)) {
				t.Fatalf("route=(%s,%s,%t)", executor, account, ok)
			}
		})
	}
}

func TestCachedRouteResolutionRequiresUniqueRWA(t *testing.T) {
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
			executor, account, ok := s.ResolveRoute(vlt, tIn, tOut)
			if ok == sameRWA || (ok && (executor != first || account != vlt)) {
				t.Fatalf("route=(%s,%s,%t)", executor, account, ok)
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
			if executor, account, ok := s.ResolveRoute(vlt, tIn, tOut); !ok || executor != fixture.reader.recipeExecutor || account != fixture.reader.recipeAccount {
				t.Fatal("managed approval did not identify the payment connector")
			}
		})
	}
}

func TestPaymentConnectorRecognizesGuardedInputPayment(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			mutate  func([][]byte)
			invalid bool
		}{
			{name: "positive input guard"},
			{name: "missing positivity check", mutate: func(steps [][]byte) {
				steps[0] = connectorDeltaBinary(t, recipes.LessOrEqual, big.NewInt(0), connectorDeltaOperand(recipes.Input, 0))
			}, invalid: true},
			{name: "different guard input", mutate: func(steps [][]byte) {
				steps[1] = connectorDeltaBinary(t, recipes.Min, big.NewInt(1), connectorDeltaOperand(recipes.Input, 2))
			}, invalid: true},
			{name: "unbounded multiplier", mutate: func(steps [][]byte) { steps[1][0] = recipes.Max }, invalid: true},
			{name: "larger multiplier", mutate: func(steps [][]byte) {
				steps[1] = connectorDeltaBinary(t, recipes.Min, big.NewInt(2), connectorDeltaOperand(recipes.Input, 0))
			}, invalid: true},
			{name: "different payment input", mutate: func(steps [][]byte) { steps[2][32] = 0 }, invalid: true},
			{name: "different divisor", mutate: func(steps [][]byte) { steps[2][96] = 2 }, invalid: true},
		} {
			t.Run(tc.name+"/managed="+strconv.FormatBool(managed), func(t *testing.T) {
				fixture := connectorExecutionFixture(t, connectorDiscountFixture())
				tree, err := recipes.DecodeTree(fixture.workflow.source.(connectorExecutionPermissions).raw)
				if err != nil {
					t.Fatal(err)
				}
				word := connectorDeltaType(t, "uint256")
				mul, err := (abi.Arguments{{Type: word}, {Type: word}, {Type: word}}).Pack(connectorDeltaOperand(recipes.Input, 1), connectorDeltaOperand(recipes.Result, 1), big.NewInt(1))
				if err != nil {
					t.Fatal(err)
				}
				steps := [][]byte{
					connectorDeltaBinary(t, recipes.LessOrEqual, big.NewInt(1), connectorDeltaOperand(recipes.Input, 0)),
					connectorDeltaBinary(t, recipes.Min, big.NewInt(1), connectorDeltaOperand(recipes.Input, 0)),
					append([]byte{recipes.MulDiv}, mul...),
				}
				var data []byte
				if managed {
					binding := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(36), 224), connectorDeltaOperand(recipes.Result, 2))
					data, err = (abi.Arguments{{Type: word}, {Type: connectorDeltaType(t, "bytes32[]")}, {Type: connectorDeltaType(t, "uint256[][]")}}).Pack(big.NewInt(1), [][32]byte{fixture.reader.approvalHash}, [][]*big.Int{{binding}})
					steps = append(steps, append([]byte{recipes.Manage}, data...))
				} else {
					data, err = (abi.Arguments{{Type: word}, {Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes32")}}).Pack(connectorDeltaOperand(recipes.Result, 2), tOut, vlt, fixture.reader.approvalHash)
					steps = append(steps, append([]byte{recipes.Approve}, data...))
				}
				if err != nil {
					t.Fatal(err)
				}
				if tc.mutate != nil {
					tc.mutate(steps)
				}
				venue, asset, err := paymentConnector(recipes.Recipe{Steps: steps}, tree)
				if tc.invalid {
					if err == nil {
						t.Fatal("non-exact payment approval accepted")
					}
				} else if err != nil || venue != vlt || asset != tOut {
					t.Fatalf("guarded payment route=(%s,%s): %v", venue, asset, err)
				}
			})
		}
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
	*connectorExecutionReader

	entered, release chan struct{}
}

func (r *blockingLogsReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	close(r.entered)
	<-r.release
	return r.connectorExecutionReader.FilterLogs(ctx, query)
}
