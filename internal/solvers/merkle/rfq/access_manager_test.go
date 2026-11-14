package rfq

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/accessmanager"
)

type accessManagerReader struct {
	*connectorExecutionReader

	immediate bool
	delay     uint32
}

func (r *accessManagerReader) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	manager := accessmanager.NewAccessManager()
	if call.To != nil && *call.To == r.recipeCaller && bytes.HasPrefix(call.Data, manager.GetABI().Methods["canCall"].ID) {
		args, err := manager.GetABI().Methods["canCall"].Inputs.Unpack(call.Data[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(common.Address) != r.rfqCaller || args[1].(common.Address) != r.recipeExecutor || args[2].([4]byte) != [4]byte(recipeBinding.GetABI().Methods["execute"].ID) || block == nil || block.Uint64() != 7 {
			return nil, errors.New("incorrect AccessManager permission read")
		}
		return manager.GetABI().Methods["canCall"].Outputs.Pack(r.immediate, r.delay)
	}
	return r.connectorExecutionReader.CallContract(ctx, call, block)
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
			f := connectorExecutionFixture(t, connectorDiscountFixture())
			f.reader.recipeCaller = common.HexToAddress("0xacce55")
			reader := &accessManagerReader{connectorExecutionReader: f.reader, immediate: tc.immediate, delay: tc.delay}
			f.workflow.chain = reader
			order, selected, backend := connectorExecutionAward(f.workflow.caller)
			built, err := f.workflow.build(t.Context(), order, nil, selected, backend)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) || f.reader.outerCalls != 0 {
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
			if len(calls) != 1 || calls[0].Target != f.reader.recipeCaller || calls[0].Value.Sign() != 0 {
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
			if inner[0].(common.Address) != f.reader.recipeExecutor {
				t.Fatal("wrong recipe target")
			}
			payload, err := recipeBinding.GetABI().Methods["execute"].Inputs.Unpack(inner[1].([]byte)[4:])
			if err != nil {
				t.Fatal(err)
			}
			inputs := payload[2].([]*big.Int)
			if len(inputs) != 2 || inputs[0].Int64() != 123 || inputs[1].Int64() != 456 || f.reader.outerCalls != 1 {
				t.Fatal("awarded recipe changed or outer fill not simulated")
			}
			s := &Solver{recipeExecutors: []common.Address{f.reader.recipeExecutor}, workflow: f.workflow}
			if err := s.Initialize(t.Context()); err != nil || !s.AcceptsConnector(vlt) {
				t.Fatalf("manager route discovery: %v", err)
			}
			executor, account, ok := s.ResolveRoute(vlt, tIn, tOut)
			if !ok || executor != f.reader.recipeExecutor || account != f.reader.recipeAccount {
				t.Fatal("manager route did not publish cached quote identities")
			}
			reader.immediate = false
			if _, err := f.workflow.build(t.Context(), order, nil, selected, backend); err == nil {
				t.Fatal("revoked manager grant accepted")
			}
		})
	}
}
