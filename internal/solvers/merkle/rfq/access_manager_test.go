package rfq

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/accessmanager"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
)

type accessManagerReader struct {
	*connectortest.Chain

	immediate bool
	delay     uint32
}

func (r *accessManagerReader) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	manager := accessmanager.NewAccessManager()
	if call.To != nil && *call.To == r.RecipeCaller && bytes.HasPrefix(call.Data, manager.GetABI().Methods["canCall"].ID) {
		args, err := manager.GetABI().Methods["canCall"].Inputs.Unpack(call.Data[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(common.Address) != r.Filler || args[1].(common.Address) != r.RecipeExecutor || args[2].([4]byte) != [4]byte(recipeBinding.GetABI().Methods["execute"].ID) || block == nil || block.Uint64() != 7 {
			return nil, errors.New("incorrect AccessManager permission read")
		}
		return manager.GetABI().Methods["canCall"].Outputs.Pack(r.immediate, r.delay)
	}
	return r.Chain.CallContract(ctx, call, block)
}

func TestConnectorAccessManagerRoute(t *testing.T) {
	for _, tc := range []struct {
		name      string
		immediate bool
		delay     uint32
		want      string
	}{
		{name: "authorized", immediate: true},
		{name: "no grant", want: "immediate"},
		{name: "delayed", delay: 60, want: "immediate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectortest.DiscountFixture())
			f.reader.RecipeCaller = common.HexToAddress("0xacce55")
			reader := &accessManagerReader{Chain: f.reader, immediate: tc.immediate, delay: tc.delay}
			f.workflow.Chain = reader
			order, selected, backend := connectorExecutionAward(f.workflow.Filler)
			built, err := buildFill(t.Context(), f.workflow, order, nil, []filler.Leg{{Source: selected}}, backend)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) || f.reader.OuterCalls != 0 {
					t.Fatalf("unauthorized route reached simulation: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			values, err := executorBinding.GetABI().Methods["fill"].Inputs.Unpack(built.Calldata[4:])
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := executorCallsArgs.Unpack(values[4].([]byte))
			if err != nil {
				t.Fatal(err)
			}
			calls := *abi.ConvertType(decoded[0], new([]executorCall)).(*[]executorCall)
			if len(calls) != 1 || calls[0].Target != f.reader.RecipeCaller || calls[0].Value.Sign() != 0 {
				t.Fatal("fill did not target manager")
			}
			manager := accessmanager.NewAccessManager()
			if !bytes.HasPrefix(calls[0].Data, manager.GetABI().Methods["execute"].ID) {
				t.Fatal("wrong manager selector")
			}
			inner, err := manager.GetABI().Methods["execute"].Inputs.Unpack(calls[0].Data[4:])
			if err != nil {
				t.Fatal(err)
			}
			if inner[0].(common.Address) != f.reader.RecipeExecutor {
				t.Fatal("wrong recipe target")
			}
			payload, err := recipeBinding.GetABI().Methods["execute"].Inputs.Unpack(inner[1].([]byte)[4:])
			if err != nil {
				t.Fatal(err)
			}
			inputs := payload[2].([]*big.Int)
			if len(inputs) != 2 || inputs[0].Int64() != 123 || inputs[1].Int64() != 456 || f.reader.OuterCalls != 1 {
				t.Fatal("awarded recipe changed or outer fill not simulated")
			}
			s := &Solver{Engine: &filler.Engine{Routes: connector.NewRoutes(f.workflow, []common.Address{f.reader.RecipeExecutor}), Workflow: f.workflow}}
			if err := s.Initialize(t.Context()); err != nil {
				t.Fatalf("manager route discovery: %v", err)
			}
			executor, account, ok := s.ResolveRoute(vlt, tIn, tOut)
			if !ok || executor != f.reader.RecipeExecutor || account != f.reader.RecipeAccount {
				t.Fatal("manager route did not publish cached quote identities")
			}
			reader.immediate = false
			if _, err := buildFill(t.Context(), f.workflow, order, nil, []filler.Leg{{Source: selected}}, backend); err == nil {
				t.Fatal("revoked manager grant accepted")
			}
		})
	}
}

func TestConnectorAccessManagerKeepsFullFillAdmission(t *testing.T) {
	for _, name := range []string{"outer revert"} {
		t.Run(name, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectortest.DiscountFixture())
			f.reader.RecipeCaller = common.HexToAddress("0xacce55")
			f.workflow.Chain = &accessManagerReader{Chain: f.reader, immediate: true}
			f.reader.OuterErr = errors.New("Reactor output shortfall")
			order, selected, backend := connectorExecutionAward(f.workflow.Filler)
			built, err := buildFill(t.Context(), f.workflow, order, nil, []filler.Leg{{Source: selected}}, backend)
			if err == nil || len(built.Calldata) != 0 || f.reader.OuterCalls != 1 {
				t.Fatalf("invalid manager fill admitted: outerCalls=%d err=%v", f.reader.OuterCalls, err)
			}
		})
	}
}
