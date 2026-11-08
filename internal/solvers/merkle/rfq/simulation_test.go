package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func TestOuterFillSimulationUsesCompleteSettlementWithOrdinaryReader(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	outer := append([]byte{}, executorBinding.GetABI().Methods["fill"].ID...)
	simulation := outerFillSimulation{reader: f.reader, executor: f.reader.recipeExecutor, inner: []byte{1, 2, 3}, outer: outer, caller: f.workflow.caller, signer: f.workflow.signer}
	request := recipes.PreparedBatchSimulationRequest{Calls: []recipes.ExecutorCall{{Executor: simulation.executor, Data: []byte{1, 2, 3}}}, Caller: simulation.caller, BlockNumber: big.NewInt(7), GasLimit: 16_777_216}
	if err := simulation.Simulate(t.Context(), request); err != nil {
		t.Fatalf("complete outer fill with empty return rejected: %v", err)
	}
	if f.reader.outerCalls != 1 {
		t.Fatal("complete settlement was not simulated exactly once")
	}
	failure := errors.New("canonical Reactor settlement rejected")
	f.reader.outerErr = failure
	if err := simulation.Simulate(t.Context(), request); !errors.Is(err, failure) {
		t.Fatalf("canonical settlement failure was masked: %v", err)
	}
}

func TestOuterFillSimulationRejectsChangedAdmissionContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*outerFillSimulation, *recipes.PreparedBatchSimulationRequest)
	}{
		{"no calls", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) { r.Calls = nil }},
		{"multiple calls", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) {
			r.Calls = append(r.Calls, r.Calls[0])
		}},
		{"different executor", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) {
			r.Calls[0].Executor = common.HexToAddress("0xbad")
		}},
		{"different inner calldata", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) { r.Calls[0].Data = []byte{9} }},
		{"different caller", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) {
			r.Caller = common.HexToAddress("0xbad")
		}},
		{"unpinned block", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) { r.BlockNumber = nil }},
		{"negative block", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) {
			r.BlockNumber = big.NewInt(-1)
		}},
		{"zero gas", func(_ *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) { r.GasLimit = 0 }},
		{"zero signer", func(s *outerFillSimulation, _ *recipes.PreparedBatchSimulationRequest) { s.signer = common.Address{} }},
		{"zero caller", func(s *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) {
			s.caller, r.Caller = common.Address{}, common.Address{}
		}},
		{"zero executor", func(s *outerFillSimulation, r *recipes.PreparedBatchSimulationRequest) {
			s.executor, r.Calls[0].Executor = common.Address{}, common.Address{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectorDiscountFixture())
			simulation := outerFillSimulation{reader: f.reader, executor: f.reader.recipeExecutor, inner: []byte{1}, outer: executorBinding.GetABI().Methods["fill"].ID, caller: f.workflow.caller, signer: f.workflow.signer}
			request := recipes.PreparedBatchSimulationRequest{Calls: []recipes.ExecutorCall{{Executor: simulation.executor, Data: []byte{1}}}, Caller: simulation.caller, BlockNumber: big.NewInt(7), GasLimit: 16_777_216}
			tc.edit(&simulation, &request)
			if err := simulation.Simulate(t.Context(), request); err == nil || f.reader.outerCalls != 0 {
				t.Fatalf("changed context reached outer simulation: calls=%d err=%v", f.reader.outerCalls, err)
			}
		})
	}
}

func TestOuterFillSimulationHonorsCancellation(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	simulation := outerFillSimulation{reader: f.reader, executor: f.reader.recipeExecutor, inner: []byte{1}, outer: executorBinding.GetABI().Methods["fill"].ID, caller: f.workflow.caller, signer: f.workflow.signer}
	request := recipes.PreparedBatchSimulationRequest{Calls: []recipes.ExecutorCall{{Executor: simulation.executor, Data: []byte{1}}}, Caller: simulation.caller, BlockNumber: big.NewInt(7), GasLimit: 16_777_216}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := simulation.Simulate(ctx, request); !errors.Is(err, context.Canceled) || f.reader.outerCalls != 0 {
		t.Fatalf("canceled simulation reached RPC: calls=%d err=%v", f.reader.outerCalls, err)
	}
}
